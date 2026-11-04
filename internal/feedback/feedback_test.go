package feedback

import "testing"

func TestExtortion(t *testing.T) {
	bad := []string{
		"Send me $20 back or I leave a negative",
		"Partial refund or you get a 1 star",
		"Negative feedback unless you refund me",
		"I'll change my feedback once you send $15",
		"give me a discount otherwise bad review",
	}
	good := []string{
		"Fast shipping, card as described",
		"Seller refunded me quickly when it got lost, great communication",
		"Negative: card was creased and seller never answered",
		"$5 card, arrived in a toploader, would buy again",
	}
	for _, s := range bad {
		if !Extortion(s) {
			t.Errorf("missed: %q", s)
		}
	}
	for _, s := range good {
		if Extortion(s) {
			t.Errorf("false alarm: %q", s)
		}
	}
	if Mask("collector") != "c***r" || Mask("ab") != "a***" {
		t.Fatal(Mask("collector"), Mask("ab"))
	}
}

func TestMilestoneFor(t *testing.T) {
	cases := []struct {
		sales     int
		key, next string
		nextSales int
	}{
		{0, "", "bronze", 10}, {9, "", "bronze", 10}, {10, "bronze", "silver", 50},
		{249, "silver", "gold", 250}, {1000, "platinum", "diamond", 5000}, {9000, "diamond", "", 0},
	}
	for _, c := range cases {
		m := MilestoneFor(c.sales)
		if m.Key != c.key || m.NextKey != c.next || m.NextSales != c.nextSales {
			t.Errorf("%d sales: got %+v", c.sales, m)
		}
	}
}
