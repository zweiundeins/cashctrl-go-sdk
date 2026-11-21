package cashctrl

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// fake answers per endpoint and records every request.
type fake struct {
	mu      sync.Mutex
	reqs    []recorded
	answer  map[string]string
	header  map[string]http.Header
	status  map[string][]int // consumed one per request
	srv     *httptest.Server
	baseURL string
}

type recorded struct {
	Method, Path, ContentType string
	User, Pass                string
	Form                      url.Values
}

func newFake(t *testing.T) (*fake, *Client) {
	t.Helper()
	f := &fake{answer: map[string]string{}, header: map[string]http.Header{}, status: map[string][]int{}}
	f.srv = httptest.NewServer(f)
	t.Cleanup(f.srv.Close)
	f.baseURL = f.srv.URL + "/api/v1/"
	return f, &Client{APIKey: "key123", BaseURL: f.baseURL, RetryBaseDelay: time.Millisecond}
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	user, pass, _ := r.BasicAuth()
	body, _ := io.ReadAll(r.Body)
	form, _ := url.ParseQuery(string(body))
	if r.Method == http.MethodGet {
		form = r.URL.Query()
	}
	ep := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	f.mu.Lock()
	f.reqs = append(f.reqs, recorded{r.Method, ep, r.Header.Get("Content-Type"), user, pass, form})
	status := http.StatusOK
	if q := f.status[ep]; len(q) > 0 {
		status, f.status[ep] = q[0], q[1:]
	}
	answer, h := f.answer[ep], f.header[ep]
	f.mu.Unlock()
	if user != "key123" || pass != "" {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, "unauthorized")
		return
	}
	for k, v := range h {
		w.Header()[k] = v
	}
	w.WriteHeader(status)
	_, _ = io.WriteString(w, answer)
}

func (f *fake) last(t *testing.T, ep string) recorded {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.reqs) - 1; i >= 0; i-- {
		if f.reqs[i].Path == ep {
			return f.reqs[i]
		}
	}
	t.Fatalf("no request to %s", ep)
	return recorded{}
}

func (f *fake) count(ep string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.reqs {
		if r.Path == ep {
			n++
		}
	}
	return n
}

func TestBaseURL(t *testing.T) {
	for _, org := range []string{"", "myorg.cashctrl.com", "https://myorg", "my org"} {
		if _, err := (&Client{Org: org}).base(); err == nil {
			t.Errorf("Org %q was taken", org)
		}
	}
	if b, err := New("myorg", "k").base(); err != nil || b != "https://myorg.cashctrl.com/api/v1/" {
		t.Errorf("base = %q, %v", b, err)
	}
	if b, _ := (&Client{BaseURL: "http://proxy/api/v1"}).base(); b != "http://proxy/api/v1/" {
		t.Errorf("BaseURL without a slash = %q", b)
	}
	if _, err := (&Client{Org: "myorg"}).Tax().Read(context.Background(), &TaxReadParams{ID: 1}); err == nil {
		t.Error("a client without an API key sent a request")
	}
}

func TestTransport(t *testing.T) {
	f, c := newFake(t)
	ctx := context.Background()
	c.Lang = LangDE
	c.UserAgent = "test-agent"

	f.answer["tax/read.json"] = `{"data":{"id":2,"code":"UN81","currentPercentage":8.1}}`
	tax, err := c.Tax().Read(ctx, &TaxReadParams{ID: 2})
	if err != nil || tax.ID != 2 || tax.Code != "UN81" {
		t.Fatalf("Tax.Read = %+v, %v", tax, err)
	}
	r := f.last(t, "tax/read.json")
	if r.Method != http.MethodGet || r.User != "key123" || r.Pass != "" || r.Form.Get("id") != "2" || r.Form.Get("lang") != "de" {
		t.Errorf("GET request = %+v", r)
	}

	// A POST is form-encoded; WithLang overrides Client.Lang.
	f.answer["person/create.json"] = `{"success":true,"message":"Person saved","insertId":8}`
	res, err := c.Person().Create(WithLang(ctx, LangFR), &PersonCreateParams{Company: Ptr("Muster AG"), IsCustomer: Ptr(false)})
	if err != nil || res.InsertID != 8 || !res.Success || res.Message != "Person saved" {
		t.Fatalf("Person.Create = %+v, %v", res, err)
	}
	r = f.last(t, "person/create.json")
	if r.Method != http.MethodPost || r.ContentType != "application/x-www-form-urlencoded" || r.Form.Get("lang") != "fr" ||
		r.Form.Get("company") != "Muster AG" || r.Form.Get("isCustomer") != "false" || r.Form.Has("firstName") {
		t.Errorf("POST request = %+v", r)
	}
}

func TestInsertIDs(t *testing.T) {
	for raw, want := range map[string][]int64{`5`: {5}, `"6"`: {6}, `"7,8"`: {7, 8}, `[9,10]`: {9, 10}, `null`: nil} {
		got := parseIDs([]byte(raw))
		if len(got) != len(want) {
			t.Errorf("parseIDs(%s) = %v", raw, got)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("parseIDs(%s) = %v", raw, got)
			}
		}
	}
}

