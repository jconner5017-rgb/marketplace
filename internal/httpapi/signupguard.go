package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Sign-up protection against scripted account creation. Three checks, none
// of which a person filling in the form ever notices:
//
//  1. A hidden "website" field (a honeypot). People never see it; form-filling
//     bots fill every field they find.
//  2. A form token from GET /v1/auth/signup-check, signed by the server and
//     stamped with the time the form was opened. A sign-up needs one that is
//     at least signupMinAge old (bots submit instantly) and less than
//     signupMaxAge old. Scripts that post straight to the API have none.
//  4. Throwaway inbox services (Mailinator and the like) are refused, at
//     most SignupsPerIPDay accounts are made from one network a day, and
//     Cloudflare Turnstile is checked when its keys are set.
//  3. Gmail address tricks: Gmail ignores dots and anything after "+", so
//     bots make endless "different" addresses for one inbox
//     (c.s.e.j.h.i.l.g@gmail.com). Heavily dotted Gmail addresses are refused,
//     and a Gmail address that is the same inbox as an existing account counts
//     as already registered.
//
// The token checks are on when SignupGuardKey is set (the API sets it from
// its own secrets); tests that don't set it skip them.

const (
	signupMinAge = 3 * time.Second
	signupMaxAge = 3 * time.Hour
)

func (s *Server) signupToken(now time.Time) string {
	var ts [8]byte
	binary.BigEndian.PutUint64(ts[:], uint64(now.Unix()))
	mac := hmac.New(sha256.New, s.signupKey)
	mac.Write([]byte("signup-form:"))
	mac.Write(ts[:])
	return base64.RawURLEncoding.EncodeToString(append(ts[:], mac.Sum(nil)[:16]...))
}

// signupTokenAge returns how long ago a valid token was issued.
func (s *Server) signupTokenAge(tok string, now time.Time) (time.Duration, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(tok))
	if err != nil || len(raw) != 24 {
		return 0, false
	}
	mac := hmac.New(sha256.New, s.signupKey)
	mac.Write([]byte("signup-form:"))
	mac.Write(raw[:8])
	if !hmac.Equal(mac.Sum(nil)[:16], raw[8:]) {
		return 0, false
	}
	issued := time.Unix(int64(binary.BigEndian.Uint64(raw[:8])), 0)
	return now.Sub(issued), true
}

func (s *Server) handleSignupCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	tok := ""
	if len(s.signupKey) > 0 {
		tok = s.signupToken(time.Now())
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": tok, "turnstile_site_key": s.turnstileSiteKey})
}

func (s *Server) verifyTurnstile(ctx context.Context, token, ip string) (bool, error) {
	if token == "" {
		return false, nil
	}
	form := url.Values{"secret": {s.turnstileSecret}, "response": {token}}
	if ip != "" {
		form.Set("remoteip", ip)
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.turnstileURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	var out struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return false, fmt.Errorf("turnstile reply: %w", err)
	}
	return out.Success, nil
}

// disposableDomains are throwaway inbox services.
var disposableDomains = map[string]bool{}

func init() {
	for _, d := range strings.Fields(`mailinator.com guerrillamail.com guerrillamail.net guerrillamail.org guerrillamailblock.com sharklasers.com grr.la
		10minutemail.com 10minutemail.net 20minutemail.com tempmail.com temp-mail.org temp-mail.io tempmail.net tempmailo.com tempr.email tempail.com
		yopmail.com yopmail.net yopmail.fr trashmail.com trashmail.de trashmail.net getnada.com nada.email dispostable.com maildrop.cc mailnesia.com
		throwawaymail.com fakeinbox.com mohmal.com emailondeck.com mintemail.com burnermail.io spamgourmet.com moakt.com mailcatch.com
		mytemp.email tempinbox.com discard.email discardmail.com spambox.us getairmail.com inboxbear.com mailpoof.com emailfake.com
		fakemail.net 1secmail.com 1secmail.org 1secmail.net mail.tm mail.gw byom.de harakirimail.com mailsac.com spam4.me 33mail.com
		anonaddy.me tmail.ws tmpmail.org tmpmail.net linshiyouxiang.net dropmail.me minuteinbox.com emailtemporanea.net`) {
		disposableDomains[d] = true
	}
}

func disposable(email string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	d := strings.ToLower(strings.TrimSpace(email[at+1:]))
	for d != "" {
		if disposableDomains[d] {
			return true
		}
		i := strings.IndexByte(d, '.')
		if i < 0 {
			break
		}
		d = d[i+1:]
	}
	return false
}

// signupBlocked returns a reason code when a sign-up looks automated, or "".
func (s *Server) signupBlocked(req signupRequest) string {
	if strings.TrimSpace(req.Website) != "" {
		return "honeypot"
	}
	if len(s.signupKey) == 0 {
		return ""
	}
	age, ok := s.signupTokenAge(req.FormToken, time.Now())
	switch {
	case !ok:
		return "no_token"
	case age < signupMinAge:
		return "too_fast"
	case age > signupMaxAge:
		return "stale_token"
	}
	return ""
}

// gmailInbox returns the real Gmail inbox an address delivers to ("" when
// it isn't a Gmail address): lower case, dots and +tags removed.
func gmailInbox(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	at := strings.LastIndexByte(email, '@')
	if at < 1 {
		return ""
	}
	local, domain := email[:at], email[at+1:]
	if domain != "gmail.com" && domain != "googlemail.com" {
		return ""
	}
	if i := strings.IndexByte(local, '+'); i >= 0 {
		local = local[:i]
	}
	return strings.ReplaceAll(local, ".", "")
}

// dottedGmail reports a Gmail address padded with dots, the usual way bots
// make many addresses for one inbox: four or more dots with most pieces
// one letter long (c.s.e.j.h.i.l.g). Ordinary names like john.a.smith pass.
func dottedGmail(email string) bool {
	if gmailInbox(email) == "" {
		return false
	}
	local := strings.ToLower(strings.TrimSpace(email))
	local = local[:strings.LastIndexByte(local, '@')]
	if i := strings.IndexByte(local, '+'); i >= 0 {
		local = local[:i]
	}
	parts := strings.Split(local, ".")
	if len(parts) < 5 {
		return false
	}
	single := 0
	for _, p := range parts {
		if len(p) <= 1 {
			single++
		}
	}
	return single*10 >= len(parts)*6 // 60% or more are single letters
}
