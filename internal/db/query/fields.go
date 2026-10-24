package query

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aluzed/grapi/schema"
)

// Errors of value conversion, the ValidationErrors of Strapi's fields.
var (
	ErrInvalidDateTime = errors.New("Invalid format, expected a timestamp or an ISO date") //nolint:revive,staticcheck // Strapi's message
	ErrInvalidDate     = errors.New("Invalid format, expected yyyy-MM-dd")                 //nolint:revive,staticcheck // Strapi's message
	ErrInvalidTime     = errors.New("Invalid time format, expected HH:mm:ss.SSS")          //nolint:revive,staticcheck // Strapi's message
)

// toDB converts a value to what is stored for an attribute type, like
// Strapi's field classes. sqlite tells how datetimes are stored there
// (epoch milliseconds).
func toDB(attrType string, v any, sqlite bool) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch attrType {
	case schema.TypeString, schema.TypeText, schema.TypeRichText, schema.TypeEmail, schema.TypePassword,
		schema.TypeUID, schema.TypeEnumeration:
		return jsString(v), nil
	case schema.TypeJSON, schema.TypeBlocks:
		// Any Go list or object, not only decoded JSON: Go hooks write
		// []int64 or structs.
		switch reflect.ValueOf(v).Kind() {
		case reflect.Map, reflect.Slice, reflect.Array, reflect.Struct, reflect.Pointer:
			b, err := json.Marshal(v)
			return string(b), err
		}
		return v, nil
	case schema.TypeBigInteger:
		return bigIntString(v)
	case schema.TypeInteger, schema.TypeFloat, schema.TypeDecimal:
		return toNumber(v)
	case schema.TypeBoolean:
		return toBool(v), nil
	case schema.TypeDate:
		return parseDate(v)
	case schema.TypeTime:
		return parseTime(v)
	case schema.TypeDateTime, schema.TypeTimestamp:
		t, err := parseDateTime(v)
		if err != nil {
			return nil, err
		}
		if sqlite {
			return t.UnixMilli(), nil
		}
		// A JavaScript Date holds milliseconds: Strapi never stores finer.
		return t.Truncate(time.Millisecond), nil
	}
	return v, nil
}

// fromDB converts a stored value to what Strapi returns for the type.
func fromDB(attrType string, v any) any {
	if v == nil {
		return nil
	}
	switch attrType {
	case schema.TypeString, schema.TypeText, schema.TypeRichText, schema.TypeEmail, schema.TypePassword,
		schema.TypeUID, schema.TypeEnumeration:
		return jsString(v)
	case schema.TypeJSON, schema.TypeBlocks:
		s, ok := asString(v)
		if !ok {
			return v
		}
		var out any
		if err := json.Unmarshal([]byte(s), &out); err != nil {
			return s
		}
		// Values stringified twice by Strapi 5 before rc.7.
		if inner, ok := out.(string); ok {
			var again any
			if json.Unmarshal([]byte(inner), &again) == nil {
				return again
			}
		}
		return out
	case schema.TypeBigInteger:
		s, err := bigIntString(v)
		if err != nil {
			return jsString(v)
		}
		return s
	case schema.TypeInteger, schema.TypeFloat, schema.TypeDecimal:
		return lodashToNumber(v)
	case schema.TypeBoolean:
		if b, ok := v.(bool); ok {
			return b
		}
		switch jsString(v) {
		case "1":
			return true
		case "0":
			return false
		}
		return nil
	case schema.TypeDate, schema.TypeTime:
		if s, ok := asString(v); ok {
			return s
		}
		if t, ok := v.(time.Time); ok {
			if attrType == schema.TypeDate {
				return t.Format("2006-01-02")
			}
			return t.Format("15:04:05")
		}
		return v
	case schema.TypeDateTime, schema.TypeTimestamp:
		t, ok := storedTime(v)
		if !ok {
			return nil
		}
		if attrType == schema.TypeTimestamp {
			return strconv.FormatInt(t.UnixMilli(), 10)
		}
		return t.UTC().Format("2006-01-02T15:04:05.000Z")
	}
	return v
}

func asString(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case []byte:
		return string(x), true
	}
	return "", false
}

