package listings

import (
	"strings"
	"testing"
)

func TestSanitizeHTML(t *testing.T) {
	cases := map[string]string{
		`<p style="color:red" onclick="x()">Hi <b>there</b></p>`:                    `<p>Hi <strong>there</strong></p>`,
		`<script>alert(1)</script><p>ok</p>`:                                        `<p>ok</p>`,
		`<SCRIPT SRC=//evil></SCRIPT>ok`:                                            `ok`,
		`<img src=x onerror=alert(1)>text`:                                          `text`,
		`<a href="javascript:alert(1)">click</a>`:                                   `click`,
		`<a href="https://example.com/x?a=1&amp;b=2" onclick=x>site</a>`:            `<a href="https://example.com/x?a=1&amp;b=2" rel="nofollow ugc noopener" target="_blank">site</a>`,
		`&lt;script&gt;alert(1)&lt;/script&gt;`:                                     `&lt;script&gt;alert(1)&lt;/script&gt;`,
		`<p title="a > b">x</p>`:                                                    `<p>x</p>`,
		`<iframe src="https://evil"></iframe><style>p{}</style>keep`:                `keep`,
		`<div><div>&nbsp;</div></div><h1>Title</h1>`:                                `<h2>Title</h2>`,
		`<ul><li>a<li>b</ul>`:                                                       `<ul><li>a</li><li>b</li></ul>`,
		`<table><tr><td>Sport</td><td>Football</td></tr><tr><td></td></tr></table>`: `<table><tr><td>Sport</td><td>Football</td></tr></table>`,
		`<!-- note --><font color=red>plain</font>`:                                 `plain`,
		`<scr<script>ipt>alert(1)</script>`:                                         `ipt&gt;alert(1)`,
		`<p>unclosed <em>em`:                                                        `<p>unclosed <em>em</em></p>`,
		`a < b`:                                                                     `a &lt; b`,
		`<svg><script>alert(1)</script></svg>x`:                                     `x`,
		`<form><input value="x">Name</form>z`:                                       `z`,
	}
	for in, want := range cases {
		got := SanitizeHTML(in)
		if got != want {
			t.Errorf("SanitizeHTML(%q)\n got  %q\n want %q", in, got, want)
		}
		if again := SanitizeHTML(got); again != got {
			t.Errorf("not stable: %q -> %q", got, again)
		}
		for _, bad := range []string{"<script", "onerror", "onclick", "javascript:", "style=", "<img", "<iframe"} {
			if strings.Contains(strings.ToLower(got), bad) {
				t.Errorf("SanitizeHTML(%q) kept %q: %q", in, bad, got)
			}
		}
	}
	// Deep nesting stays bounded.
	deep := strings.Repeat("<div>", 500) + "x" + strings.Repeat("</div>", 500)
	if got := SanitizeHTML(deep); strings.Count(got, "<div>") > maxNested {
		t.Errorf("deep nesting: %d divs", strings.Count(got, "<div>"))
	}
}

func TestTextFromHTML(t *testing.T) {
	got := TextFromHTML(SanitizeHTML(`<h2>Details</h2><table><tr><td>Sport</td><td>Football</td></tr></table><ul><li>Fast</li><li>Safe</li></ul><p>A &amp; B</p>`))
	want := "Details\nSport Football\n\n• Fast\n• Safe\n\nA & B"
	if got != want {
		t.Errorf("TextFromHTML = %q, want %q", got, want)
	}
}
