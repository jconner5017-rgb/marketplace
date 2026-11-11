package listings

import (
	"html"
	"regexp"
	"strings"
	"unicode"
)

// Listing descriptions may use a small, safe subset of HTML: headings,
// paragraphs, bold/italic, lists, simple tables, dividers and links. Every
// attribute except a link's address is dropped (no styles, colors or
// classes), so a seller's eBay-style template keeps its structure but takes
// the site's own look, works in dark mode and can't imitate site buttons.
// Scripts, frames, forms, images and anything unknown are removed.

// MaxDescriptionHTML caps the cleaned HTML stored with a listing.
const MaxDescriptionHTML = 20000

var (
	// rename maps an allowed tag to the tag written out.
	richRename = map[string]string{
		"h1": "h2", "h2": "h2", "h3": "h3", "h4": "h4", "h5": "h4", "h6": "h4",
		"p": "p", "div": "div", "span": "span", "br": "br", "hr": "hr",
		"strong": "strong", "b": "strong", "em": "em", "i": "em", "u": "u", "small": "small",
		"ul": "ul", "ol": "ol", "li": "li", "blockquote": "blockquote",
		"table": "table", "thead": "thead", "tbody": "tbody", "tfoot": "tbody", "tr": "tr", "th": "th", "td": "td",
		"a": "a",
	}
	// richDrop removes the element and everything inside it.
	richDrop = map[string]bool{
		"script": true, "style": true, "iframe": true, "frame": true, "frameset": true, "object": true, "embed": true,
		"noscript": true, "template": true, "svg": true, "math": true, "form": true, "button": true, "select": true,
		"textarea": true, "title": true, "head": true, "audio": true, "video": true, "canvas": true, "applet": true,
	}
	richVoid  = map[string]bool{"br": true, "hr": true, "img": true, "input": true, "meta": true, "link": true, "source": true, "wbr": true, "col": true, "area": true, "base": true}
	spaceRun  = regexp.MustCompile(`\s+`)
	safeHref  = regexp.MustCompile(`(?i)^https?://[^\s"'<>]+$`)
	maxNested = 40
)

type richNode struct {
	tag      string // "" for text
	text     string // decoded text
	href     string
	children []*richNode
	parent   *richNode
}

// SanitizeHTML returns the listing-safe version of a description written in
// HTML. Running it again on its own output changes nothing.
func SanitizeHTML(in string) string {
	root := &richNode{tag: "#root"}
	cur := root
	depth := 0
	s := in
	for i := 0; i < len(s); {
		if s[i] != '<' {
			j := strings.IndexByte(s[i:], '<')
			if j < 0 {
				j = len(s) - i
			}
			cur.children = append(cur.children, &richNode{text: html.UnescapeString(s[i : i+j]), parent: cur})
			i += j
			continue
		}
		rest := s[i:]
		switch {
		case strings.HasPrefix(rest, "<!--"):
			end := strings.Index(rest[4:], "-->")
			if end < 0 {
				i = len(s)
			} else {
				i += 4 + end + 3
			}
			continue
		case strings.HasPrefix(rest, "<!") || strings.HasPrefix(rest, "<?"):
			end := strings.IndexByte(rest, '>')
			if end < 0 {
				i = len(s)
			} else {
				i += end + 1
			}
			continue
		}
		closing := strings.HasPrefix(rest, "</")
		k := 1
		if closing {
			k = 2
		}
		n := k
		for n < len(rest) && (rest[n] >= 'a' && rest[n] <= 'z' || rest[n] >= 'A' && rest[n] <= 'Z' || rest[n] >= '0' && rest[n] <= '9') {
			n++
		}
		if n == k { // a lone "<": plain text
			cur.children = append(cur.children, &richNode{text: "<", parent: cur})
			i++
			continue
		}
		name := strings.ToLower(rest[k:n])
		end, attrs := tagEnd(rest, n)
		i += end

		if closing {
			if t, ok := richRename[name]; ok {
				for p := cur; p != root; p = p.parent {
					if p.tag == t {
						for cur != p.parent {
							cur = cur.parent
							depth--
						}
						break
					}
				}
			}
			continue
		}
		if richDrop[name] {
			// Skip everything up to the matching close tag.
			lower := strings.ToLower(s[i:])
			c := strings.Index(lower, "</"+name)
			if c < 0 {
				i = len(s)
			} else {
				g := strings.IndexByte(lower[c:], '>')
				if g < 0 {
					i = len(s)
				} else {
					i += c + g + 1
				}
			}
			continue
		}
		t, ok := richRename[name]
		if !ok || depth >= maxNested {
			continue // unknown or too deep: keep its text, drop the tag
		}
		// Implied closes, as a browser would do them.
		closeOpen := func(tag string, stop ...string) {
			for p := cur; p != root; p = p.parent {
				for _, b := range stop {
					if p.tag == b {
						return
					}
				}
				if p.tag == tag {
					for cur != p.parent {
						cur = cur.parent
						depth--
					}
					return
				}
			}
		}
		switch t {
		case "li":
			closeOpen("li", "ul", "ol")
		case "td", "th":
			closeOpen("td", "tr", "table")
			closeOpen("th", "tr", "table")
		case "tr":
			closeOpen("tr", "table", "tbody", "thead")
		case "p", "div", "h2", "h3", "h4", "ul", "ol", "table", "blockquote", "hr":
			if cur.tag == "p" {
				cur = cur.parent
				depth--
			}
		}
		node := &richNode{tag: t, parent: cur}
		if t == "a" {
			if h := html.UnescapeString(attrs["href"]); safeHref.MatchString(strings.TrimSpace(h)) {
				node.href = strings.TrimSpace(h)
			}
		}
		cur.children = append(cur.children, node)
		if !richVoid[name] && !richVoid[t] {
			cur = node
			depth++
		}
	}
	var b strings.Builder
	for _, c := range root.children {
		render(&b, c)
	}
	out := strings.TrimSpace(b.String())
	if len(out) > MaxDescriptionHTML {
		out = "" // too long to keep whole; the caller reports it
	}
	return out
}

