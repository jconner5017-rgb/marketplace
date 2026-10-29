package admin

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"strings"
	"sync"
	"time"
)

// IPRule is one network staff may reach the back office from.
type IPRule struct {
	ID        int64     `json:"id"`
	CIDR      string    `json:"cidr"`
	Label     string    `json:"label"`
	CreatedBy *string   `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

var ErrLockout = errors.New("that change would block the address you're using now; add your current network first")

// IPRules is the admin IP allowlist. With no rules, any address may sign in
// (the authenticator check still applies). Disabled turns enforcement off
// (ADMIN_IP_ALLOWLIST=off), the break-glass switch if staff lock themselves out.
type IPRules struct {
	DB       *sql.DB
	Audit    *Audit
	Disabled bool

	mu     sync.Mutex
	cached []*net.IPNet
	at     time.Time
}

// Allowed reports whether ip may use the back office. Rules are cached for 15 seconds.
func (p *IPRules) Allowed(ctx context.Context, ip string) (bool, error) {
	if p.Disabled {
		return true, nil
	}
	nets, err := p.nets(ctx)
	if err != nil {
		return false, err
	}
	return inNets(nets, net.ParseIP(ip)), nil
}

func inNets(nets []*net.IPNet, ip net.IP) bool {
	if len(nets) == 0 {
		return true
	}
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func (p *IPRules) nets(ctx context.Context) ([]*net.IPNet, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cached != nil && time.Since(p.at) < 15*time.Second {
		return p.cached, nil
	}
	rules, err := p.List(ctx)
	if err != nil {
		return nil, err
	}
	nets := make([]*net.IPNet, 0, len(rules))
	for _, r := range rules {
		if _, n, err := net.ParseCIDR(r.CIDR); err == nil {
			nets = append(nets, n)
		}
	}
	p.cached, p.at = nets, time.Now()
	return nets, nil
}

// Forget drops the cached rules (after a direct database change).
func (p *IPRules) Forget() {
	p.mu.Lock()
	p.cached = nil
	p.mu.Unlock()
}

func (p *IPRules) List(ctx context.Context) ([]IPRule, error) {
	rows, err := p.DB.QueryContext(ctx, `SELECT r.id, r.cidr::text, r.label, u.email, r.created_at
		FROM admin_ip_rules r LEFT JOIN users u ON u.id = r.created_by ORDER BY r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IPRule{}
	for rows.Next() {
		var r IPRule
		if err := rows.Scan(&r.ID, &r.CIDR, &r.Label, &r.CreatedBy, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ParseNetwork accepts an address (203.0.113.7) or a network (203.0.113.0/24).
func ParseNetwork(s string) (*net.IPNet, error) {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "/") {
		ip := net.ParseIP(s)
		if ip == nil {
			return nil, &FieldError{"cidr", "enter an IP address like 203.0.113.7 or a range like 203.0.113.0/24"}
		}
		bits := 128
		if ip.To4() != nil {
			ip, bits = ip.To4(), 32
		}
		return &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}, nil
	}
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		return nil, &FieldError{"cidr", "enter an IP address like 203.0.113.7 or a range like 203.0.113.0/24"}
	}
	if ones, bits := n.Mask.Size(); (bits == 32 && ones < 8) || (bits == 128 && ones < 16) {
		return nil, &FieldError{"cidr", "that range is too wide to be useful"}
	}
	return n, nil
}

// Add allows a network. The first rule must cover the address in use.
func (p *IPRules) Add(ctx context.Context, a Actor, cidr, label string) (IPRule, error) {
	n, err := ParseNetwork(cidr)
	if err != nil {
		return IPRule{}, err
	}
	label = strings.TrimSpace(label)
	if len(label) > 80 {
		return IPRule{}, &FieldError{"label", "keep the label under 80 characters"}
	}
	rules, err := p.List(ctx)
	if err != nil {
		return IPRule{}, err
	}
	nets := []*net.IPNet{n}
	for _, r := range rules {
		if _, x, err := net.ParseCIDR(r.CIDR); err == nil {
			nets = append(nets, x)
		}
	}
	if !p.Disabled && !inNets(nets, net.ParseIP(a.IP)) {
		return IPRule{}, ErrLockout
	}
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return IPRule{}, err
	}
	defer tx.Rollback()
	var r IPRule
	err = tx.QueryRowContext(ctx, `INSERT INTO admin_ip_rules (cidr, label, created_by) VALUES ($1, $2, $3)
		ON CONFLICT (cidr) DO NOTHING RETURNING id, cidr::text, label, created_at`, n.String(), label, a.ID).
		Scan(&r.ID, &r.CIDR, &r.Label, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return IPRule{}, &FieldError{"cidr", "that network is already on the list"}
	}
	if err != nil {
		return IPRule{}, err
	}
	if err := p.Audit.Log(ctx, tx, a.entry("security.ip_rule_added", "ip_rule", n.String(), "",
		map[string]any{"cidr": r.CIDR, "label": label})); err != nil {
		return IPRule{}, err
	}
	if err := tx.Commit(); err != nil {
		return IPRule{}, err
	}
	p.Forget()
	return r, nil
}

// Remove deletes a rule, unless that would block the address in use.
func (p *IPRules) Remove(ctx context.Context, a Actor, id int64) error {
	rules, err := p.List(ctx)
	if err != nil {
		return err
	}
	var gone *IPRule
	var nets []*net.IPNet
	for i, r := range rules {
		if r.ID == id {
			gone = &rules[i]
			continue
		}
		if _, x, err := net.ParseCIDR(r.CIDR); err == nil {
			nets = append(nets, x)
		}
	}
	if gone == nil {
		return ErrNotFound
	}
	if !p.Disabled && len(nets) > 0 && !inNets(nets, net.ParseIP(a.IP)) {
		return ErrLockout
	}
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM admin_ip_rules WHERE id = $1`, id); err != nil {
		return err
	}
	if err := p.Audit.Log(ctx, tx, a.entry("security.ip_rule_removed", "ip_rule", gone.CIDR, "",
		map[string]any{"cidr": gone.CIDR, "label": gone.Label, "rules_left": len(nets)})); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	p.Forget()
	return nil
}

// Blocked records a refused back-office request as a fraud alert, once per
// staff member and address per day.
func (p *IPRules) Blocked(ctx context.Context, userID, email, ip string) error {
	_, err := p.DB.ExecContext(ctx, `
		INSERT INTO fraud_alerts (kind, severity, subject_user_id, summary, details, dedupe_key)
		VALUES ('admin_ip_blocked', 'high', $1, $2, jsonb_build_object('ip', $3::text), $4)
		ON CONFLICT (dedupe_key) DO NOTHING`,
		userID, "Staff account "+email+" tried the back office from an address not on the allowlist", ip,
		"admin_ip_blocked:"+userID+":"+ip+":"+time.Now().UTC().Format("2006-01-02"))
	return err
}
