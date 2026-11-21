package cashctrl

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

// TestRequestShapes checks what one or more representative endpoints of every
// group send: method, path, auth and the form parameters, nested JSON included.
func TestRequestShapes(t *testing.T) {
	f, c := newFake(t)
	ctx := context.Background()
	for _, ep := range []string{"account/list.json", "currency/create.json", "customfield/reorder.json", "fiscalperiod/list.json",
		"history/list.json", "inventory/article/create.json", "journal/create.json", "location/create.json",
		"order/create.json", "person/create.json", "report/element/create.json", "rounding/create.json",
		"salary/statement/list.json", "setting/update.json", "tax/create.json", "text/create.json"} {
		f.answer[ep] = `{"success":true,"insertId":1,"data":[],"total":0}`
	}
	f.answer["sequencenumber/get"] = `"RE-2026-0001"`
	f.answer["file/get"] = "binary"
	f.answer["domain/current/logo"] = "\x89PNG"

	cases := []struct {
		group, method, path string
		call                func() error
		form                map[string]string
		json                map[string]any // parameters sent as JSON, compared after decoding
		absent              []string
	}{
		{"account", "GET", "account/list.json", func() error {
			_, err := c.Account().List(ctx, &AccountListParams{CategoryID: Ptr(int64(3)), OnlyActive: Ptr(true), Dir: Ptr(Asc),
				Sort: Ptr("number"), Query: Ptr("Kasse"), FiscalPeriodID: Ptr(int64(11))})
			return err
		}, map[string]string{"categoryId": "3", "onlyActive": "true", "dir": "ASC", "sort": "number", "query": "Kasse", "fiscalPeriodId": "11"},
			nil, []string{"filter", "onlyNotes"}},
		{"currency", "POST", "currency/create.json", func() error {
			_, err := c.Currency().Create(ctx, &CurrencyCreateParams{Code: "EUR", Rate: Ptr(0.9312)})
			return err
		}, map[string]string{"code": "EUR", "rate": "0.9312"}, nil, []string{"isDefault", "description"}},
		{"customfield", "POST", "customfield/reorder.json", func() error {
			// type is undocumented but required (overrides.ts).
			_, err := c.Customfield().Reorder(ctx, &CustomfieldReorderParams{IDs: []int64{4, 5}, Target: 9, Type: CustomfieldTypePerson})
			return err
		}, map[string]string{"ids": "4,5", "target": "9", "type": "PERSON"}, nil, []string{"before"}},
		{"domain", "GET", "domain/current/logo", func() error {
			d, err := c.Domain().Current().Logo(ctx)
			if err == nil && string(d.Data) != "\x89PNG" {
				t.Errorf("logo = %q", d.Data)
			}
			return err
		}, nil, nil, nil},
		{"file", "GET", "file/get", func() error {
			_, err := c.File().Get(ctx, &FileGetParams{ID: 77})
			return err
		}, map[string]string{"id": "77"}, nil, nil},
		{"fiscalperiod", "GET", "fiscalperiod/list.json", func() error {
			_, err := c.Fiscalperiod().List(ctx)
			return err
		}, nil, nil, nil},
		{"history", "GET", "history/list.json", func() error {
			_, err := c.History().List(ctx, &HistoryListParams{OrderID: Ptr(int64(412)), Start: Ptr(0), Limit: Ptr(50)})
			return err
		}, map[string]string{"orderId": "412", "start": "0", "limit": "50"}, nil, nil},
		{"inventory", "POST", "inventory/article/create.json", func() error {
			_, err := c.Inventory().Article().Create(ctx, &InventoryArticleCreateParams{Name: "Reifen", SalesPrice: Ptr(120.5),
				IsStockArticle: Ptr(false), UnitID: Ptr("1")})
			return err
		}, map[string]string{"name": "Reifen", "salesPrice": "120.5", "isStockArticle": "false", "unitId": "1"}, nil, nil},
		{"journal", "POST", "journal/create.json", func() error {
			_, err := c.Journal().Create(ctx, &JournalCreateParams{Amount: 100, CreditID: 1, DebitID: 2, DateAdded: Ptr(NewDate(2026, 11, 20)),
				Items: []JournalItemParam{{AccountID: 5, Debit: Ptr(100.0),
					Allocations: []JournalItemAllocationParam{{Share: 1, ToCostCenterID: 8}}}}})
			return err
		}, map[string]string{"amount": "100", "creditId": "1", "debitId": "2", "dateAdded": "2026-11-20"},
			map[string]any{"items": []any{map[string]any{"accountId": 5.0, "debit": 100.0,
				"allocations": []any{map[string]any{"share": 1.0, "toCostCenterId": 8.0}}}}}, []string{"title"}},
		{"location", "POST", "location/create.json", func() error {
			_, err := c.Location().Create(ctx, &LocationCreateParams{Name: "Biel", Country: Ptr(LocationCountry("CHE")), VatUID: Ptr("CHE-1")})
			return err
		}, map[string]string{"name": "Biel", "country": "CHE", "vatUid": "CHE-1"}, nil, nil},
		{"order", "POST", "order/create.json", func() error {
			_, err := c.Order().Create(ctx, &OrderCreateParams{AssociateID: []int64{8}, CategoryID: 4, Date: NewDate(2026, 11, 20),
				DueDays: Ptr(30), IsDisplayItemGross: Ptr(false), Language: Ptr(OrderLanguage("DE")), Notes: Ptr("libretto:abc"),
				Items: []OrderItemParam{{AccountID: 15, Name: "Fleet", UnitPrice: 1790, Quantity: Ptr(1.0), TaxID: Ptr(int64(2)), UnitID: 3}}})
			return err
		}, map[string]string{"associateId": "8", "categoryId": "4", "date": "2026-11-20", "dueDays": "30", "isDisplayItemGross": "false",
			"language": "DE", "notes": "libretto:abc"},
			map[string]any{"items": []any{map[string]any{"accountId": 15.0, "name": "Fleet", "unitPrice": 1790.0, "quantity": 1.0,
				"taxId": 2.0, "unitId": 3.0}}}, []string{"statusId", "description"}},
		{"person", "POST", "person/create.json", func() error {
			_, err := c.Person().Create(ctx, &PersonCreateParams{Company: Ptr("Muster AG"), Nr: Ptr("LIB-1"), IsCustomer: Ptr(true),
				Addresses: []PersonAddressParam{{Type: PersonAddressTypeMain, Address: Ptr("Bahnhofstrasse 1"), Zip: Ptr("8001"),
					City: Ptr("Zürich"), Country: Ptr(PersonAddressCountry("CHE"))}},
				Contacts: []PersonContactParam{{Type: PersonContactTypeEmailInvoice, Address: "rechnung@muster.ch"}}})
			return err
		}, map[string]string{"company": "Muster AG", "nr": "LIB-1", "isCustomer": "true"},
			map[string]any{
				"addresses": []any{map[string]any{"type": "MAIN", "address": "Bahnhofstrasse 1", "zip": "8001", "city": "Zürich", "country": "CHE"}},
				"contacts":  []any{map[string]any{"type": "EMAIL_INVOICE", "address": "rechnung@muster.ch"}},
			}, []string{"firstName", "lastName"}},
		{"report", "POST", "report/element/create.json", func() error {
			_, err := c.Report().Element().Create(ctx, &ReportElementCreateParams{Type: ReportElementType("JOURNAL"),
				Config: map[string]any{"accounts": []int{1, 2}}})
			return err
		}, map[string]string{"type": "JOURNAL"}, map[string]any{"config": map[string]any{"accounts": []any{1.0, 2.0}}}, nil},
		{"rounding", "POST", "rounding/create.json", func() error {
			_, err := c.Rounding().Create(ctx, &RoundingCreateParams{AccountID: 6, Name: "5 Rappen", Rounding: 0.05})
			return err
		}, map[string]string{"accountId": "6", "name": "5 Rappen", "rounding": "0.05"}, nil, []string{"mode"}},
		{"salary", "GET", "salary/statement/list.json", func() error {
			_, err := c.Salary().Statement().List(ctx, &SalaryStatementListParams{Filter: []Filter{Where("personId", Eq, 4), {Field: "name", Value: "x"}},
				OnlyOpen: Ptr(true)})
			return err
		}, map[string]string{"onlyOpen": "true"},
			map[string]any{"filter": []any{map[string]any{"field": "personId", "comparison": "eq", "value": 4.0},
				map[string]any{"field": "name", "value": "x"}}}, nil},
		{"sequencenumber", "GET", "sequencenumber/get", func() error {
			_, err := c.Sequencenumber().Get(ctx, &SequencenumberGetParams{Type: Ptr(SequencenumberTypeOrder), CategoryID: Ptr(int64(4))})
			return err
		}, map[string]string{"type": "ORDER", "categoryId": "4"}, nil, []string{"id"}},
		{"setting", "POST", "setting/update.json", func() error {
			_, err := c.Setting().Update(ctx, map[string]string{"THOUSAND_SEPARATOR": "'"})
			return err
		}, map[string]string{"THOUSAND_SEPARATOR": "'"}, nil, nil},
		{"tax", "POST", "tax/create.json", func() error {
			_, err := c.Tax().Create(ctx, &TaxCreateParams{Code: "UN81",
				Components: []TaxComponentParam{{AccountID: 9, ApplyRule: TaxComponentApplyRule("SALES"), CalcType: TaxComponentCalcTypeNet, Code: Ptr("302")}},
				Rates:      []TaxRateParam{{Percentage: 8.1}}})
			return err
		}, map[string]string{"code": "UN81"},
			map[string]any{"components": []any{map[string]any{"accountId": 9.0, "applyRule": "SALES", "calcType": "NET", "code": "302"}},
				"rates": []any{map[string]any{"percentage": 8.1}}}, nil},
		{"text", "POST", "text/create.json", func() error {
			_, err := c.Text().Create(ctx, &TextCreateParams{Name: "Gruss", Type: TextType("ORDER_ITEM"), Value: Ptr("<p>Hallo</p>")})
			return err
		}, map[string]string{"name": "Gruss", "type": "ORDER_ITEM", "value": "<p>Hallo</p>"}, nil, nil},
	}
	groups := map[string]bool{}
	for _, tc := range cases {
		groups[tc.group] = true
		if err := tc.call(); err != nil {
			t.Errorf("%s: %v", tc.path, err)
			continue
		}
		r := f.last(t, tc.path)
		if r.Method != tc.method || r.User != "key123" || r.Pass != "" {
			t.Errorf("%s: %s as %q:%q", tc.path, r.Method, r.User, r.Pass)
		}
		if tc.method == http.MethodPost && r.ContentType != "application/x-www-form-urlencoded" {
			t.Errorf("%s: content type %q", tc.path, r.ContentType)
		}
		for k, v := range tc.form {
			if r.Form.Get(k) != v {
				t.Errorf("%s: %s = %q, want %q", tc.path, k, r.Form.Get(k), v)
			}
		}
		for k, want := range tc.json {
			var got any
			if err := json.Unmarshal([]byte(r.Form.Get(k)), &got); err != nil || !reflect.DeepEqual(got, want) {
				t.Errorf("%s: %s = %s, want %v", tc.path, k, r.Form.Get(k), want)
			}
		}
		for _, k := range tc.absent {
			if r.Form.Has(k) {
				t.Errorf("%s: %s sent although not set", tc.path, k)
			}
		}
		want := len(tc.form) + len(tc.json)
		if got := len(r.Form); got != want {
			t.Errorf("%s: %d parameters sent, want %d: %v", tc.path, got, want, r.Form)
		}
	}
	for _, e := range generatedEndpoints {
		g := e.Path[:indexByte(e.Path, '/')]
		if !groups[g] {
			t.Errorf("group %s has no request-shape case", g)
			groups[g] = true
		}
	}
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return len(s)
}
