package fcm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Inventory is what a seller's inventory endpoint returns (GET, signed
// like webhooks over an empty body):
//
//	{"items": [{"external_id": "FCM-123", "quantity": 2}, ...]}
//
// Only listed SKUs are touched: quantity 0 removes the listing here, a lower
// quantity lowers ours. SKUs missing from the answer are left alone.
type Inventory struct {
	Items []struct {
		ExternalID string `json:"external_id"`
		Quantity   int    `json:"quantity"`
	} `json:"items"`
}

// ReconcileResult reports one seller's reconciliation.
type ReconcileResult struct {
	Checked, Removed, Lowered int
}

// Reconcile compares one seller's listings with their inventory tool.
// onRemoved is called for each listing taken down (to stop checkouts in progress).
func (s *Service) Reconcile(ctx context.Context, userID string, onRemoved func(context.Context, string) error) (ReconcileResult, error) {
	var res ReconcileResult
	var target, secret string
	if err := s.DB.QueryRowContext(ctx, `SELECT coalesce(inventory_url, ''), coalesce(webhook_secret, '') FROM seller_profiles
		WHERE user_id = $1`, userID).Scan(&target, &secret); err != nil {
		return res, err
	}
	if target == "" || secret == "" {
		return res, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return res, err
	}
	req.Header.Set("SS-Signature", Sign(secret, nil, s.now()))
	req.Header.Set("User-Agent", "SlabsAndSingles-Reconcile/1")
	r, err := s.client().Do(req)
	if err != nil {
		return res, err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return res, fmt.Errorf("inventory endpoint answered %d", r.StatusCode)
	}
	var inv Inventory
	if err := json.NewDecoder(io.LimitReader(r.Body, 20<<20)).Decode(&inv); err != nil {
		return res, fmt.Errorf("read inventory: %w", err)
	}
	for _, it := range inv.Items {
		if it.ExternalID == "" || it.Quantity < 0 {
			continue
		}
		res.Checked++
		var id string
		var qty int
		err := s.DB.QueryRowContext(ctx, `SELECT id, quantity FROM listings WHERE seller_id = $1 AND external_id = $2
			AND format = 'fixed' AND status IN ('active', 'sold_out', 'hidden')`, userID, it.ExternalID).Scan(&id, &qty)
		if err != nil {
			continue // not listed here, or not a fixed-price listing
		}
		switch {
		case it.Quantity == 0:
			if _, err := s.DB.ExecContext(ctx, `UPDATE listings SET status = 'removed', version = version + 1, updated_at = now()
				WHERE id = $1`, id); err != nil {
				return res, err
			}
			res.Removed++
			if onRemoved != nil {
				if err := onRemoved(ctx, id); err != nil {
					return res, err
				}
			}
		case it.Quantity < qty:
			if _, err := s.DB.ExecContext(ctx, `UPDATE listings SET quantity = $2, version = version + 1, updated_at = now()
				WHERE id = $1`, id, it.Quantity); err != nil {
				return res, err
			}
			res.Lowered++
		}
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE seller_profiles SET reconciled_at = now() WHERE user_id = $1`, userID)
	return res, err
}

// RunReconcile reconciles every connected seller not checked in 20 hours.
func (s *Service) RunReconcile(ctx context.Context, onRemoved func(context.Context, string) error, logf func(string, ...any)) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT user_id FROM seller_profiles WHERE inventory_url IS NOT NULL
		AND (reconciled_at IS NULL OR reconciled_at < now() - interval '20 hours') LIMIT 500`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		cctx, cancel := context.WithTimeout(ctx, time.Minute)
		r, err := s.Reconcile(cctx, id, onRemoved)
		cancel()
		if err != nil {
			logf("reconcile failed", "seller", id, "err", err)
			// Try again tomorrow rather than every hour.
			s.DB.ExecContext(ctx, `UPDATE seller_profiles SET reconciled_at = now() WHERE user_id = $1`, id)
			continue
		}
		if r.Removed+r.Lowered > 0 {
			logf("reconciled", "seller", id, "removed", r.Removed, "lowered", r.Lowered)
		}
	}
	return nil
}
