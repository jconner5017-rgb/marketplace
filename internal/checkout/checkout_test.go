package checkout

import (
	"testing"
	"time"
)

func TestShipBy(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	cases := map[string]string{
		"2026-10-28T10:00:00": "2026-11-02", // Wed -> Mon
		"2026-10-30T22:00:00": "2026-11-04", // Fri -> Wed
		"2026-10-31T09:00:00": "2026-11-04", // Sat -> Wed
	}
	for in, want := range cases {
		p, _ := time.ParseInLocation("2006-01-02T15:04:05", in, ny)
		if got := ShipBy(p).Format("2006-01-02"); got != want {
			t.Errorf("ShipBy(%s) = %s, want %s", in, got, want)
		}
	}
}
