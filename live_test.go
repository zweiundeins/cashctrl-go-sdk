package cashctrl

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLive reads from a real organisation. It runs only with
// CASHCTRL_LIVE_TEST=1 and CASHCTRL_ORG and CASHCTRL_APIKEY set, and calls
// read endpoints only: it never writes, and it skips the GETs with side
// effects (fiscalperiod/reopen_months.json, sequencenumber/get) and the
// document reads, which CashCtrl logs as downloads.
func TestLive(t *testing.T) {
	if os.Getenv("CASHCTRL_LIVE_TEST") != "1" {
		t.Skip("set CASHCTRL_LIVE_TEST=1, CASHCTRL_ORG and CASHCTRL_APIKEY to read from a real organisation")
	}
	c := &Client{Org: os.Getenv("CASHCTRL_ORG"), APIKey: os.Getenv("CASHCTRL_APIKEY"), Lang: LangEN, MaxRetries: 5}
	if c.Org == "" || c.APIKey == "" {
		t.Fatal("CASHCTRL_ORG and CASHCTRL_APIKEY must be set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	periods, err := c.Fiscalperiod().List(ctx)
	if err != nil {
		t.Fatalf("fiscalperiod/list.json: %v", err)
	}
	t.Logf("%d fiscal periods", len(periods.Data))
	if _, err := c.Account().List(ctx, &AccountListParams{OnlyActive: Ptr(true)}); err != nil {
		t.Errorf("account/list.json: %v", err)
	}
	if _, err := c.Account().Category().Tree(ctx, nil); err != nil {
		t.Errorf("account/category/tree.json: %v", err)
	}
	taxes, err := c.Tax().List(ctx)
	if err != nil {
		t.Errorf("tax/list.json: %v", err)
	} else if len(taxes.Data) > 0 {
		if _, err := c.Tax().Read(ctx, &TaxReadParams{ID: taxes.Data[0].ID}); err != nil {
			t.Errorf("tax/read.json: %v", err)
		}
	}
	people, err := c.Person().List(ctx, &PersonListParams{Limit: Ptr(5)})
	if err != nil {
		t.Errorf("person/list.json: %v", err)
	} else {
		t.Logf("%d persons, %d on the first page", people.Total, len(people.Data))
	}
	cats, err := c.Order().Category().List(ctx, &OrderCategoryListParams{Type: Ptr(OrderCategoryType("SALES"))})
	if err != nil {
		t.Errorf("order/category/list.json: %v", err)
	} else if len(cats.Data) > 0 {
		if _, err := c.Order().Category().Read(ctx, &OrderCategoryReadParams{ID: cats.Data[0].ID}); err != nil {
			t.Errorf("order/category/read.json: %v", err)
		}
	}
	orders, err := c.Order().List(ctx, &OrderListParams{Type: Ptr(OrderTypeSales), Limit: Ptr(3)})
	if err != nil {
		t.Errorf("order/list.json: %v", err)
	} else if len(orders.Data) > 0 {
		if _, err := c.Order().Read(ctx, &OrderReadParams{ID: orders.Data[0].ID}); err != nil {
			t.Errorf("order/read.json: %v", err)
		}
	}
	if _, err := c.Setting().Read(ctx); err != nil {
		t.Errorf("setting/read.json: %v", err)
	}
}
