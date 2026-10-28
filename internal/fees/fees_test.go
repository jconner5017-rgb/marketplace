package fees

import "testing"

func TestFor(t *testing.T) {
	s := Default
	for _, c := range []struct{ price, want int }{
		{100, 100},     // never more than the card
		{2000, 150},    // flat up to $100
		{10000, 150},   //
		{10001, 300},   // 3% of $100.01 = 300
		{20000, 600},   // 3%
		{100000, 2500}, // capped at $25
	} {
		if got := s.For(c.price); got != c.want {
			t.Errorf("fee on %d: got %d want %d", c.price, got, c.want)
		}
	}
}
