// Package imb encodes USPS Intelligent Mail barcodes (USPS-B-3200), the
// 65-bar code USPS sorting machines read on letters. A uniquely numbered
// barcode lets us follow an envelope through Informed Visibility scans.
package imb

import (
	"errors"
	"fmt"
	"math/big"
	"math/bits"
	"strings"
)

// Bar states, left to right: Full, Ascender, Descender, Tracker.
const (
	Full      = 'F'
	Ascender  = 'A'
	Descender = 'D'
	Tracker   = 'T'
)

var (
	ErrTracking = errors.New("tracking code must be 20 digits and the second digit 0-4")
	ErrRouting  = errors.New("routing code must be empty or 5, 9 or 11 digits")
)

// barTable maps each of the 13 bits of the 10 characters to one of the 130
// bar halves (65 descenders, then 65 ascenders), 1-based. From USPS-B-3200
// Appendix D, Table IV.
var barTable = [130]uint8{
	67, 6, 78, 16, 86, 95, 34, 40, 45, 113, 117, 121, 62, 87, 18, 104, 41, 76, 57, 119,
	115, 72, 97, 2, 127, 26, 105, 35, 122, 52, 114, 7, 24, 82, 68, 63, 94, 44, 77, 112,
	70, 100, 39, 30, 107, 15, 125, 85, 10, 65, 54, 88, 20, 106, 46, 66, 8, 116, 29, 61,
	99, 80, 90, 37, 123, 51, 25, 84, 129, 56, 4, 109, 96, 28, 36, 47, 11, 71, 33, 102,
	21, 9, 17, 49, 124, 79, 64, 91, 42, 69, 53, 60, 14, 1, 27, 103, 126, 75, 89, 50,
	120, 19, 32, 110, 92, 111, 130, 59, 31, 12, 81, 43, 55, 5, 74, 22, 101, 128, 58, 118,
	48, 108, 38, 98, 93, 23, 83, 13, 73, 3,
}

// The codeword-to-character tables (USPS-B-3200 Appendix C): every 13-bit
// value with five bits set, then every one with two bits set.
var table5, table2 = nOf13(5, 1287), nOf13(2, 78)

func nOf13(n, size int) []uint16 {
	t := make([]uint16, size)
	lo, hi := 0, size-1
	for c := uint16(0); c < 8192; c++ {
		if bits.OnesCount16(c) != n {
			continue
		}
		r := bits.Reverse16(c) >> 3
		switch {
		case r < c:
		case r == c:
			t[hi] = c
			hi--
		default:
			t[lo], t[lo+1] = c, r
			lo += 2
		}
	}
	return t
}

func digits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// Encode returns the 65 bar states for a 20-digit tracking code (barcode ID,
// service type ID, mailer ID and serial number) and a routing code (the
// delivery ZIP, ZIP+4 or ZIP+4+delivery point, without dashes).
func Encode(tracking, routing string) (string, error) {
	if len(tracking) != 20 || !digits(tracking) || tracking[1] > '4' {
		return "", ErrTracking
	}
	switch len(routing) {
	case 0, 5, 9, 11:
	default:
		return "", ErrRouting
	}
	if !digits(routing) {
		return "", ErrRouting
	}

	// Step 1: the routing code plus its length offset, then the tracking digits.
	v := new(big.Int)
	if routing != "" {
		v.SetString(routing, 10)
		switch len(routing) {
		case 5:
			v.Add(v, big.NewInt(1))
		case 9:
			v.Add(v, big.NewInt(100001))
		case 11:
			v.Add(v, big.NewInt(1000100001))
		}
	}
	ten, five := big.NewInt(10), big.NewInt(5)
	v.Mul(v, ten).Add(v, big.NewInt(int64(tracking[0]-'0')))
	v.Mul(v, five).Add(v, big.NewInt(int64(tracking[1]-'0')))
	for i := 2; i < 20; i++ {
		v.Mul(v, ten).Add(v, big.NewInt(int64(tracking[i]-'0')))
	}

	// Step 2: 11-bit CRC over the 102-bit value.
	var buf [13]byte
	v.FillBytes(buf[:])
	fcs := crc11(buf)

	// Step 3: codewords J (base 636) and I..B (base 1365); A is what remains.
	var cw [10]int
	q, m := new(big.Int), new(big.Int)
	q.DivMod(v, big.NewInt(636), m)
	cw[9] = int(m.Int64())
	b1365 := big.NewInt(1365)
	for i := 8; i > 0; i-- {
		q.DivMod(q, b1365, m)
		cw[i] = int(m.Int64())
	}
	cw[0] = int(q.Int64())

	// Step 4: orientation bit in J, top CRC bit in A.
	cw[9] *= 2
	if fcs&0x400 != 0 {
		cw[0] += 659
	}

	// Step 5: codewords to 13-bit characters; low CRC bits invert them.
	var ch [10]uint16
	for i, c := range cw {
		if c < 1287 {
			ch[i] = table5[c]
		} else {
			ch[i] = table2[c-1287]
		}
		if fcs&(1<<i) != 0 {
			ch[i] = 0x1FFF &^ ch[i]
		}
	}

	// Step 6: characters to bars.
	var on [130]bool
	for i := 0; i < 10; i++ {
		for j := 0; j < 13; j++ {
			on[barTable[13*i+j]-1] = ch[i]>>j&1 == 1
		}
	}
	var sb strings.Builder
	for i := 0; i < 65; i++ {
		desc, asc := on[i], on[i+65]
		switch {
		case desc && asc:
			sb.WriteByte(Full)
		case asc:
			sb.WriteByte(Ascender)
		case desc:
			sb.WriteByte(Descender)
		default:
			sb.WriteByte(Tracker)
		}
	}
	return sb.String(), nil
}

