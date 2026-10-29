package photos

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
)

const (
	// MaxUploadBytes is the largest file accepted before processing.
	MaxUploadBytes = 15 << 20
	// MaxSide is the longest side kept; larger photos are scaled down.
	MaxSide = 1600
	// MinSide keeps out thumbnails too small to judge a card's condition.
	MinSide = 300
	// maxPixels guards decoding: a tiny file can claim huge dimensions.
	maxPixels = 40_000_000
	quality   = 85
)

var (
	ErrNotImage  = errors.New("that file isn't a JPEG, PNG or GIF photo")
	ErrTooSmall  = errors.New("photos need to be at least 300 pixels on each side so buyers can see the card")
	ErrTooLarge  = errors.New("that photo is too large; use one under 15 MB")
	ErrTooMany   = errors.New("you've uploaded a lot of photos today; try again tomorrow")
	ErrNotFound  = errors.New("photo not found")
	ErrDimension = errors.New("that photo's dimensions are too large")
)

// process decodes an upload, scales it to fit MaxSide, and re-encodes it as
// JPEG. Re-encoding drops all metadata, including camera GPS location.
func process(data []byte) (out []byte, w, h int, err error) {
	if len(data) > MaxUploadBytes {
		return nil, 0, 0, ErrTooLarge
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, ErrNotImage
	}
	if cfg.Width*cfg.Height > maxPixels || cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, 0, 0, ErrDimension
	}
	if cfg.Width < MinSide || cfg.Height < MinSide {
		return nil, 0, 0, ErrTooSmall
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, ErrNotImage
	}
	dst := fit(src, MaxSide)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: quality}); err != nil {
		return nil, 0, 0, err
	}
	b := dst.Bounds()
	return buf.Bytes(), b.Dx(), b.Dy(), nil
}

// fit scales src down (never up) so its longest side is at most max, using
// box averaging, and flattens transparency onto white.
func fit(src image.Image, max int) *image.RGBA {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dw, dh := sw, sh
	if sw > max || sh > max {
		if sw >= sh {
			dw, dh = max, int(float64(sh)*float64(max)/float64(sw)+0.5)
		} else {
			dw, dh = int(float64(sw)*float64(max)/float64(sh)+0.5), max
		}
	}
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		y0 := sb.Min.Y + y*sh/dh
		y1 := sb.Min.Y + (y+1)*sh/dh
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < dw; x++ {
			x0 := sb.Min.X + x*sw/dw
			x1 := sb.Min.X + (x+1)*sw/dw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, b, a, n uint64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					cr, cg, cb, ca := src.At(xx, yy).RGBA()
					r, g, b, a = r+uint64(cr), g+uint64(cg), b+uint64(cb), a+uint64(ca)
					n++
				}
			}
			// Premultiplied colour over white: c + (1 - alpha).
			wht := (0xffff*n - a)
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8(((r + wht) / n) >> 8),
				G: uint8(((g + wht) / n) >> 8),
				B: uint8(((b + wht) / n) >> 8),
				A: 0xff,
			})
		}
	}
	return dst
}
