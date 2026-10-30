package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jconner5017-rgb/marketplace/internal/inbox"
	"github.com/jconner5017-rgb/marketplace/internal/shopping"
)

func (s *Server) accountError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, shopping.ErrNotAvailable), errors.Is(err, inbox.ErrNotAvailable):
		writeError(w, http.StatusNotFound, "not_available", "this card is no longer available", nil)
	case errors.Is(err, shopping.ErrOwnListing), errors.Is(err, inbox.ErrOwnListing):
		writeError(w, http.StatusUnprocessableEntity, "own_listing", "that's your own listing", nil)
	case errors.Is(err, shopping.ErrTooMany):
		writeError(w, http.StatusUnprocessableEntity, "not_enough", "the seller doesn't have that many", map[string]string{"quantity": "more than available"})
	case errors.Is(err, shopping.ErrAuctionCart):
		writeError(w, http.StatusUnprocessableEntity, "auction", err.Error(), nil)
	case errors.Is(err, shopping.ErrCartFull):
		writeError(w, http.StatusUnprocessableEntity, "cart_full", "your cart holds up to 50 different cards", nil)
	case errors.Is(err, inbox.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "conversation not found", nil)
	case errors.Is(err, inbox.ErrOffPlatform):
		writeError(w, http.StatusUnprocessableEntity, "off_platform", err.Error(), map[string]string{"body": "remove contact or payment details"})
	case errors.Is(err, inbox.ErrEmpty), errors.Is(err, inbox.ErrTooLong):
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", err.Error(), map[string]string{"body": err.Error()})
	default:
		s.internalError(w, r, err)
	}
}

// GET /v1/me/summary — everything the header needs in one call.
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	counts, err := s.shopping.Counts(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	bell, err := s.inbox.BellFor(r.Context(), u.ID, 1)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	unreadMsgs, err := s.inbox.UnreadMessages(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	_, sellerErr := s.sellers.Get(r.Context(), u.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"user":            u,
		"is_seller":       sellerErr == nil,
		"cart_cards":      counts.CartCards,
		"watching":        counts.Watching,
		"unread_notices":  bell.Unread,
		"unread_messages": unreadMsgs,
	})
}

// ---- Watchlist ----

func (s *Server) handleWatchlist(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	items, err := s.shopping.Watchlist(r.Context(), u.ID)
	if err != nil {
		s.accountError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleWatch(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := listingID(w, r)
	if !ok {
		return
	}
	if err := s.shopping.Watch(r.Context(), u.ID, id); err != nil {
		s.accountError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"watching": true})
}

func (s *Server) handleUnwatch(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := listingID(w, r)
	if !ok {
		return
	}
	if err := s.shopping.Unwatch(r.Context(), u.ID, id); err != nil {
		s.accountError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"watching": false})
}

// GET /v1/listings/{id}/me — the viewer's relationship to a listing.
func (s *Server) handleListingForMe(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := listingID(w, r)
	if !ok {
		return
	}
	watching, err := s.shopping.IsWatching(r.Context(), u.ID, id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	thread, err := s.inbox.ThreadFor(r.Context(), u.ID, id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	var inCart int
	s.db.QueryRowContext(r.Context(),
		`SELECT coalesce(max(quantity), 0) FROM cart_items WHERE user_id = $1 AND listing_id = $2`, u.ID, id).Scan(&inCart)
	var own bool
	s.db.QueryRowContext(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM listings WHERE id = $1 AND seller_id = $2)`, id, u.ID).Scan(&own)
	// Best Offer: how many offers this buyer has left and any open one.
	var made int
	s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM offers
		WHERE listing_id = $1 AND buyer_id = $2 AND from_role = 'buyer'`, id, u.ID).Scan(&made)
	var pending *string
	var pid string
	if err := s.db.QueryRowContext(r.Context(), `SELECT id FROM offers
		WHERE listing_id = $1 AND buyer_id = $2 AND status = 'pending' ORDER BY created_at DESC LIMIT 1`,
		id, u.ID).Scan(&pid); err == nil {
		pending = &pid
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"watching": watching, "in_cart": inCart, "own": own, "thread_id": thread,
		"offers_left": max(0, 3-made), "pending_offer_id": pending,
	})
}

// ---- Cart ----

