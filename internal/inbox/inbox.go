// Package inbox handles buyer–seller messages, per-user notifications and
// site-wide announcements — everything behind the header bell.
package inbox

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/lib/pq"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jconner5017-rgb/marketplace/internal/listings"
)

var (
	ErrNotFound     = errors.New("conversation not found")
	ErrOwnListing   = errors.New("that's your own listing")
	ErrEmpty        = errors.New("write a message first")
	ErrTooLong      = errors.New("messages can be up to 2,000 characters")
	ErrOffPlatform  = errors.New("keep payment and contact on Slabs & Singles; messages with emails, phone numbers or payment apps aren't sent")
	ErrNotAvailable = errors.New("this listing isn't available")
)

const maxBody = 2000

type Service struct {
	DB       *sql.DB
	Listings *listings.Service
}

func clean(body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", ErrEmpty
	}
	if utf8.RuneCountInString(body) > maxBody {
		return "", ErrTooLong
	}
	return body, nil
}

// AskSeller starts (or continues) a buyer's conversation about a listing.
// It returns the thread ID.
func (s *Service) AskSeller(ctx context.Context, buyerID, buyerName, listingID, body string) (string, error) {
	body, err := clean(body)
	if err != nil {
		return "", err
	}
	l, err := s.Listings.GetPublic(ctx, listingID)
	if errors.Is(err, listings.ErrNotFound) {
		return "", ErrNotAvailable
	}
	if err != nil {
		return "", err
	}
	if l.SellerID == buyerID {
		return "", ErrOwnListing
	}
	var threadID string
	err = s.DB.QueryRowContext(ctx, `
		INSERT INTO message_threads (listing_id, buyer_id, seller_id) VALUES ($1, $2, $3)
		ON CONFLICT (listing_id, buyer_id) DO UPDATE SET listing_id = EXCLUDED.listing_id
		RETURNING id`, listingID, buyerID, l.SellerID).Scan(&threadID)
	if err != nil {
		return "", err
	}
	return threadID, s.post(ctx, threadID, buyerID, buyerName, l.SellerID, l.Title, body)
}

// Reply adds a message to a thread the user is part of.
func (s *Service) Reply(ctx context.Context, userID, userName, threadID, body string) error {
	body, err := clean(body)
	if err != nil {
		return err
	}
	t, err := s.thread(ctx, userID, threadID)
	if err != nil {
		return err
	}
	other := t.BuyerID
	if userID == t.BuyerID {
		other = t.SellerID
	}
	return s.post(ctx, threadID, userID, userName, other, t.ListingTitle, body)
}

func (s *Service) post(ctx context.Context, threadID, senderID, senderName, recipientID, listingTitle, body string) error {
	reason := screen(body)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Blocked messages are kept for review but never delivered.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO messages (thread_id, sender_id, body, blocked) VALUES ($1, $2, $3, NULLIF($4, ''))`,
		threadID, senderID, body, reason); err != nil {
		return err
	}
	if reason != "" {
		if err := tx.Commit(); err != nil {
			return err
		}
		return ErrOffPlatform
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE message_threads SET last_message_at = now() WHERE id = $1`, threadID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO thread_reads (thread_id, user_id, last_read_at) VALUES ($1, $2, now())
		ON CONFLICT (thread_id, user_id) DO UPDATE SET last_read_at = now()`, threadID, senderID); err != nil {
		return err
	}
	preview := body
	if r := []rune(preview); len(r) > 120 {
		preview = string(r[:119]) + "…"
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO notifications (user_id, kind, title, body, link) VALUES ($1, 'message', $2, $3, $4)`,
		recipientID, fmt.Sprintf("%s sent you a message about %s", senderName, listingTitle), preview,
		"/messages/"+threadID); err != nil {
		return err
	}
	return tx.Commit()
}