// tagEnd finds the end of a tag starting at name end n, reading attributes.
func tagEnd(s string, n int) (int, map[string]string) {
	attrs := map[string]string{}
	i := n
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' || s[i] == '/' || s[i] == '\f') {
			i++
		}
		if i >= len(s) {
			return len(s), attrs
		}
		if s[i] == '>' {
			return i + 1, attrs
		}
		start := i
		for i < len(s) && s[i] != '=' && s[i] != '>' && s[i] != ' ' && s[i] != '\t' && s[i] != '\n' && s[i] != '\r' && s[i] != '/' {
			i++
		}
		key := strings.ToLower(s[start:i])
		val := ""
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
			i++
		}
		if i < len(s) && s[i] == '=' {
			i++
			for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
				i++
			}
			if i < len(s) && (s[i] == '"' || s[i] == '\'') {
				q := s[i]
				j := strings.IndexByte(s[i+1:], q)
				if j < 0 {
					return len(s), attrs
				}
				val = s[i+1 : i+1+j]
				i += j + 2
			} else {
				start := i
				for i < len(s) && s[i] != ' ' && s[i] != '>' && s[i] != '\t' && s[i] != '\n' && s[i] != '\r' {
					i++
				}
				val = s[start:i]
			}
		}
		if key != "" {
			attrs[key] = val
		}
	}
	return len(s), attrs
}

func hasText(n *richNode) bool {
	if n.tag == "" {
		return strings.TrimFunc(n.text, unicode.IsSpace) != ""
	}
	for _, c := range n.children {
		if hasText(c) {
			return true
		}
	}
	return false
}

func render(b *strings.Builder, n *richNode) {
	if n.tag == "" {
		b.WriteString(html.EscapeString(spaceRun.ReplaceAllString(n.text, " ")))
		return
	}
	switch n.tag {
	case "br", "hr":
		b.WriteString("<" + n.tag + ">")
		return
	case "td", "th":
		// Kept even when empty so the row's columns line up.
	default:
		if !hasText(n) {
			return
		}
	}
	if n.tag == "a" {
		if n.href == "" {
			for _, c := range n.children {
				render(b, c)
			}
			return
		}
		b.WriteString(`<a href="` + html.EscapeString(n.href) + `" rel="nofollow ugc noopener" target="_blank">`)
	} else {
		b.WriteString("<" + n.tag + ">")
	}
	for _, c := range n.children {
		render(b, c)
	}
	b.WriteString("</" + n.tag + ">")
}

var (
	textBlockEnd = regexp.MustCompile(`(?i)</(p|div|h[2-4]|li|tr|table|blockquote|ul|ol)>|<br>|<hr>`)
	textCellEnd  = regexp.MustCompile(`(?i)</t[dh]>`)
	textLi       = regexp.MustCompile(`(?i)<li>`)
	textTag      = regexp.MustCompile(`<[^>]+>`)
	textBlank    = regexp.MustCompile(`\n{3,}`)
)

// TextFromHTML is a plain-text version of sanitized description HTML, for
// search, previews and anywhere HTML can't be shown.
func TextFromHTML(clean string) string {
	s := textLi.ReplaceAllString(clean, "• ")
	s = textCellEnd.ReplaceAllString(s, "  ")
	s = textBlockEnd.ReplaceAllString(s, "\n")
	s = html.UnescapeString(textTag.ReplaceAllString(s, ""))
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimFunc(spaceRun.ReplaceAllString(l, " "), unicode.IsSpace)
	}
	return strings.TrimSpace(textBlank.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}
