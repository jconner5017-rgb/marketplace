package fcm

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// SafeClient refuses to connect to private, loopback and link-local
// addresses, so a seller can't point a webhook at our own network.
func SafeClient() *http.Client {
	d := &net.Dialer{Timeout: 5 * time.Second, Control: func(network, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip := net.ParseIP(host)
		if ip == nil || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() ||
			ip.IsLinkLocalMulticast() || ip.IsMulticast() {
			return fmt.Errorf("refusing to connect to %s", host)
		}
		return nil
	}}
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{DialContext: d.DialContext, TLSHandshakeTimeout: 5 * time.Second, MaxIdleConns: 10},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (s *Service) client() HTTPDoer {
	if s.HTTP != nil {
		return s.HTTP
	}
	return SafeClient()
}

// FieldError is a problem with one input.
type FieldError struct{ Field, Msg string }

func (e *FieldError) Error() string { return e.Msg }

func (s *Service) checkURL(field, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || len(raw) > 500 {
		return "", &FieldError{field, "enter a full URL, like https://app.firecardmanager.com/hooks/marketplace"}
	}
	if u.Scheme != "https" && !(s.AllowPrivate && u.Scheme == "http") {
		return "", &FieldError{field, "the URL must start with https://"}
	}
	if !s.AllowPrivate {
		h := u.Hostname()
		if h == "localhost" || strings.HasSuffix(h, ".internal") || strings.HasSuffix(h, ".local") {
			return "", &FieldError{field, "use a public address"}
		}
		if ip := net.ParseIP(h); ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()) {
			return "", &FieldError{field, "use a public address"}
		}
	}
	return u.String(), nil
}

// Endpoint is a seller's connection settings.
type Endpoint struct {
	WebhookURL   string     `json:"webhook_url"`
	InventoryURL string     `json:"inventory_url"`
	HasSecret    bool       `json:"has_secret"`
	Reconciled   *time.Time `json:"reconciled_at"`
}

func (s *Service) Endpoint(ctx context.Context, userID string) (Endpoint, error) {
	var e Endpoint
	var wh, inv, sec sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT webhook_url, inventory_url, webhook_secret, reconciled_at FROM seller_profiles WHERE user_id = $1`,
		userID).Scan(&wh, &inv, &sec, &e.Reconciled)
	if errors.Is(err, sql.ErrNoRows) {
		return e, ErrNotFound
	}
	e.WebhookURL, e.InventoryURL, e.HasSecret = wh.String, inv.String, sec.Valid
	return e, err
}

// SetEndpoint saves the webhook and inventory URLs. A signing secret is made
// the first time (or when rotate is set) and returned only then.
func (s *Service) SetEndpoint(ctx context.Context, userID, webhookURL, inventoryURL string, rotate bool) (string, error) {
	wh, err := s.checkURL("webhook_url", webhookURL)
	if err != nil {
		return "", err
	}
	inv, err := s.checkURL("inventory_url", inventoryURL)
	if err != nil {
		return "", err
	}
	var cur sql.NullString
	if err := s.DB.QueryRowContext(ctx, `SELECT webhook_secret FROM seller_profiles WHERE user_id = $1`, userID).Scan(&cur); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	secret := ""
	if !cur.Valid || rotate {
		secret = "whsec_ss_" + randomString(40)
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE seller_profiles SET webhook_url = nullif($2, ''), inventory_url = nullif($3, ''),
		webhook_secret = coalesce(nullif($4, ''), webhook_secret), updated_at = now() WHERE user_id = $1`, userID, wh, inv, secret)
	return secret, err
}

// Sign makes the SS-Signature header value: t=<unix>,v1=<hex HMAC-SHA256 of "t.body">.
func Sign(secret string, body []byte, t time.Time) string {
	ts := strconv.FormatInt(t.Unix(), 10)
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(ts + "."))
	m.Write(body)
	return "t=" + ts + ",v1=" + hex.EncodeToString(m.Sum(nil))
}

// Execer runs a statement inside the caller's transaction.
type Execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// Enqueue queues an event for a seller who has a webhook URL. Safe to call
// for sellers without one (nothing is queued).
func Enqueue(ctx context.Context, ex Execer, sellerID, event, eventID string, data any) error {
	body, err := json.Marshal(map[string]any{"id": eventID, "type": event, "created": time.Now().Unix(), "data": data})
	if err != nil {
		return err
	}
	_, err = ex.ExecContext(ctx, `INSERT INTO webhook_deliveries (user_id, event, event_id, payload)
		SELECT user_id, $2, $3, $4 FROM seller_profiles WHERE user_id = $1 AND webhook_url IS NOT NULL
		ON CONFLICT (event_id) DO NOTHING`, sellerID, event, eventID, string(body))
	return err
}

