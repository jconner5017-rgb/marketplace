package inbox

import "regexp"

// Messages must keep buyers on the platform, where their card's purchase
// protection applies. The most common card-hobby scam is "pay me on
// Venmo/Zelle/PayPal Friends & Family instead", so those are blocked.
var offPlatform = []*regexp.Regexp{
	regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`), // email address
	// US phone numbers written with separators. Bare 10-digit runs are allowed
	// because grading cert numbers look the same.
	regexp.MustCompile(`(?:\+?1[\s.\-])?(?:\(\d{3}\)\s?|\d{3}[\s.\-])\d{3}[\s.\-]\d{4}\b`),
	regexp.MustCompile(`(?i)\b(venmo|zelle|cash\s?app|cashtag|paypal|apple\s?pay|google\s?pay|crypto|bitcoin|btc|usdt)\b`),
	regexp.MustCompile(`(?i)\b(f\s?&\s?f|friends\s*(and|&|n)\s*family|goods\s*(and|&|n)\s*services|g\s?&\s?s)\b`),
	regexp.MustCompile(`(?i)(^|\s)\$[a-z][a-z0-9_]{2,}`), // $cashtag
	regexp.MustCompile(`(?i)\b(text|call|whats\s?app|telegram|signal|dm)\s+me\b`),
}

// screen returns a reason when a message tries to move the sale off-site.
func screen(body string) string {
	for _, re := range offPlatform {
		if re.MatchString(body) {
			return "off_platform"
		}
	}
	return ""
}

// Screen reports whether text tries to move a sale off the site. Other
// packages (offers) use it on buyer notes.
func Screen(body string) bool { return screen(body) != "" }