// storedTime reads a datetime column: a time from Postgres, epoch milliseconds
// (or a string) from SQLite.
func storedTime(v any) (time.Time, bool) {
	switch x := v.(type) {
	case time.Time:
		return x, true
	case int64:
		return time.UnixMilli(x).UTC(), true
	case float64:
		return time.UnixMilli(int64(x)).UTC(), true
	}
	if s, ok := asString(v); ok {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return time.UnixMilli(n).UTC(), true
		}
		if t, err := parseISO(s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// jsString is lodash's toString.
func jsString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []byte:
		return string(x)
	case bool:
		return strconv.FormatBool(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	case float64:
		return formatJSNumber(x)
	case time.Time:
		return x.String()
	case []any:
		parts := make([]string, len(x))
		for i, p := range x {
			parts[i] = jsString(p)
		}
		return strings.Join(parts, ",")
	}
	return fmt.Sprint(v)
}

// formatJSNumber writes a float like JavaScript's Number#toString.
func formatJSNumber(f float64) string {
	if f == math.Trunc(f) && math.Abs(f) < 1e21 {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

var bigIntPattern = regexp.MustCompile(`^[+-]?\d+$`)

func bigIntString(v any) (string, error) {
	switch x := v.(type) {
	case int64:
		return strconv.FormatInt(x, 10), nil
	case int:
		return strconv.Itoa(x), nil
	case float64:
		if math.IsInf(x, 0) || math.IsNaN(x) || x != math.Trunc(x) {
			return "", fmt.Errorf("Expected a valid BigInteger, got %s", formatJSNumber(x)) //nolint:revive,staticcheck // Strapi's message
		}
		return new(big.Float).SetFloat64(x).Text('f', 0), nil
	case string:
		s := strings.TrimSpace(x)
		if !bigIntPattern.MatchString(s) {
			return "", fmt.Errorf("Expected a valid BigInteger, got %s", x) //nolint:revive,staticcheck // Strapi's message
		}
		n, _ := new(big.Int).SetString(strings.TrimPrefix(s, "+"), 10)
		return n.String(), nil
	case []byte:
		return bigIntString(string(x))
	}
	return "", fmt.Errorf("Expected a valid BigInteger, got %v", v) //nolint:revive,staticcheck // Strapi's message
}

func toNumber(v any) (any, error) {
	switch x := v.(type) {
	case int64, int:
		return x, nil
	case float64:
		if math.IsInf(x, 0) || math.IsNaN(x) {
			return nil, fmt.Errorf("Expected a valid Number, got %s", formatJSNumber(x)) //nolint:revive,staticcheck // Strapi's message
		}
		return x, nil
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return nil, fmt.Errorf("Expected a valid Number, got %s", x) //nolint:revive,staticcheck // Strapi's message
		}
		f, ok := jsNumber(s)
		if !ok || math.IsInf(f, 0) {
			return nil, fmt.Errorf("Expected a valid Number, got %s", x) //nolint:revive,staticcheck // Strapi's message
		}
		return f, nil
	}
	return nil, fmt.Errorf("Expected a valid Number, got %v", v) //nolint:revive,staticcheck // Strapi's message
}

// jsNumber parses like JavaScript's Number(string) on a trimmed, non empty string.
func jsNumber(s string) (float64, bool) {
	lower := strings.ToLower(s)
	for _, p := range []struct {
		prefix string
		base   int
	}{{"0x", 16}, {"0o", 8}, {"0b", 2}} {
		if strings.HasPrefix(lower, p.prefix) {
			n, err := strconv.ParseUint(s[2:], p.base, 64)
			return float64(n), err == nil
		}
	}
	switch s {
	case "Infinity", "+Infinity":
		return math.Inf(1), true
	case "-Infinity":
		return math.Inf(-1), true
	}
	if strings.ContainsAny(lower, "_xn") {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}

// lodashToNumber is lodash's toNumber, used when reading numbers back.
func lodashToNumber(v any) any {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return x
	case bool:
		if x {
			return int64(1)
		}
		return int64(0)
	}
	if s, ok := asString(v); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return int64(0)
		}
		if f, ok := jsNumber(s); ok {
			if f == math.Trunc(f) && math.Abs(f) < 1<<53 {
				return int64(f)
			}
			return f
		}
		return math.NaN()
	}
	return v
}

