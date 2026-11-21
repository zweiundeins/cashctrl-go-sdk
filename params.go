package cashctrl

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Ptr returns a pointer to v, for the optional fields of the params structs:
// a nil field is not sent, a pointer to the zero value is (false, 0, "").
// CashCtrl's update endpoints clear every optional field left out.
func Ptr[T any](v T) *T { return &v }

// Date is a calendar day, sent as YYYY-MM-DD.
type Date struct{ time.Time }

// DateOf is the day of t in t's location.
func DateOf(t time.Time) Date { return Date{t} }

// NewDate is the given day.
func NewDate(year int, month time.Month, day int) Date {
	return Date{time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

// ParseDate reads YYYY-MM-DD, and the date part of a longer timestamp such as
// the "2026-11-20 00:00:00.0" CashCtrl answers.
func ParseDate(s string) (Date, error) {
	if len(s) < 10 {
		return Date{}, fmt.Errorf("cashctrl: %q is no date", s)
	}
	t, err := time.Parse("2006-01-02", s[:10])
	return Date{t}, err
}

func (d Date) String() string { return d.Format("2006-01-02") }

// MarshalJSON writes "YYYY-MM-DD", for dates inside JSON parameters.
func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// UnmarshalJSON reads what ParseDate reads.
func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s == "" {
		*d = Date{}
		return nil
	}
	v, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// Comparison is how a Filter compares a column with its value.
type Comparison string

// The comparisons of the filter parameter of the list endpoints.
const (
	Eq   Comparison = "eq"
	Like Comparison = "like" // the default
	Gt   Comparison = "gt"
	Lt   Comparison = "lt"
)

// Filter is one condition of a list endpoint's filter parameter. All
// conditions of a list must match.
type Filter struct {
	// Field is the column, as the list answers it ("nr", "notes").
	Field string `json:"field"`
	// Comparison is empty for like.
	Comparison Comparison `json:"comparison,omitempty"`
	// Value is compared with the column: a string, number or boolean, or a
	// slice of them, of which one must match.
	Value any `json:"value"`
}

// Where is a filter on field with comparison c.
func Where(field string, c Comparison, value any) Filter {
	return Filter{Field: field, Comparison: c, Value: value}
}

// SortDir is the dir parameter of the list endpoints.
type SortDir string

// The sort directions.
const (
	Asc  SortDir = "ASC"
	Desc SortDir = "DESC"
)

// form builds the parameters of one request.
type form struct {
	v   url.Values
	err error
}

func newForm() *form { return &form{v: url.Values{}} }

func (f *form) str(k, v string)     { f.v.Set(k, v) }
func (f *form) int(k string, v int) { f.v.Set(k, strconv.Itoa(v)) }
func (f *form) id(k string, v int64) {
	f.v.Set(k, strconv.FormatInt(v, 10))
}
func (f *form) num(k string, v float64) { f.v.Set(k, strconv.FormatFloat(v, 'f', -1, 64)) }
func (f *form) bool(k string, v bool)   { f.v.Set(k, strconv.FormatBool(v)) }
func (f *form) date(k string, v Date)   { f.v.Set(k, v.String()) }

func (f *form) ids(k string, v []int64) {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = strconv.FormatInt(n, 10)
	}
	f.v.Set(k, strings.Join(parts, ","))
}

func (f *form) strs(k string, v []string) { f.v.Set(k, strings.Join(v, ",")) }

func (f *form) json(k string, v any) {
	b, err := json.Marshal(v)
	if err != nil && f.err == nil {
		f.err = fmt.Errorf("cashctrl: parameter %s: %w", k, err)
	}
	f.v.Set(k, string(b))
}

func (f *form) open(m map[string]string) {
	for k, v := range m {
		f.v.Set(k, v)
	}
}

func (f *form) done() (url.Values, error) { return f.v, f.err }
