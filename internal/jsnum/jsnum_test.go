package jsnum

import (
	"math"
	"testing"
)

func TestNumber(t *testing.T) {
	cases := map[string]float64{
		"": 0, "  12 ": 12, "0x10": 16, "0b11": 3, "1e3": 1000, "-Infinity": math.Inf(-1),
		".5": 0.5, "5.": 5, "+3": 3,
	}
	for in, want := range cases {
		if got := Number(in); got != want {
			t.Errorf("Number(%q) = %v, want %v", in, got, want)
		}
	}
	for _, in := range []string{"12abc", "inf", "NaN", "1_000", "0x", "--1", "infinity"} {
		if got := Number(in); !math.IsNaN(got) {
			t.Errorf("Number(%q) = %v, want NaN", in, got)
		}
	}
}

func TestParse(t *testing.T) {
	if got := ParseFloat("12.5kg"); got != 12.5 {
		t.Errorf("ParseFloat = %v", got)
	}
	if got := ParseFloat("-.5e1x"); got != -5 {
		t.Errorf("ParseFloat = %v", got)
	}
	if got := ParseInt(" 42.9"); got != 42 {
		t.Errorf("ParseInt = %v", got)
	}
	if !math.IsNaN(ParseInt("x1")) || !math.IsNaN(ParseFloat("e5")) {
		t.Error("want NaN")
	}
}

func TestString(t *testing.T) {
	cases := map[float64]string{
		1e21: "1e+21", 1e20: "100000000000000000000", 1.5e-7: "1.5e-7", 1e-7: "1e-7", 0.000001: "0.000001",
		-2.5: "-2.5", 123456789012: "123456789012", math.Copysign(0, -1): "0",
	}
	for in, want := range cases {
		if got := String(in); got != want {
			t.Errorf("String(%v) = %q, want %q", in, got, want)
		}
	}
}
