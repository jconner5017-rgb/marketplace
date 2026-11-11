// Package email sends the site's mail through Postmark, Resend or Mailjet: account
// emails (confirm your email, password reset) and staff email blasts. Every
// message goes through an outbox table so a slow or failing provider never
// holds up a request, and blasts go out in batches.
package email

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

// Message is one email.
type Message struct {
	To, Subject, Text string
	Broadcast         bool              // a blast (marketing stream), not an account email
	Headers           map[string]string // e.g. List-Unsubscribe
}

// Sender delivers one message.
type Sender interface {
	Send(ctx context.Context, from string, m Message) error
	Name() string
}

var httpClient = &http.Client{Timeout: 20 * time.Second}

func post(ctx context.Context, url string, hdr map[string]string, body any) error {
	_, err := postRead(ctx, url, hdr, body)
	return err
}

// postRead is post that also returns the response body.
func postRead(ctx context.Context, url string, hdr map[string]string, body any) ([]byte, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 20000))
	if res.StatusCode >= 300 {
		return data, fmt.Errorf("HTTP %d: %s", res.StatusCode, trim(strings.TrimSpace(string(data)), 2000))
	}
	return data, nil
}

// Postmark sends with a server API token. Blasts use its "broadcast"
// message stream, kept apart from account email as Postmark requires.
type Postmark struct{ Token string }

func (p Postmark) Name() string { return "postmark" }

func (p Postmark) Send(ctx context.Context, from string, m Message) error {
	stream := "outbound"
	if m.Broadcast {
		stream = "broadcast"
	}
	var hs []map[string]string
	for k, v := range m.Headers {
		hs = append(hs, map[string]string{"Name": k, "Value": v})
	}
	return post(ctx, "https://api.postmarkapp.com/email", map[string]string{"X-Postmark-Server-Token": p.Token},
		map[string]any{"From": from, "To": m.To, "Subject": m.Subject, "TextBody": m.Text, "MessageStream": stream, "Headers": hs})
}

// Resend sends with an API key.
type Resend struct{ Key string }

func (r Resend) Name() string { return "resend" }

func (r Resend) Send(ctx context.Context, from string, m Message) error {
	body := map[string]any{"from": from, "to": []string{m.To}, "subject": m.Subject, "text": m.Text}
	if len(m.Headers) > 0 {
		body["headers"] = m.Headers
	}
	return post(ctx, "https://api.resend.com/emails", map[string]string{"Authorization": "Bearer " + r.Key}, body)
}

// Mailjet sends with an API key and secret key (Send API v3.1).
type Mailjet struct{ Key, Secret string }

func (j Mailjet) Name() string { return "mailjet" }

func (j Mailjet) Send(ctx context.Context, from string, m Message) error {
	addr, err := mail.ParseAddress(from)
	if err != nil {
		return fmt.Errorf("EMAIL_FROM: %w", err)
	}
	msg := map[string]any{
		"From":     map[string]string{"Email": addr.Address, "Name": addr.Name},
		"To":       []map[string]string{{"Email": m.To}},
		"Subject":  m.Subject,
		"TextPart": m.Text,
	}
	if len(m.Headers) > 0 {
		msg["Headers"] = m.Headers
	}
	auth := base64.StdEncoding.EncodeToString([]byte(j.Key + ":" + j.Secret))
	data, err := postRead(ctx, "https://api.mailjet.com/v3.1/send", map[string]string{"Authorization": "Basic " + auth},
		map[string]any{"Messages": []any{msg}})
	if err != nil {
		return err
	}
	// Mailjet answers per message; anything but "success" is a failure.
	var r struct {
		Messages []struct {
			Status string
			Errors []struct{ ErrorMessage, ErrorCode string }
		}
	}
	if json.Unmarshal(data, &r) == nil && len(r.Messages) > 0 && r.Messages[0].Status != "success" {
		var why []string
		for _, e := range r.Messages[0].Errors {
			why = append(why, e.ErrorCode+" "+e.ErrorMessage)
		}
		return fmt.Errorf("mailjet: %s %s", r.Messages[0].Status, strings.Join(why, "; "))
	}
	return nil
}

