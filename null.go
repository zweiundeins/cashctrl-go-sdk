package cashctrl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// NullInt64 is an integer response field whose type the probe never saw (it
// was null in every sample). It accepts a JSON number, a numeric string, an
// empty string and null; Valid is false for the last two.
type NullInt64 struct {
	Int64 int64
	Valid bool
}

// Ptr returns the value, or nil when it is not valid.
func (n NullInt64) Ptr() *int64 {
	if !n.Valid {
		return nil
	}
	return &n.Int64
}

// MarshalJSON writes the number, or null.
func (n NullInt64) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return strconv.AppendInt(nil, n.Int64, 10), nil
}

// UnmarshalJSON reads 1, "1", "" and null.
func (n *NullInt64) UnmarshalJSON(b []byte) error {
	s, ok, err := looseNumber(b)
	if err != nil || !ok {
		*n = NullInt64{}
		return err
	}
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		*n = NullInt64{Int64: v, Valid: true}
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f != math.Trunc(f) || math.Abs(f) > 1<<53 {
		*n = NullInt64{}
		return fmt.Errorf("cashctrl: %s is no integer", b)
	}
	*n = NullInt64{Int64: int64(f), Valid: true}
	return nil
}

// NullFloat64 is a decimal response field whose type the probe never saw. It
// accepts what NullInt64 accepts, decimals included.
type NullFloat64 struct {
	Float64 float64
	Valid   bool
}

// Ptr returns the value, or nil when it is not valid.
func (n NullFloat64) Ptr() *float64 {
	if !n.Valid {
		return nil
	}
	return &n.Float64
}

// MarshalJSON writes the number, or null.
func (n NullFloat64) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(n.Float64)
}

// UnmarshalJSON reads 1.5, "1.5", "" and null.
func (n *NullFloat64) UnmarshalJSON(b []byte) error {
	s, ok, err := looseNumber(b)
	if err != nil || !ok {
		*n = NullFloat64{}
		return err
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		*n = NullFloat64{}
		return fmt.Errorf("cashctrl: %s is no number", b)
	}
	*n = NullFloat64{Float64: f, Valid: true}
	return nil
}

// looseNumber returns the digits of a JSON number or string, and false for
// null and an empty string.
func looseNumber(b []byte) (string, bool, error) {
	b = bytes.TrimSpace(b)
	switch {
	case bytes.Equal(b, []byte("null")):
		return "", false, nil
	case len(b) > 0 && b[0] == '"':
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return "", false, err
		}
		s = strings.TrimSpace(s)
		return s, s != "", nil
	case len(b) > 0 && (b[0] == '-' || (b[0] >= '0' && b[0] <= '9')):
		return string(b), true, nil
	}
	return "", false, fmt.Errorf("cashctrl: %s is no number", b)
}
