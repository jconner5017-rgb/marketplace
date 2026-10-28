package inbox

import "testing"

func TestScreen(t *testing.T) {
	blocked := []string{
		"email me at card.guy@gmail.com",
		"text me 555-123-4567",
		"call (555) 123 4567 for a deal",
		"I'll take Venmo",
		"zelle works",
		"can you do F&F to save fees",
		"friends and family is fine",
		"send it goods & services",
		"my cashapp is $cardking",
		"DM me on telegram",
		"pay with paypal?",
		"my number is 555.123.4567",
		"+1 555 123 4567",
	}
	for _, b := range blocked {
		if screen(b) == "" {
			t.Errorf("not blocked: %q", b)
		}
	}
	ok := []string{
		"Is the corner whitening visible in person?",
		"Would you take $15 for it?",
		"Can you ship it in a toploader? Thanks!",
		"PSA 10 cert 12345678 checks out",
		"Is this the 1st edition print?",
		"PSA cert 4401234567 is on the label",
	}
	for _, b := range ok {
		if screen(b) != "" {
			t.Errorf("wrongly blocked: %q", b)
		}
	}
}