// Pick builds a sender from EMAIL_PROVIDER, EMAIL_API_KEY and (Mailjet)
// EMAIL_API_SECRET; nil when unset.
func Pick(provider, key, secret string) (Sender, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "none":
		return nil, nil
	case "postmark":
		if key == "" {
			return nil, errors.New("EMAIL_PROVIDER=postmark needs EMAIL_API_KEY (a Postmark server token)")
		}
		return Postmark{Token: key}, nil
	case "resend":
		if key == "" {
			return nil, errors.New("EMAIL_PROVIDER=resend needs EMAIL_API_KEY")
		}
		return Resend{Key: key}, nil
	case "mailjet":
		if key == "" || secret == "" {
			return nil, errors.New("EMAIL_PROVIDER=mailjet needs EMAIL_API_KEY and EMAIL_API_SECRET")
		}
		return Mailjet{Key: key, Secret: secret}, nil
	}
	return nil, errors.New("EMAIL_PROVIDER must be postmark, resend or mailjet")
}

// Service queues and sends mail.
type Service struct {
	DB     *sql.DB
	Sender Sender                        // nil: nothing is sent and email confirmation isn't required
	From   string                        // e.g. "Slabs & Singles <hello@slabsnsingles.com>"
	WebURL string                        // https://slabsnsingles.com
	Postal string                        // mailing address printed on blasts (required by US law)
	Secret []byte                        // signs unsubscribe links
	Pace   time.Duration                 // wait between sends (Resend allows about 2 a second)
	Warn   func(msg string, args ...any) // logs failed sends (optional)
	Now    func() time.Time
}

// Enabled reports whether mail actually goes out.
func (s *Service) Enabled() bool { return s != nil && s.Sender != nil }

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

type execer interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
}

// Queue adds an account email to the outbox.
func (s *Service) Queue(ctx context.Context, q execer, userID, to, subject, text string) error {
	// .invalid addresses (internal accounts like the test buyer) can never be delivered.
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(to)), ".invalid") {
		return nil
	}
	_, err := q.ExecContext(ctx, `INSERT INTO email_outbox (user_id, to_email, kind, subject, body) VALUES (NULLIF($1, '')::uuid, $2, 'transactional', $3, $4)`,
		userID, to, subject, text)
	return err
}

// RunOutbox sends up to batch queued messages. Failures retry with a
// growing delay, five tries in all.
func (s *Service) RunOutbox(ctx context.Context, batch int) (sent int, err error) {
	if !s.Enabled() {
		return 0, nil
	}
	type job struct {
		id                              int64
		userID, to, kind, subject, body string
		attempts                        int
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id, coalesce(user_id::text, ''), to_email, kind, subject, body, attempts FROM email_outbox
		WHERE status = 'queued' AND send_after <= now() ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED`, batch)
	if err != nil {
		return 0, err
	}
	var jobs []job
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.id, &j.userID, &j.to, &j.kind, &j.subject, &j.body, &j.attempts); err != nil {
			rows.Close()
			return 0, err
		}
		jobs = append(jobs, j)
	}
	rows.Close()
	for _, j := range jobs {
		m := Message{To: j.to, Subject: j.subject, Text: j.body, Broadcast: j.kind == "broadcast"}
		if m.Broadcast {
			if j.userID == "" {
				continue
			}
			link := s.UnsubscribeURL(j.userID)
			m.Text += "\n\n--\nYou're getting this because you have a Slabs & Singles account.\nUnsubscribe from announcements: " + link + "\n" + s.Postal
			m.Headers = map[string]string{"List-Unsubscribe": "<" + s.unsubscribeAPI(j.userID) + ">", "List-Unsubscribe-Post": "List-Unsubscribe=One-Click"}
		}
		if s.Pace > 0 {
			time.Sleep(s.Pace)
		}
		if serr := s.Sender.Send(ctx, s.From, m); serr != nil {
			if s.Warn != nil {
				s.Warn("email not sent", "to", j.to, "subject", j.subject, "try", j.attempts+1, "err", serr)
			}
			status := "queued"
			if j.attempts+1 >= 5 {
				status = "failed"
			}
			if _, err := tx.ExecContext(ctx, `UPDATE email_outbox SET attempts = attempts + 1, last_error = $2, status = $3,
				send_after = now() + make_interval(mins => 5 * (attempts + 1)) WHERE id = $1`, j.id, trim(serr.Error(), 500), status); err != nil {
				return sent, err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE email_outbox SET status = 'sent', sent_at = now(), attempts = attempts + 1, last_error = NULL WHERE id = $1`, j.id); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, tx.Commit()
}