func toBool(v any) any {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		switch x {
		case "true", "t", "1":
			return true
		case "false", "f", "0":
			return false
		}
		return x != ""
	case int64:
		return x != 0
	case int:
		return x != 0
	case float64:
		switch x {
		case 1:
			return true
		case 0:
			return false
		}
		return !math.IsNaN(x)
	}
	return true
}

var (
	partialDateRegex = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])`)
	timeRegex        = regexp.MustCompile(`^(2[0-3]|[01][0-9]):([0-5][0-9]):([0-5][0-9])(\.[0-9]{1,3})?$`)
)

func parseDate(v any) (any, error) {
	if t, ok := v.(time.Time); ok {
		return t.In(time.Local).Format("2006-01-02"), nil
	}
	s, ok := v.(string)
	if !ok {
		return nil, ErrInvalidDate
	}
	found := partialDateRegex.FindString(s)
	if found == "" {
		return nil, ErrInvalidDate
	}
	if _, err := time.Parse("2006-01-02", found); err != nil {
		return nil, errors.New("Invalid date") //nolint:revive,staticcheck // Strapi's message
	}
	return found, nil
}

func parseTime(v any) (any, error) {
	if t, ok := v.(time.Time); ok {
		return t.In(time.Local).Format("15:04:05.000"), nil
	}
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("Expected a string, got a %s", jsTypeOf(v)) //nolint:revive,staticcheck // Strapi's message
	}
	m := timeRegex.FindStringSubmatch(s)
	if m == nil {
		return nil, ErrInvalidTime
	}
	fraction := m[4]
	if fraction == "" {
		fraction = ".000"
	}
	frac := fraction[1:] + strings.Repeat("0", 3-len(fraction[1:]))
	return m[1] + ":" + m[2] + ":" + m[3] + "." + frac, nil
}

func jsTypeOf(v any) string {
	switch v.(type) {
	case float64, int64, int:
		return "number"
	case bool:
		return "boolean"
	case nil:
		return "object"
	case map[string]any, []any:
		return "object"
	}
	return "string"
}

func parseDateTime(v any) (time.Time, error) {
	if t, ok := v.(time.Time); ok {
		return t, nil
	}
	s := jsString(v)
	if t, err := parseISO(s); err == nil {
		return t, nil
	}
	// date-fns' "T" pattern: epoch milliseconds.
	if n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil && strings.TrimSpace(s) == s {
		return time.UnixMilli(n), nil
	}
	return time.Time{}, ErrInvalidDateTime
}

var isoPattern = regexp.MustCompile(`^([+-]?\d{4,6})(?:-(\d{2})(?:-(\d{2}))?)?(?:[T ](\d{2})(?::?(\d{2})(?::?(\d{2})(?:[.,](\d+))?)?)?)?\s*(Z|[+-]\d{2}(?::?\d{2})?)?$`)

// parseISO follows date-fns' parseISO for the forms an API receives: a date,
// a date and time, an optional offset. Without an offset the time is local,
// like date-fns.
func parseISO(s string) (time.Time, error) {
	m := isoPattern.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}, ErrInvalidDateTime
	}
	num := func(x string, def int) int {
		if x == "" {
			return def
		}
		n, _ := strconv.Atoi(x)
		return n
	}
	year := num(m[1], 0)
	month, day := num(m[2], 1), num(m[3], 1)
	hour, minute, sec := num(m[4], 0), num(m[5], 0), num(m[6], 0)
	nsec := 0
	if m[7] != "" {
		frac := (m[7] + "000000000")[:9]
		nsec = num(frac, 0)
	}
	if month < 1 || month > 12 || day < 1 || day > daysIn(year, month) || hour > 24 || minute > 59 || sec > 59 {
		return time.Time{}, ErrInvalidDateTime
	}
	loc := time.Local
	switch {
	case m[8] == "Z":
		loc = time.UTC
	case m[8] != "":
		off := strings.ReplaceAll(m[8][1:], ":", "")
		h := num(off[:2], 0)
		mi := 0
		if len(off) >= 4 {
			mi = num(off[2:4], 0)
		}
		secs := h*3600 + mi*60
		if m[8][0] == '-' {
			secs = -secs
		}
		loc = time.FixedZone("", secs)
	case m[4] == "":
		// A date alone is local midnight in date-fns too.
	}
	return time.Date(year, time.Month(month), day, hour, minute, sec, nsec, loc), nil
}

func daysIn(year, month int) int {
	return time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