type Thread struct {
	ID            string    `json:"id"`
	ListingID     *string   `json:"listing_id"`
	ListingTitle  string    `json:"listing_title"`
	ListingPhoto  string    `json:"listing_photo"`
	BuyerID       string    `json:"-"`
	SellerID      string    `json:"-"`
	OtherUsername string    `json:"other_username"`
	Role          string    `json:"role"` // "buyer" or "seller" — the viewer's side
	LastMessage   string    `json:"last_message"`
	LastMessageAt time.Time `json:"last_message_at"`
	Unread        bool      `json:"unread"`
}

const threadSelect = `
	SELECT t.id, t.listing_id, coalesce(l.title, 'Removed listing'), coalesce(l.photo_urls[1], ''),
	       t.buyer_id, t.seller_id,
	       CASE WHEN t.buyer_id = $1 THEN su.username ELSE bu.username END,
	       CASE WHEN t.buyer_id = $1 THEN 'buyer' ELSE 'seller' END,
	       coalesce((SELECT m.body FROM messages m WHERE m.thread_id = t.id AND m.blocked IS NULL ORDER BY m.id DESC LIMIT 1), ''),
	       t.last_message_at,
	       EXISTS (SELECT 1 FROM messages m WHERE m.thread_id = t.id AND m.blocked IS NULL AND m.sender_id <> $1
	               AND m.created_at > coalesce((SELECT r.last_read_at FROM thread_reads r WHERE r.thread_id = t.id AND r.user_id = $1), 'epoch'))
	FROM message_threads t
	JOIN users bu ON bu.id = t.buyer_id
	JOIN users su ON su.id = t.seller_id
	LEFT JOIN listings l ON l.id = t.listing_id`

func scanThread(row interface{ Scan(...any) error }) (Thread, error) {
	var t Thread
	var lid sql.NullString
	err := row.Scan(&t.ID, &lid, &t.ListingTitle, &t.ListingPhoto, &t.BuyerID, &t.SellerID,
		&t.OtherUsername, &t.Role, &t.LastMessage, &t.LastMessageAt, &t.Unread)
	if lid.Valid {
		t.ListingID = &lid.String
	}
	return t, err
}

