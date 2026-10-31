package imb

import (
	"encoding/csv"
	"os"
	"strings"
	"testing"
)

// The USPS spec's own example.
func TestSpecExample(t *testing.T) {
	got, err := Encode("01234567094987654321", "01234567891")
	if err != nil {
		t.Fatal(err)
	}
	if want := "AADTFFDFTDADTAADAATFDTDDAAADDTDTTDAFADADDDTFFFDDTTTADFAAADFTDAADA"; got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

// USPS's published encoder test cases (the successful ones), as
// redistributed in the zint barcode project's test data.
func TestUSPSCases(t *testing.T) {
	f, err := os.Open("testdata/usps_cases.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		got, err := Encode(r[1], r[2])
		if err != nil || got != r[3] {
			t.Fatalf("case %s: %v\ngot  %s\nwant %s", r[0], err, got, r[3])
		}
	}
	if len(rows) < 1000 {
		t.Fatalf("only %d cases", len(rows))
	}
}

func TestTrackingAndErrors(t *testing.T) {
	tr, err := Tracking(0, 40, "901234567", 12)
	if err != nil || tr != "00040901234567000012" {
		t.Fatal(tr, err)
	}
	if h := HumanReadable(tr, "64082491111"); h != "00 040 901234567 000012 64082491111" {
		t.Fatal(h)
	}
	if _, err := Tracking(0, 40, "901234567", 1_000_000); err == nil {
		t.Fatal("serial too big accepted")
	}
	if _, err := Encode("05040901234567000012", "64082"); err != ErrTracking {
		t.Fatal("barcode id digit 5 accepted")
	}
	if _, err := Encode(tr, "6408"); err != ErrRouting {
		t.Fatal("4-digit zip accepted")
	}
	bars, _ := Encode(tr, "64082")
	if svg := SVG(bars); strings.Count(svg, "<rect") != 65 {
		t.Fatal("expected 65 bars")
	}
}
