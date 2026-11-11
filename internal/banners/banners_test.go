package banners

import "testing"

func TestCleanLink(t *testing.T) {
	for in, want := range map[string]bool{
		"": true, "/search?q=prizm": true, "https://firecardmanager.com": true, "http://example.com/x": true,
		"//evil.com": false, "javascript:alert(1)": false, "ftp://x.com": false, "example.com": false,
	} {
		if _, ok := CleanLink(in); ok != want {
			t.Errorf("CleanLink(%q) = %v, want %v", in, ok, want)
		}
	}
}