func trim(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// ---- Tokens for confirming an email and resetting a password ----

var ErrToken = errors.New("that link has expired or was already used")

func hashToken(t string) []byte {
	h := sha256.Sum256([]byte(t))
	return h[:]
}

// NewToken stores a single-use token for a user and returns it.
func (s *Service) NewToken(ctx context.Context, q execer, userID, email, purpose string, ttl time.Duration) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	t := base64.RawURLEncoding.EncodeToString(b)
	_, err := q.ExecContext(ctx, `INSERT INTO email_tokens (token_hash, user_id, purpose, email, expires_at) VALUES ($1, $2, $3, $4, $5)`,
		hashToken(t), userID, purpose, email, s.now().Add(ttl))
	return t, err
}

// UseToken spends a token and returns whose it was and the email it was
// sent to.
func (s *Service) UseToken(ctx context.Context, tx *sql.Tx, token, purpose string) (userID, email string, err error) {
	err = tx.QueryRowContext(ctx, `UPDATE email_tokens SET used_at = now()
		WHERE token_hash = $1 AND purpose = $2 AND used_at IS NULL AND expires_at > $3
		RETURNING user_id, email`, hashToken(token), purpose, s.now()).Scan(&userID, &email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrToken
	}
	return userID, email, err
}

// RecentTokens counts tokens of a kind made for a user in the last window,
// to limit resends.
func (s *Service) RecentTokens(ctx context.Context, userID, purpose string, window time.Duration) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM email_tokens WHERE user_id = $1 AND purpose = $2 AND created_at > $3`,
		userID, purpose, s.now().Add(-window)).Scan(&n)
	return n, err
}

// ---- Unsubscribe links for blasts ----

func (s *Service) sign(userID string) string {
	m := hmac.New(sha256.New, s.Secret)
	m.Write([]byte("unsubscribe:" + userID))
	return hex.EncodeToString(m.Sum(nil))[:32]
}

// UnsubscribeURL is the page a member opens from a blast.
func (s *Service) UnsubscribeURL(userID string) string {
	return s.WebURL + "/unsubscribe?u=" + userID + "&t=" + s.sign(userID)
}

func (s *Service) unsubscribeAPI(userID string) string {
	return s.WebURL + "/api/v1/email/unsubscribe?u=" + userID + "&t=" + s.sign(userID)
}

// Unsubscribe turns off blasts for a member, given a signed link.
func (s *Service) Unsubscribe(ctx context.Context, userID, sig string) error {
	if len(sig) != 32 || !hmac.Equal([]byte(sig), []byte(s.sign(userID))) {
		return ErrToken
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE users SET email_opt_out = true WHERE id::text = $1`, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrToken
	}
	return nil
}

// ---- Blasts ----

// Audiences a blast can go to.
var Audiences = map[string]string{
	"all":     "Everyone",
	"sellers": "Sellers who can take payments",
}

// Broadcast is one blast.
type Broadcast struct {
	ID         int64     `json:"id"`
	Subject    string    `json:"subject"`
	Audience   string    `json:"audience"`
	Recipients int       `json:"recipients"`
	Sent       int       `json:"sent"`
	Failed     int       `json:"failed"`
	Queued     int       `json:"queued"`
	By         string    `json:"by"`
	CreatedAt  time.Time `json:"created_at"`
}

// FieldError names a bad field.
type FieldError struct{ Field, Msg string }

func (e *FieldError) Error() string { return e.Msg }

