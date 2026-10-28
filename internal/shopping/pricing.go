package shopping

import (
	"fmt"
	"strconv"
)

// BundleDiscount applies a seller's multi-card discount to one seller's
// part of an order. per_extra_item: val cents off every card after the
// first. percent_3plus: val basis points off when 3+ cards. The discount
// never exceeds half the subtotal, so a misconfigured discount can't
// give cards away.
func BundleDiscount(typ *string, val *int, cards, subtotal int) (int, string) {
	if typ == nil || val == nil || cards < 2 {
		return 0, ""
	}
	d, label := 0, ""
	switch *typ {
	case "per_extra_item":
		d = *val * (cards - 1)
		label = fmt.Sprintf("$%d.%02d off each extra card", *val/100, *val%100)
	case "percent_3plus":
		if cards < 3 {
			return 0, ""
		}
		d = subtotal * *val / 10000
		label = fmt.Sprintf("%s%% off 3+ cards", trimZero(float64(*val)/100))
	}
	if limit := subtotal / 2; d > limit {
		d = limit
	}
	return d, label
}

// ShipMethod picks how a seller's part of the order ships: a stamped
// envelope only for exactly one raw card at $20 or less; otherwise a label.
func ShipMethod(g SellerGroup) string {
	if g.CardCount == 1 && g.TotalCents <= 2000 {
		for _, l := range g.Lines {
			if l.Available && l.Listing.Grader == nil && l.Listing.PackagePreset == "envelope" {
				return "envelope"
			}
		}
	}
	return "label"
}

func trimZero(f float64) string {
	s := strconv.FormatFloat(f, 'f', 2, 64)
	for len(s) > 0 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	if len(s) > 0 && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	return s
}

func itoa(n int) string { return strconv.Itoa(n) }
