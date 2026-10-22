// Package jsnum reproduces JavaScript's number conversions, which Strapi's
// behaviour (and its error messages) depends on.
package jsnum

import (
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const whitespace = " \t\n\r\v\f\u00a0\ufeff\u2028\u2029"

// Number is JavaScript's Number(v) for JSON values.
func Number(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case bool:
		if x {
			return 1
		}
		return 0
	case nil:
		return 0
	case string:
		return numberString(x)
	}
	return math.NaN()
}

func numberString(s string) float64 {
	t := strings.Trim(s, whitespace)
	switch t {
	case "":
		return 0
	case "Infinity", "+Infinity":
		return math.Inf(1)
	case "-Infinity":
		return math.Inf(-1)
	}
	if len(t) > 2 && t[0] == '0' {
		base := map[byte]int{'x': 16, 'X': 16, 'o': 8, 'O': 8, 'b': 2, 'B': 2}[t[1]]
		if base != 0 {
			if strings.Contains(t, "_") {
				return math.NaN()
			}
			if n, err := strconv.ParseUint(t[2:], base, 64); err == nil {
				return float64(n)
			}
			return math.NaN()
		}
	}
	for _, c := range t {
		if !strings.ContainsRune("0123456789+-.eE", c) {
			return math.NaN()
		}
	}
	f, err := strconv.ParseFloat(t, 64)
	if err == nil || errors.Is(err, strconv.ErrRange) {
		return f
	}
	return math.NaN()
}

var floatPrefix = regexp.MustCompile(`^[+-]?(Infinity|(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?)`)

// ParseFloat is JavaScript's parseFloat: the longest numeric prefix.
func ParseFloat(s string) float64 {
	m := floatPrefix.FindString(strings.TrimLeft(s, whitespace))
	switch strings.TrimLeft(m, "+-") {
	case "":
		return math.NaN()
	case "Infinity":
		if strings.HasPrefix(m, "-") {
			return math.Inf(-1)
		}
		return math.Inf(1)
	}
	f, _ := strconv.ParseFloat(m, 64)
	return f
}

// ParseInt is JavaScript's parseInt(s, 10).
func ParseInt(s string) float64 {
	t := strings.TrimLeft(s, whitespace)
	end := 0
	if end < len(t) && (t[end] == '+' || t[end] == '-') {
		end++
	}
	digits := end
	for end < len(t) && t[end] >= '0' && t[end] <= '9' {
		end++
	}
	if end == digits {
		return math.NaN()
	}
	f, _ := strconv.ParseFloat(t[:end], 64)
	return f
}

// IsInteger is Number.isInteger.
func IsInteger(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0) && f == math.Trunc(f)
}

// String is JavaScript's String(number).
func String(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case f == 0:
		return "0"
	}
	abs := math.Abs(f)
	if abs >= 1e-6 && abs < 1e21 {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	s := strconv.FormatFloat(f, 'e', -1, 64)
	mant, exp, _ := strings.Cut(s, "e")
	sign := exp[0]
	exp = strings.TrimLeft(exp[1:], "0")
	return mant + "e" + string(sign) + exp
}
