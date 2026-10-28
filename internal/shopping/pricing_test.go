package shopping

import "testing"

func TestBundleDiscount(t *testing.T) {
	per, pct := "per_extra_item", "percent_3plus"
	v75, v1500, v9000 := 75, 1500, 9000
	cases := []struct {
		typ        *string
		val        *int
		cards, sub int
		want       int
		label      string
	}{
		{nil, nil, 5, 5000, 0, ""},
		{&per, &v75, 1, 1000, 0, ""},
		{&per, &v75, 4, 4000, 225, "$0.75 off each extra card"},
		{&pct, &v1500, 2, 4000, 0, ""},
		{&pct, &v1500, 3, 6000, 900, "15% off 3+ cards"},
		{&per, &v9000, 3, 1000, 500, "$90.00 off each extra card"}, // capped at half
	}
	for i, c := range cases {
		got, label := BundleDiscount(c.typ, c.val, c.cards, c.sub)
		if got != c.want || label != c.label {
			t.Errorf("case %d: got %d %q, want %d %q", i, got, label, c.want, c.label)
		}
	}
}