func TestErrors(t *testing.T) {
	f, c := newFake(t)
	ctx := context.Background()

	// A validation error is HTTP 200 with success false (#errors).
	f.answer["order/create.json"] = `{"success":false,"message":"Check your input","errors":[{"field":"associateId","message":"This field cannot be empty."},{"field":null,"message":"General"}]}`
	_, err := c.Order().Create(ctx, &OrderCreateParams{CategoryID: 4})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !apiErr.Validation() || apiErr.Temporary() || len(apiErr.Fields) != 2 ||
		apiErr.Fields[0].Field != "associateId" || apiErr.Endpoint != "order/create.json" || apiErr.Message != "Check your input" {
		t.Fatalf("refusal = %#v", err)
	}
	if by := apiErr.ByField(); len(by["associateId"]) != 1 || len(by[""]) != 1 {
		t.Errorf("ByField = %v", by)
	}
	if !strings.Contains(err.Error(), "associateId: This field cannot be empty.") {
		t.Errorf("Error() = %q", err.Error())
	}

	// Some endpoints send the errors as a map.
	f.answer["account/create.json"] = `{"success":false,"errors":{"number":["Taken","Too long"]}}`
	_, err = c.Account().Create(ctx, &AccountCreateParams{})
	if !errors.As(err, &apiErr) || len(apiErr.Fields) != 2 || apiErr.Fields[1].Field != "number" || apiErr.Fields[1].Message != "Too long" {
		t.Errorf("map errors = %#v", err)
	}

	// A read of an unknown id is a 200 with success false too.
	f.answer["order/read.json"] = `{"success":false,"message":"Entry not found"}`
	if _, err := c.Order().Read(ctx, &OrderReadParams{ID: 9}); !errors.As(err, &apiErr) || apiErr.Message != "Entry not found" {
		t.Errorf("unknown order = %v", err)
	}

	// A wrong key is HTTP 401, not retried.
	bad := &Client{APIKey: "wrong", BaseURL: f.baseURL}
	if _, err := bad.Tax().Read(ctx, &TaxReadParams{ID: 2}); !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized || apiErr.Temporary() {
		t.Errorf("wrong key = %v", err)
	}

	// An answer that does not fit the type is a DecodeError carrying it.
	f.answer["tax/read.json"] = `{"data":{"id":"not a number"}}`
	_, err = c.Tax().Read(ctx, &TaxReadParams{ID: 2})
	var decErr *DecodeError
	if !errors.As(err, &decErr) || !strings.Contains(string(decErr.Raw), "not a number") {
		t.Errorf("misfit answer = %v", err)
	}
}

func TestRetries(t *testing.T) {
	f, c := newFake(t)
	ctx := context.Background()
	f.answer["tax/read.json"] = `{"data":{"id":2}}`

	// A 429 is sent again, and Retry-After is honoured.
	f.status["tax/read.json"] = []int{http.StatusTooManyRequests, http.StatusTooManyRequests}
	f.header["tax/read.json"] = http.Header{"Retry-After": {"0"}}
	if _, err := c.Tax().Read(ctx, &TaxReadParams{ID: 2}); err != nil || f.count("tax/read.json") != 3 {
		t.Errorf("after two 429s: %v, %d requests", err, f.count("tax/read.json"))
	}

	// Bounded: MaxRetries 1 gives up on the second 429 with a temporary error.
	c.MaxRetries = 1
	f.status["tax/read.json"] = []int{429, 429, 429}
	f.header["tax/read.json"] = http.Header{"Retry-After": {"7"}}
	c.MaxRetryDelay = time.Millisecond
	_, err := c.Tax().Read(ctx, &TaxReadParams{ID: 2})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 429 || !apiErr.Temporary() || apiErr.RetryAfter != 7*time.Second {
		t.Errorf("retries exhausted = %#v", err)
	}

	// A write is retried after a 429, never after a 5xx: the write may have
	// happened.
	f.answer["order/create.json"] = `{"success":true,"insertId":5}`
	f.status["order/create.json"] = []int{429}
	if res, err := c.Order().Create(ctx, &OrderCreateParams{}); err != nil || res.InsertID != 5 {
		t.Errorf("write after a 429 = %v, %v", res, err)
	}
	before := f.count("order/create.json")
	f.status["order/create.json"] = []int{http.StatusBadGateway}
	if _, err := c.Order().Create(ctx, &OrderCreateParams{}); !errors.As(err, &apiErr) || apiErr.Status != 502 {
		t.Errorf("write after a 502 = %v", err)
	}
	if n := f.count("order/create.json") - before; n != 1 {
		t.Errorf("a write that got a 502 was sent %d times", n)
	}
	// A read is retried after a 503.
	before = f.count("tax/read.json")
	f.status["tax/read.json"] = []int{http.StatusServiceUnavailable}
	if _, err := c.Tax().Read(ctx, &TaxReadParams{ID: 2}); err != nil || f.count("tax/read.json")-before != 2 {
		t.Errorf("read after a 503 = %v", err)
	}

	// Disabled.
	c.MaxRetries = -1
	f.status["tax/read.json"] = []int{429}
	if _, err := c.Tax().Read(ctx, &TaxReadParams{ID: 2}); !errors.As(err, &apiErr) {
		t.Errorf("no retries = %v", err)
	}

	// A cancelled context ends the wait.
	c.MaxRetries, c.MaxRetryDelay = 3, time.Hour
	f.status["tax/read.json"] = []int{429}
	f.header["tax/read.json"] = http.Header{"Retry-After": {"3600"}}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if _, err := c.Tax().Read(cctx, &TaxReadParams{ID: 2}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("cancelled wait = %v", err)
	}
}