// Delivery is one webhook as the seller sees it.
type Delivery struct {
	ID          int64      `json:"id"`
	Event       string     `json:"event"`
	EventID     string     `json:"event_id"`
	Attempts    int        `json:"attempts"`
	DeliveredAt *time.Time `json:"delivered_at"`
	LastStatus  *int       `json:"last_status"`
	LastError   *string    `json:"last_error"`
	GaveUpAt    *time.Time `json:"gave_up_at"`
	NextAttempt time.Time  `json:"next_attempt_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (s *Service) Deliveries(ctx context.Context, userID string) ([]Delivery, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, event, event_id, attempts, delivered_at, last_status, last_error, gave_up_at,
		next_attempt_at, created_at FROM webhook_deliveries WHERE user_id = $1 ORDER BY id DESC LIMIT 50`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Delivery{}
	for rows.Next() {
		var d Delivery
		if err := rows.Scan(&d.ID, &d.Event, &d.EventID, &d.Attempts, &d.DeliveredAt, &d.LastStatus, &d.LastError, &d.GaveUpAt,
			&d.NextAttempt, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SendTest queues a "ping" event.
func (s *Service) SendTest(ctx context.Context, userID string) error {
	var has bool
	if err := s.DB.QueryRowContext(ctx, `SELECT webhook_url IS NOT NULL FROM seller_profiles WHERE user_id = $1`, userID).Scan(&has); err != nil {
		return err
	}
	if !has {
		return &FieldError{"webhook_url", "save a webhook URL first"}
	}
	return Enqueue(ctx, s.DB, userID, "ping", "evt_ping_"+randomString(16), map[string]string{"message": "Test event from Slabs & Singles"})
}

// Backoff before attempt n (1-based): 30s, 1m, 2m, 4m … capped at 1 hour.
func Backoff(n int) time.Duration {
	d := 30 * time.Second << max(n-1, 0)
	if d > time.Hour || d <= 0 {
		return time.Hour
	}
	return d
}

const (
	alertAfter  = time.Hour
	giveUpAfter = 24 * time.Hour
)

// RunDeliveries sends due webhooks. Failures retry with backoff; after an
// hour the seller and staff are alerted; after a day it gives up.
func (s *Service) RunDeliveries(ctx context.Context) (sent int, err error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT d.id, d.user_id, d.event, d.event_id, d.payload, d.attempts, d.created_at, d.alerted, sp.webhook_url, sp.webhook_secret
		FROM webhook_deliveries d JOIN seller_profiles sp ON sp.user_id = d.user_id
		WHERE d.delivered_at IS NULL AND d.gave_up_at IS NULL AND d.next_attempt_at <= now()
		ORDER BY d.next_attempt_at LIMIT 50`)
	if err != nil {
		return 0, err
	}
	type job struct {
		id                   int64
		user, event, eventID string
		payload              []byte
		attempts             int
		created              time.Time
		alerted              bool
		url, secret          sql.NullString
	}
	var jobs []job
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.id, &j.user, &j.event, &j.eventID, &j.payload, &j.attempts, &j.created, &j.alerted, &j.url, &j.secret); err != nil {
			rows.Close()
			return 0, err
		}
		jobs = append(jobs, j)
	}
	rows.Close()
	for _, j := range jobs {
		status, derr := s.send(ctx, j.url.String, j.secret.String, j.event, j.eventID, j.payload)
		n := j.attempts + 1
		if derr == nil {
			if _, err := s.DB.ExecContext(ctx, `UPDATE webhook_deliveries SET attempts = $2, delivered_at = now(), last_status = $3,
				last_error = NULL WHERE id = $1`, j.id, n, status); err != nil {
				return sent, err
			}
			sent++
			continue
		}
		var st any
		if status > 0 {
			st = status
		}
		age := s.now().Sub(j.created)
		var gaveUp any
		if age > giveUpAfter {
			gaveUp = s.now()
		}
		if _, err := s.DB.ExecContext(ctx, `UPDATE webhook_deliveries SET attempts = $2, last_status = $3, last_error = $4,
			next_attempt_at = now() + $5::interval, gave_up_at = $6 WHERE id = $1`,
			j.id, n, st, clip(derr.Error(), 300), fmt.Sprintf("%d seconds", int(Backoff(n).Seconds())), gaveUp); err != nil {
			return sent, err
		}
		if age > alertAfter && !j.alerted {
			if err := s.alert(ctx, j.id, j.user, j.event, derr.Error()); err != nil {
				return sent, err
			}
		}
	}
	return sent, nil
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func (s *Service) send(ctx context.Context, target, secret, event, eventID string, body []byte) (int, error) {
	if target == "" || secret == "" {
		return 0, errors.New("no webhook URL set")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "SlabsAndSingles-Webhooks/1")
	req.Header.Set("SS-Event", event)
	req.Header.Set("SS-Delivery", eventID)
	req.Header.Set("SS-Signature", Sign(secret, body, s.now()))
	res, err := s.client().Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return res.StatusCode, fmt.Errorf("endpoint answered %d", res.StatusCode)
	}
	return res.StatusCode, nil
}

func (s *Service) alert(ctx context.Context, id int64, userID, event, why string) error {
	if _, err := s.DB.ExecContext(ctx, `UPDATE webhook_deliveries SET alerted = true WHERE id = $1`, id); err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO notifications (user_id, kind, title, body, link) VALUES ($1, 'system', $2, $3, '/sell/connect')`,
		userID, "FireCardManager isn't receiving updates",
		"We've been trying to send "+event+" for over an hour ("+clip(why, 120)+"). Cards sold here may still be listed elsewhere."); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO fraud_alerts (kind, severity, subject_user_id, summary, details, dedupe_key)
		VALUES ('webhook_failing', 'medium', $1, 'Seller webhook failing for over an hour; risk of cross-listing oversells',
		        jsonb_build_object('event', $2::text, 'error', $3::text), $4)
		ON CONFLICT (dedupe_key) DO NOTHING`, userID, event, clip(why, 200),
		"webhook_failing:"+userID+":"+time.Now().UTC().Format("2006-01-02"))
	return err
}
