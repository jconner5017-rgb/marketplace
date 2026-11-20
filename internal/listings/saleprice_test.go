package listings

import "testing"

func TestSalePriceRounding(t *testing.T) {
	for _, c := range []struct{ cents, pct, want int }{
		{188, 15, 160}, // 28.2¢ off -> 28¢
		{99, 15, 84},   // 14.85¢ off -> 15¢ (was 14¢)
		{1000, 20, 800},
		{150, 0, 150},
	} {
		if got := SalePrice(c.cents, c.pct); got != c.want {
			t.Errorf("SalePrice(%d, %d) = %d, want %d", c.cents, c.pct, got, c.want)
		}
		// The badge shows the percent the seller chose.
		if c.pct > 0 {
			sale := SalePrice(c.cents, c.pct)
			if shown := max(1, ((c.cents-sale)*200/c.cents+1)/2); shown != c.pct {
				t.Errorf("%d at %d%%: badge shows %d%%", c.cents, c.pct, shown)
			}
		}
	}
}