func (s *Server) handleCart(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	c, err := s.shopping.GetCart(r.Context(), u.ID)
	if err != nil {
		s.accountError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

type cartRequest struct {
	Quantity int `json:"quantity"`
}

func (s *Server) handleSetCart(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := listingID(w, r)
	if !ok {
		return
	}
	req := cartRequest{Quantity: 1}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.shopping.SetCartQuantity(r.Context(), u.ID, id, req.Quantity); err != nil {
		s.accountError(w, r, err)
		return
	}
	s.handleCart(w, r)
}

func (s *Server) handleRemoveCart(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := listingID(w, r)
	if !ok {
		return
	}
	if err := s.shopping.RemoveFromCart(r.Context(), u.ID, id); err != nil {
		s.accountError(w, r, err)
		return
	}
	s.handleCart(w, r)
}

// ---- Messages ----

type messageRequest struct {
	Body string `json:"body"`
}

// POST /v1/listings/{id}/messages — a buyer asks the seller a question.
func (s *Server) handleAskSeller(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := listingID(w, r)
	if !ok {
		return
	}
	var req messageRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !s.msgLimiter.allow(u.ID) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "you're sending messages too quickly; wait a few minutes", nil)
		return
	}
	threadID, err := s.inbox.AskSeller(r.Context(), u.ID, u.Username, id, req.Body)
	if err != nil {
		s.accountError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"thread_id": threadID})
}

// POST /v1/orders/{id}/messages {"body"} — message the other side of an order.
func (s *Server) handleOrderMessage(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := orderParam(w, r)
	if !ok {
		return
	}
	var req messageRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !s.msgLimiter.allow(u.ID) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "you're sending messages too quickly; wait a few minutes", nil)
		return
	}
	threadID, err := s.inbox.MessageAboutOrder(r.Context(), u.ID, u.Username, id, req.Body)
	if err != nil {
		s.accountError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"thread_id": threadID})
}

func (s *Server) handleThreads(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	ts, err := s.inbox.Threads(r.Context(), u.ID)
	if err != nil {
		s.accountError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"threads": ts})
}

func threadID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !isUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", "conversation not found", nil)
		return "", false
	}
	return id, true
}

func (s *Server) handleConversation(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := threadID(w, r)
	if !ok {
		return
	}
	t, msgs, err := s.inbox.Conversation(r.Context(), u.ID, id)
	if err != nil {
		s.accountError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"thread": t, "messages": msgs})
}

func (s *Server) handleReply(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := threadID(w, r)
	if !ok {
		return
	}
	var req messageRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !s.msgLimiter.allow(u.ID) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "you're sending messages too quickly; wait a few minutes", nil)
		return
	}
	if err := s.inbox.Reply(r.Context(), u.ID, u.Username, id, req.Body); err != nil {
		s.accountError(w, r, err)
		return
	}
	s.handleConversation(w, r)
}

// ---- Bell ----

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	b, err := s.inbox.BellFor(r.Context(), u.ID, 30)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// POST /v1/notifications/{id}/read — one notice, when it's clicked.
func (s *Server) handleReadOne(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	if err := s.inbox.MarkRead(r.Context(), u.ID, r.PathValue("id")); err != nil {
		if errors.Is(err, inbox.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "notification not found", nil)
			return
		}
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReadAll(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	if err := s.inbox.MarkAllRead(r.Context(), u.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type announceRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Link  string `json:"link"`
}

// POST /v1/admin/announcements — shows in every member's bell.
func (s *Server) handleAnnounce(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var req announceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	id, err := s.inbox.Announce(r.Context(), u.ID, req.Title, req.Body, req.Link)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "required") || strings.Contains(msg, "up to") || strings.Contains(msg, "must") {
			writeError(w, http.StatusUnprocessableEntity, "invalid_input", msg, nil)
			return
		}
		s.internalError(w, r, err)
		return
	}
	s.audit(r, "announcement.posted", "announcement", strconv.FormatInt(id, 10), "", map[string]any{"title": req.Title})
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

// GET /v1/announcements — public news list.
func (s *Server) handleAnnouncements(w http.ResponseWriter, r *http.Request) {
	list, err := s.inbox.Announcements(r.Context(), 50)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"announcements": list})
}