func crc11(b [13]byte) uint16 {
	const poly = 0x0F35
	fcs := uint16(0x07FF)
	step := func(data uint16, n int) {
		for ; n > 0; n-- {
			if (fcs^data)&0x400 != 0 {
				fcs = fcs<<1 ^ poly
			} else {
				fcs <<= 1
			}
			fcs &= 0x7FF
			data <<= 1
		}
	}
	step(uint16(b[0])<<5, 6) // the top 2 bits of the first byte are padding
	for _, x := range b[1:] {
		step(uint16(x)<<3, 8)
	}
	return fcs
}

// Tracking builds the 20-digit tracking code. A 9-digit mailer ID takes a
// 6-digit serial; a 6-digit mailer ID takes a 9-digit serial.
func Tracking(barcodeID, serviceType int, mailerID string, serial int) (string, error) {
	if barcodeID < 0 || barcodeID > 94 || barcodeID%10 > 4 || serviceType < 0 || serviceType > 999 || !digits(mailerID) {
		return "", ErrTracking
	}
	var s string
	switch len(mailerID) {
	case 9:
		if serial < 0 || serial > 999_999 {
			return "", errors.New("serial must be 0-999999 with a 9-digit mailer ID")
		}
		s = fmt.Sprintf("%06d", serial)
	case 6:
		if serial < 0 || serial > 999_999_999 {
			return "", errors.New("serial must be 0-999999999 with a 6-digit mailer ID")
		}
		s = fmt.Sprintf("%09d", serial)
	default:
		return "", errors.New("mailer ID must be 6 or 9 digits")
	}
	return fmt.Sprintf("%02d%03d%s%s", barcodeID, serviceType, mailerID, s), nil
}

// HumanReadable formats the full 31-digit code the way USPS prints it under
// the bars: 00 040 123456789 000001 64082491117.
func HumanReadable(tracking, routing string) string {
	if len(tracking) != 20 {
		return tracking + " " + routing
	}
	mid := 6 // 9-digit mailer IDs start with 9
	if tracking[5] == '9' {
		mid = 9
	}
	out := tracking[:2] + " " + tracking[2:5] + " " + tracking[5:5+mid] + " " + tracking[5+mid:]
	if routing != "" {
		out += " " + routing
	}
	return out
}

// SVG draws the barcode at USPS print dimensions (22 bars per inch, bars
// 0.02" wide, full bars 0.145" tall) inside a viewBox measured in
// thousandths of an inch, so it prints at true size with width="3.0in".
func SVG(bars string) string {
	const (
		pitch = 1000.0 / 22 // ~45.45 mils
		width = 20.0
		full  = 145.0
		track = 48.0
	)
	mid := full / 2
	var sb strings.Builder
	w := pitch*float64(len(bars)-1) + width
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.1f %.0f" width="%.3fin" height="%.3fin" role="img" aria-label="USPS Intelligent Mail barcode"><g fill="#000">`,
		w, full, w/1000, full/1000)
	for i, b := range bars {
		top, bottom := mid-track/2, mid+track/2
		if b == Full || b == Ascender {
			top = 0
		}
		if b == Full || b == Descender {
			bottom = full
		}
		fmt.Fprintf(&sb, `<rect x="%.2f" y="%.1f" width="%.0f" height="%.1f"/>`, float64(i)*pitch, top, width, bottom-top)
	}
	sb.WriteString(`</g></svg>`)
	return sb.String()
}