func checkBlast(subject, body, audience string) error {
	switch {
	case strings.TrimSpace(subject) == "" || len(subject) > 150:
		return &FieldError{"subject", "enter a subject (up to 150 characters)"}
	case strings.TrimSpace(body) == "" || len(body) > 20000:
		return &FieldError{"body", "enter the message (up to 20,000 characters)"}
	case Audiences[audience] == "":
		return &FieldError{"audience", "choose who gets it"}
	}
	return nil
}

// recipientsSQL: active members with a confirmed (or pre-verification)
// email who haven't unsubscribed.
func recipientsSQL(audience string) string {
	q := `SELECT u.id::text, u.email FROM users u WHERE u.status = 'active' AND NOT u.email_opt_out
		AND (u.email_verified_at IS NOT NULL OR NOT u.verify_required)`
	if audience == "sellers" {
		q += ` AND EXISTS (SELECT 1 FROM seller_profiles sp WHERE sp.user_id = u.id AND sp.charges_enabled)`
	}
	return q
}

// CountRecipients is how many members a blast would reach.
func (s *Service) CountRecipients(ctx context.Context, audience string) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM (`+recipientsSQL(audience)+`) r`).Scan(&n)
	return n, err
}

func (s *Service) blastReady() error {
	if !s.Enabled() {
		return &FieldError{"subject", "email isn't set up yet: add EMAIL_PROVIDER, EMAIL_API_KEY and EMAIL_FROM in Render"}
	}
	if strings.TrimSpace(s.Postal) == "" {
		return &FieldError{"subject", "add EMAIL_POSTAL_ADDRESS in Render first: US law requires a mailing address on marketing email"}
	}
	return nil
}

// TestBlast sends the blast to one address (the staff member's own).
func (s *Service) TestBlast(ctx context.Context, userID, to, subject, body string) error {
	if err := s.blastReady(); err != nil {
		return err
	}
	if err := checkBlast(subject, body, "all"); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO email_outbox (user_id, to_email, kind, subject, body) VALUES ($1, $2, 'broadcast', $3, $4)`,
		userID, to, "[Test] "+subject, body)
	return err
}

// SendBlast queues a blast to an audience and returns how many it reaches.
func (s *Service) SendBlast(ctx context.Context, byUserID, subject, body, audience string) (int64, int, error) {
	if err := s.blastReady(); err != nil {
		return 0, 0, err
	}
	if err := checkBlast(subject, body, audience); err != nil {
		return 0, 0, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO email_broadcasts (subject, body, audience, created_by) VALUES ($1, $2, $3, $4) RETURNING id`,
		subject, body, audience, byUserID).Scan(&id); err != nil {
		return 0, 0, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO email_outbox (user_id, to_email, kind, subject, body, broadcast_id)
		SELECT r.id::uuid, r.email, 'broadcast', $1, $2, $3 FROM (`+recipientsSQL(audience)+`) r`, subject, body, id)
	if err != nil {
		return 0, 0, err
	}
	n, _ := res.RowsAffected()
	if _, err := tx.ExecContext(ctx, `UPDATE email_broadcasts SET recipients = $2 WHERE id = $1`, id, n); err != nil {
		return 0, 0, err
	}
	return id, int(n), tx.Commit()
}

// Broadcasts lists recent blasts with delivery counts.
func (s *Service) Broadcasts(ctx context.Context, limit int) ([]Broadcast, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT b.id, b.subject, b.audience, b.recipients, coalesce(u.username, ''), b.created_at,
		count(o.id) FILTER (WHERE o.status = 'sent'), count(o.id) FILTER (WHERE o.status = 'failed'), count(o.id) FILTER (WHERE o.status = 'queued')
		FROM email_broadcasts b LEFT JOIN users u ON u.id = b.created_by LEFT JOIN email_outbox o ON o.broadcast_id = b.id
		GROUP BY b.id, u.username ORDER BY b.id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Broadcast{}
	for rows.Next() {
		var b Broadcast
		if err := rows.Scan(&b.ID, &b.Subject, &b.Audience, &b.Recipients, &b.By, &b.CreatedAt, &b.Sent, &b.Failed, &b.Queued); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
