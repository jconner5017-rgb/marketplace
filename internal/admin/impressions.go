package admin

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/lib/pq"
)

// Impressions (2026-11-20): listings shown on a results page. Counting each
// one with its own write would add dozens of writes per page, so they're
// tallied in memory and written in one statement every 30 seconds.

type impressionBuffer struct {
	mu    sync.Mutex
	n     map[string]int
	start sync.Once
}

// AddImpressions counts one showing of each listing.
func (m *Metrics) AddImpressions(ids []string) {
	if len(ids) == 0 {
		return
	}
	b := &m.impressions
	b.mu.Lock()
	if b.n == nil {
		b.n = map[string]int{}
	}
	for _, id := range ids {
		b.n[id]++
	}
	b.mu.Unlock()
	b.start.Do(func() {
		go func() {
			t := time.NewTicker(30 * time.Second)
			defer t.Stop()
			for range t.C {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				if err := m.FlushImpressions(ctx); err != nil {
					slog.Error("flush impressions", "err", err)
				}
				cancel()
			}
		}()
	})
}

// FlushImpressions writes the tallied impressions to today's totals.
func (m *Metrics) FlushImpressions(ctx context.Context) error {
	b := &m.impressions
	b.mu.Lock()
	pending := b.n
	b.n = map[string]int{}
	b.mu.Unlock()
	if len(pending) == 0 {
		return nil
	}
	ids := make([]string, 0, len(pending))
	counts := make([]int64, 0, len(pending))
	for id, n := range pending {
		ids = append(ids, id)
		counts = append(counts, int64(n))
	}
	_, err := m.DB.ExecContext(ctx, `
		INSERT INTO listing_impressions_daily (listing_id, day, impressions)
		SELECT x.id, current_date, x.n FROM unnest($1::uuid[], $2::int[]) AS x(id, n)
		JOIN listings l ON l.id = x.id
		ON CONFLICT (listing_id, day) DO UPDATE SET impressions = listing_impressions_daily.impressions + EXCLUDED.impressions`,
		pq.Array(ids), pq.Array(counts))
	return err
}