// Threads lists the user's conversations, newest activity first.
func (s *Service) Threads(ctx context.Context, userID string) ([]Thread, error) {
	rows, err := s.DB.QueryContext(ctx, threadSelect+`
		WHERE (t.buyer_id = $1 OR t.seller_id = $1)
		  AND EXISTS (SELECT 1 FROM messages m WHERE m.thread_id = t.id AND m.blocked IS NULL)
		ORDER BY t.last_message_at DESC LIMIT 100`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Thread{}
	for rows.Next() {
		t, err := scanThread(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Service) thread(ctx context.Context, userID, threadID string) (Thread, error) {
	t, err := scanThread(s.DB.QueryRowContext(ctx, threadSelect+`
		WHERE t.id = $2 AND (t.buyer_id = $1 OR t.seller_id = $1)`, userID, threadID))
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

type Message struct {
	ID        int64     `json:"id"`
	Mine      bool      `json:"mine"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// Conversation returns a thread and its messages, and marks it read.
func (s *Service) Conversation(ctx context.Context, userID, threadID string) (Thread, []Message, error) {
	t, err := s.thread(ctx, userID, threadID)
	if err != nil {
		return t, nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, sender_id = $2, body, created_at FROM messages
		WHERE thread_id = $1 AND blocked IS NULL
		ORDER BY id`, threadID, userID)
	if err != nil {
		return t, nil, err
	}
	defer rows.Close()
	msgs := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Mine, &m.Body, &m.CreatedAt); err != nil {
			return t, nil, err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return t, nil, err
	}
	if _, err := s.DB.ExecContext(ctx, `
		INSERT INTO thread_reads (thread_id, user_id, last_read_at) VALUES ($1, $2, now())
		ON CONFLICT (thread_id, user_id) DO UPDATE SET last_read_at = now()`, threadID, userID); err != nil {
		return t, nil, err
	}
	// Reading the conversation also clears its bell notices.
	if _, err := s.DB.ExecContext(ctx, `
		UPDATE notifications SET read_at = now()
		WHERE user_id = $1 AND link = $2 AND read_at IS NULL`, userID, "/messages/"+threadID); err != nil {
		return t, nil, err
	}
	t.Unread = false
	return t, msgs, nil
}

// ThreadFor returns the buyer's existing thread about a listing, if any.
func (s *Service) ThreadFor(ctx context.Context, buyerID, listingID string) (string, error) {
	var id string
	err := s.DB.QueryRowContext(ctx,
		`SELECT id FROM message_threads WHERE buyer_id = $1 AND listing_id = $2`, buyerID, listingID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// ---- Bell: notifications + announcements ----

type Notice struct {
	ID        string    `json:"id"` // "n123" for a notification, "a45" for an announcement
	Kind      string    `json:"kind"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Link      *string   `json:"link"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}

type Bell struct {
	Unread  int      `json:"unread"`
	Notices []Notice `json:"notices"`
}

// BellFor returns the newest notices for a user, announcements included.
// Announcements posted before the user joined are not shown.
func (s *Service) BellFor(ctx context.Context, userID string, limit int) (Bell, error) {
	b := Bell{Notices: []Notice{}}
	if err := s.DB.QueryRowContext(ctx, `
		SELECT
		  (SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL) +
		  (SELECT count(*) FROM announcements a, users u
		    WHERE u.id = $1 AND a.created_at >= u.created_at - interval '30 days'
		      AND NOT EXISTS (SELECT 1 FROM announcement_reads r WHERE r.announcement_id = a.id AND r.user_id = $1))`,
		userID).Scan(&b.Unread); err != nil {
		return b, err
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT 'n' || id, kind, title, body, link, read_at IS NOT NULL, created_at
		FROM notifications WHERE user_id = $1
		UNION ALL
		SELECT 'a' || a.id, 'announcement', a.title, a.body, a.link,
		       EXISTS (SELECT 1 FROM announcement_reads r WHERE r.announcement_id = a.id AND r.user_id = $1),
		       a.created_at
		FROM announcements a, users u
		WHERE u.id = $1 AND a.created_at >= u.created_at - interval '30 days'
		ORDER BY 7 DESC LIMIT $2`, userID, limit)
	if err != nil {
		return b, err
	}
	defer rows.Close()
	for rows.Next() {
		var n Notice
		var link sql.NullString
		if err := rows.Scan(&n.ID, &n.Kind, &n.Title, &n.Body, &link, &n.Read, &n.CreatedAt); err != nil {
			return b, err
		}
		if link.Valid {
			n.Link = &link.String
		}
		b.Notices = append(b.Notices, n)
	}
	return b, rows.Err()
}

// MarkRead marks one bell notice read. id is as BellFor returns it:
// "n<id>" for a notification, "a<id>" for an announcement.
func (s *Service) MarkRead(ctx context.Context, userID, id string) error {
	if len(id) < 2 {
		return ErrNotFound
	}
	n, err := strconv.ParseInt(id[1:], 10, 64)
	if err != nil || n <= 0 {
		return ErrNotFound
	}
	switch id[0] {
	case 'n':
		_, err = s.DB.ExecContext(ctx, `UPDATE notifications SET read_at = coalesce(read_at, now()) WHERE id = $1 AND user_id = $2`, n, userID)
	case 'a':
		_, err = s.DB.ExecContext(ctx, `INSERT INTO announcement_reads (announcement_id, user_id)
			SELECT id, $2 FROM announcements WHERE id = $1 ON CONFLICT DO NOTHING`, n, userID)
	default:
		return ErrNotFound
	}
	return err
}

// MarkLinkRead marks read the user's notifications that point at a page,
// so opening an order (however you got there) clears its alerts.
func (s *Service) MarkLinkRead(ctx context.Context, userID string, links ...string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE notifications SET read_at = now()
		WHERE user_id = $1 AND read_at IS NULL AND link = ANY($2)`, userID, pq.Array(links))
	return err
}

// MarkAllRead clears the bell.
func (s *Service) MarkAllRead(ctx context.Context, userID string) error {
	if _, err := s.DB.ExecContext(ctx,
		`UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL`, userID); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO announcement_reads (announcement_id, user_id)
		SELECT a.id, $1 FROM announcements a ON CONFLICT DO NOTHING`, userID)
	return err
}

// UnreadMessages counts conversations with messages the user hasn't read.
func (s *Service) UnreadMessages(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `
		SELECT count(*) FROM message_threads t
		WHERE (t.buyer_id = $1 OR t.seller_id = $1)
		  AND EXISTS (SELECT 1 FROM messages m WHERE m.thread_id = t.id AND m.blocked IS NULL AND m.sender_id <> $1
		      AND m.created_at > coalesce((SELECT r.last_read_at FROM thread_reads r WHERE r.thread_id = t.id AND r.user_id = $1), 'epoch'))`,
		userID).Scan(&n)
	return n, err
}

// Announce posts a site-wide announcement (admins only; checked by the caller).
func (s *Service) Announce(ctx context.Context, adminID, title, body, link string) (int64, error) {
	title, body, link = strings.TrimSpace(title), strings.TrimSpace(body), strings.TrimSpace(link)
	if title == "" || utf8.RuneCountInString(title) > 120 {
		return 0, errors.New("title is required, up to 120 characters")
	}
	if utf8.RuneCountInString(body) > 1000 {
		return 0, errors.New("body can be up to 1,000 characters")
	}
	if link != "" && !strings.HasPrefix(link, "/") && !strings.HasPrefix(link, "https://") {
		return 0, errors.New("link must start with / or https://")
	}
	var id int64
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO announcements (title, body, link, created_by) VALUES ($1, $2, NULLIF($3, ''), $4) RETURNING id`,
		title, body, link, adminID).Scan(&id)
	return id, err
}

type Announcement struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Link      *string   `json:"link"`
	CreatedAt time.Time `json:"created_at"`
}

// Announcements lists recent announcements for the public news page.
func (s *Service) Announcements(ctx context.Context, limit int) ([]Announcement, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, title, body, link, created_at FROM announcements ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Announcement{}
	for rows.Next() {
		var a Announcement
		var link sql.NullString
		if err := rows.Scan(&a.ID, &a.Title, &a.Body, &link, &a.CreatedAt); err != nil {
			return nil, err
		}
		if link.Valid {
			a.Link = &link.String
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// MessageAboutOrder starts (or continues) the conversation for an order,
// from either side, even after the card has sold. It returns the thread ID.
func (s *Service) MessageAboutOrder(ctx context.Context, userID, userName, orderID, body string) (string, error) {
	body, err := clean(body)
	if err != nil {
		return "", err
	}
	var buyer, seller, listingID, title string
	err = s.DB.QueryRowContext(ctx, `SELECT o.buyer_id, o.seller_id, oi.listing_id, oi.title FROM orders o
		JOIN order_items oi ON oi.order_id = o.id
		WHERE (o.id::text = $1 OR o.number::text = $1) AND (o.buyer_id = $2 OR o.seller_id = $2)
		  AND o.paid_at IS NOT NULL AND oi.listing_id IS NOT NULL
		ORDER BY oi.id LIMIT 1`, orderID, userID).Scan(&buyer, &seller, &listingID, &title)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotAvailable
	}
	if err != nil {
		return "", err
	}
	var threadID string
	if err := s.DB.QueryRowContext(ctx, `
		INSERT INTO message_threads (listing_id, buyer_id, seller_id) VALUES ($1, $2, $3)
		ON CONFLICT (listing_id, buyer_id) DO UPDATE SET listing_id = EXCLUDED.listing_id
		RETURNING id`, listingID, buyer, seller).Scan(&threadID); err != nil {
		return "", err
	}
	to := seller
	if userID == seller {
		to = buyer
	}
	return threadID, s.post(ctx, threadID, userID, userName, to, title, body)
}