func TestDownload(t *testing.T) {
	f, c := newFake(t)
	ctx := context.Background()
	f.answer["order/document/read.pdf"] = "%PDF-1.7\n..."
	f.header["order/document/read.pdf"] = http.Header{"Content-Type": {"application/pdf"},
		"Content-Disposition": {`attachment; filename="RE-2026-0042.pdf"`}}
	pdf, err := c.Order().Document().ReadPDF(WithLang(ctx, LangIT), &OrderDocumentReadPDFParams{IDs: []int64{412, 413}})
	if err != nil || !strings.HasPrefix(string(pdf.Data), "%PDF") || pdf.ContentType != "application/pdf" || pdf.Filename != "RE-2026-0042.pdf" {
		t.Fatalf("ReadPDF = %+v, %v", pdf, err)
	}
	if r := f.last(t, "order/document/read.pdf"); r.Method != http.MethodGet || r.Form.Get("ids") != "412,413" || r.Form.Get("lang") != "it" {
		t.Errorf("read.pdf request = %+v", r)
	}
	// A refusal comes as the JSON envelope with status 200.
	f.header["order/document/read.pdf"] = http.Header{"Content-Type": {"application/json"}}
	f.answer["order/document/read.pdf"] = `{"success":false,"message":"No permission"}`
	var apiErr *APIError
	if _, err := c.Order().Document().ReadPDF(ctx, &OrderDocumentReadPDFParams{IDs: []int64{1}}); !errors.As(err, &apiErr) || apiErr.Message != "No permission" {
		t.Errorf("refused PDF = %v", err)
	}
}

func TestListAndAll(t *testing.T) {
	f, c := newFake(t)
	ctx := context.Background()
	f.answer["person/list.json"] = `{"data":[{"id":1,"nr":"P-1"},{"id":2,"nr":"P-2"}],"total":5}`
	page, err := c.Person().List(ctx, &PersonListParams{Filter: []Filter{Where("nr", Eq, "P-1")}, Sort: Ptr("nr"), Dir: Ptr(Desc), Limit: Ptr(2)})
	if err != nil || len(page.Data) != 2 || page.Total != 5 || page.Data[1].Nr != "P-2" {
		t.Fatalf("Person.List = %+v, %v", page, err)
	}
	r := f.last(t, "person/list.json")
	if r.Form.Get("filter") != `[{"field":"nr","comparison":"eq","value":"P-1"}]` || r.Form.Get("sort") != "nr" ||
		r.Form.Get("dir") != "DESC" || r.Form.Get("limit") != "2" || r.Form.Has("start") {
		t.Errorf("list request = %v", r.Form)
	}

	// All pages with start and limit until total is reached.
	var starts []int
	rows := All(ctx, 2, func(ctx context.Context, start, limit int) (*List[Person], error) {
		starts = append(starts, start)
		data := []Person{{ID: int64(start + 1)}, {ID: int64(start + 2)}}
		if start == 4 {
			data = data[:1]
		}
		return &List[Person]{Data: data, Total: 5}, nil
	})
	var ids []int64
	for p, err := range rows {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, p.ID)
	}
	if len(ids) != 5 || ids[4] != 5 || len(starts) != 3 || starts[2] != 4 {
		t.Errorf("All = %v over starts %v", ids, starts)
	}
	boom := errors.New("boom")
	for _, err := range All(ctx, 0, func(context.Context, int, int) (*List[Person], error) { return nil, boom }) {
		if !errors.Is(err, boom) {
			t.Errorf("All error = %v", err)
		}
	}
}

func TestDate(t *testing.T) {
	d, err := ParseDate("2026-11-20 00:00:00.0")
	if err != nil || d.String() != "2026-11-20" {
		t.Errorf("ParseDate = %v, %v", d, err)
	}
	if b, _ := NewDate(2027, 1, 5).MarshalJSON(); string(b) != `"2027-01-05"` {
		t.Errorf("MarshalJSON = %s", b)
	}
	var back Date
	if err := back.UnmarshalJSON([]byte(`"2027-01-05"`)); err != nil || back.String() != "2027-01-05" {
		t.Errorf("UnmarshalJSON = %v, %v", back, err)
	}
}
