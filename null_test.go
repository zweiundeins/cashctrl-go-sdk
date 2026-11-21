package cashctrl

import (
	"encoding/json"
	"testing"
)

// CashCtrl's demo organisation answers account/list.json with currencyId as a
// string, which the probe only ever saw as null.
func TestAccountCurrencyIDDecodesLoosely(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		want  int64
		valid bool
	}{
		{`"1"`, 1, true},
		{`1`, 1, true},
		{`""`, 0, false},
		{`null`, 0, false},
	} {
		var a Account
		if err := json.Unmarshal([]byte(`{"id":7,"currencyId":`+tc.raw+`}`), &a); err != nil {
			t.Errorf("currencyId %s: %v", tc.raw, err)
			continue
		}
		if a.ID != 7 || a.CurrencyID.Int64 != tc.want || a.CurrencyID.Valid != tc.valid {
			t.Errorf("currencyId %s: got %+v", tc.raw, a.CurrencyID)
		}
		if (a.CurrencyID.Ptr() != nil) != tc.valid {
			t.Errorf("currencyId %s: Ptr() = %v", tc.raw, a.CurrencyID.Ptr())
		}
	}
	var a Account
	if err := json.Unmarshal([]byte(`{"currencyId":"CHF"}`), &a); err == nil {
		t.Errorf("currencyId \"CHF\" decoded as %+v", a.CurrencyID)
	}
}

func TestNullFloat64(t *testing.T) {
	for raw, want := range map[string]NullFloat64{
		`"2.5"`: {2.5, true}, `2.5`: {2.5, true}, `""`: {}, `null`: {},
	} {
		var n NullFloat64
		if err := json.Unmarshal([]byte(raw), &n); err != nil || n != want {
			t.Errorf("%s: got %+v, %v", raw, n, err)
		}
		out, _ := json.Marshal(n)
		if !want.Valid && string(out) != "null" || want.Valid && string(out) != "2.5" {
			t.Errorf("%s: marshals to %s", raw, out)
		}
	}
}
