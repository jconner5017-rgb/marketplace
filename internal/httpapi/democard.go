package httpapi

import (
	"fmt"
	"hash/fnv"
	"html"
	"net/http"
	"strings"
	"unicode/utf8"
)

// handleDemoCard draws a placeholder card image for sample listings:
// GET /demo/card/{name}.svg?t=Title&s=Set&k=tcg|sports&side=front|back
// It is generic artwork (a frame, a pattern, text) — never a real card.
func handleDemoCard(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !strings.HasSuffix(name, ".svg") || len(name) > 80 {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	title := clip(q.Get("t"), 26)
	set := clip(q.Get("s"), 34)
	sports := q.Get("k") == "sports"
	back := q.Get("side") == "back"

	h := fnv.New32a()
	h.Write([]byte(name))
	seed := h.Sum32()
	palettes := [][3]string{
		{"#1F4FA8", "#E8EEF9", "#F2B705"},
		{"#8C1D18", "#F8E9E7", "#D9A441"},
		{"#1D6B4F", "#E6F2EC", "#E3C567"},
		{"#4B2A86", "#EFE8F8", "#5CC8D6"},
		{"#B35A00", "#FBF0E3", "#2F5D8A"},
		{"#20343F", "#E7EEF1", "#E86A33"},
	}
	pal := palettes[int(seed)%len(palettes)]
	frame, paper, accent := pal[0], pal[1], pal[2]

	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 250 350" width="500" height="700" font-family="Arial, Helvetica, sans-serif">`)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="250" height="350" rx="14" fill="%s"/>`, frame)

	if back {
		fmt.Fprintf(&b, `<rect x="12" y="12" width="226" height="326" rx="8" fill="none" stroke="%s" stroke-width="3"/>`, accent)
		for i := 0; i < 9; i++ {
			fmt.Fprintf(&b, `<circle cx="125" cy="175" r="%d" fill="none" stroke="%s" stroke-opacity="%.2f" stroke-width="2"/>`,
				12+i*13, paper, 0.15+float64(i)*0.06)
		}
		fmt.Fprintf(&b, `<text x="125" y="181" text-anchor="middle" font-size="18" font-weight="700" fill="%s">SAMPLE</text>`, paper)
		b.WriteString(`</svg>`)
		writeSVG(w, b.String())
		return
	}

	fmt.Fprintf(&b, `<rect x="12" y="12" width="226" height="326" rx="8" fill="%s"/>`, paper)
	// Name bar.
	fmt.Fprintf(&b, `<rect x="12" y="12" width="226" height="40" rx="8" fill="%s"/>`, frame)
	fmt.Fprintf(&b, `<text x="24" y="38" font-size="15" font-weight="700" fill="#FFFFFF">%s</text>`, html.EscapeString(title))
	// Art window with a geometric pattern seeded by the name.
	fmt.Fprintf(&b, `<rect x="24" y="62" width="202" height="160" rx="4" fill="%s"/>`, frame)
	if sports {
		// Stadium-like bands.
		for i := 0; i < 6; i++ {
			y := 72 + i*25
			op := 0.12 + float64((int(seed)>>uint(i))&3)*0.08
			fmt.Fprintf(&b, `<rect x="24" y="%d" width="202" height="14" fill="%s" fill-opacity="%.2f"/>`, y, paper, op)
		}
		fmt.Fprintf(&b, `<circle cx="125" cy="142" r="44" fill="%s" fill-opacity="0.9"/>`, accent)
	} else {
		for i := 0; i < 7; i++ {
			cx := 40 + int((seed>>uint(i*3))%170)
			cy := 78 + int((seed>>uint(i*2+1))%128)
			rr := 10 + int((seed>>uint(i))%26)
			fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="%d" fill="%s" fill-opacity="0.55"/>`, cx, cy, rr, accent)
		}
		fmt.Fprintf(&b, `<polygon points="125,92 165,142 125,192 85,142" fill="%s" fill-opacity="0.85"/>`, paper)
	}
	// Text box.
	fmt.Fprintf(&b, `<rect x="24" y="232" width="202" height="76" rx="4" fill="#FFFFFF" stroke="%s" stroke-opacity="0.3"/>`, frame)
	fmt.Fprintf(&b, `<text x="34" y="256" font-size="11" fill="#333333">%s</text>`, html.EscapeString(set))
	fmt.Fprintf(&b, `<rect x="34" y="268" width="150" height="5" rx="2" fill="%s" fill-opacity="0.25"/>`, frame)
	fmt.Fprintf(&b, `<rect x="34" y="281" width="120" height="5" rx="2" fill="%s" fill-opacity="0.25"/>`, frame)
	fmt.Fprintf(&b, `<rect x="34" y="294" width="135" height="5" rx="2" fill="%s" fill-opacity="0.25"/>`, frame)
	fmt.Fprintf(&b, `<text x="226" y="330" text-anchor="end" font-size="9" font-weight="700" fill="%s">SAMPLE CARD</text>`, frame)
	b.WriteString(`</svg>`)
	writeSVG(w, b.String())
}

func writeSVG(w http.ResponseWriter, svg string) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	// The SVG has no scripts; this keeps it that way even if opened directly.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.Write([]byte(svg))
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}
