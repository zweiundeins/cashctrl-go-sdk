package cashctrl

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLive reads from a disposable test organisation. It runs only with
// CASHCTRL_TEST_DOMAINID and CASHCTRL_TEST_APIKEY set and CASHCTRL_LIVE_TEST
// repeating the organisation's name, and calls read endpoints only: it never
// writes, and it skips the GETs with side effects (fiscalperiod/reopen_months.json,
// sequencenumber/get) and the document reads, which CashCtrl logs as downloads.
func TestLive(t *testing.T) {
	c := liveClient(t)
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

// liveClient resolves the test organisation as cashctrl-ts-sdk's write harness
// does: only the CASHCTRL_TEST_* names, never a production organisation or key
// from the environment, and the name confirmed in CASHCTRL_LIVE_TEST.
func liveClient(t *testing.T) *Client {
	t.Helper()
	org := strings.TrimSpace(os.Getenv("CASHCTRL_TEST_DOMAINID"))
	key := strings.TrimSpace(os.Getenv("CASHCTRL_TEST_APIKEY"))
	confirm := os.Getenv("CASHCTRL_LIVE_TEST")
	if confirm == "" {
		t.Skip("set CASHCTRL_TEST_DOMAINID and CASHCTRL_TEST_APIKEY to a disposable organisation and CASHCTRL_LIVE_TEST to its name")
	}
	if org == "" || key == "" {
		t.Fatal("CASHCTRL_TEST_DOMAINID and CASHCTRL_TEST_APIKEY must name a disposable organisation")
	}
	for _, name := range []string{"CASHCTRL_DOMAINID", "CASHCTRL_ORG"} {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" && v == org {
			t.Fatalf("CASHCTRL_TEST_DOMAINID is the organisation in %s: refusing to test against production", name)
		}
	}
	for _, name := range []string{"CASHCTRL_APIKEY", "CASHCTRL_API_KEY"} {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" && v == key {
			t.Fatalf("CASHCTRL_TEST_APIKEY is the key in %s: refusing to test against production", name)
		}
	}
	if confirm != org {
		t.Fatalf("confirm the target: CASHCTRL_LIVE_TEST=%s", org)
	}
	return &Client{Org: org, APIKey: key, Lang: LangEN, MaxRetries: 5}
}
