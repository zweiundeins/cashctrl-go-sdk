// Package cashctrl is a typed client for the CashCtrl accounting API
// (https://app.cashctrl.com/static/help/en/api/index.html).
//
// The endpoint methods are generated from the specification of
// github.com/zweiundeins/cashctrl-ts-sdk (spec/api.json for the requests,
// spec/responses.json for the response shapes); spec/SOURCE names the version.
// Resources mirror the API's paths, so POST /api/v1/order/category/create.json
// is client.Order().Category().Create(ctx, &OrderCategoryCreateParams{...}).
//
// The API takes form-encoded parameters (nested values as JSON strings),
// answers JSON, authenticates with HTTP Basic (the API key as the user, no
// password) and reports a validation error as HTTP 200 with "success": false,
// which this package returns as an *APIError.
package cashctrl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Lang is a language the API writes error messages and generated documents
// (PDF, Excel, CSV) in.
type Lang string

// The languages of the lang parameter (#lang).
const (
	LangDE Lang = "de"
	LangFR Lang = "fr"
	LangIT Lang = "it"
	LangEN Lang = "en"
)

// Client talks to one CashCtrl organisation. The zero value of every field but
// Org and APIKey is usable; a Client is safe for concurrent use.
type Client struct {
	// Org is the subdomain of <org>.cashctrl.com. Ignored when BaseURL is set.
	Org string
	// APIKey is the key of an API user (Settings, Users & Roles, Add API user).
	APIKey string
	// Lang is sent as the lang parameter of every request, unless empty or
	// overridden per call with WithLang.
	Lang Lang
	// HTTPClient sends the requests; nil uses a client with a 60 s timeout.
	HTTPClient *http.Client
	// BaseURL replaces https://<org>.cashctrl.com/api/v1/, for tests and
	// proxies. Endpoint paths such as "order/read.json" are appended to it.
	BaseURL string
	// MaxRetries bounds how often a request answered 429 is sent again (GET
	// requests also on 408, 502, 503, 504 and network errors). 0 means 3, a
	// negative value disables retries.
	MaxRetries int
	// RetryBaseDelay is the first wait before a retry when the answer has no
	// Retry-After header; it doubles per attempt. 0 means 1 s.
	RetryBaseDelay time.Duration
	// MaxRetryDelay caps one wait. 0 means 30 s.
	MaxRetryDelay time.Duration
	// MaxResponseBytes caps a response body. 0 means 64 MiB.
	MaxResponseBytes int64
	// UserAgent is sent as the User-Agent header when set.
	UserAgent string
}

// New returns a client for the organisation's subdomain org.
func New(org, apiKey string) *Client {
	return &Client{Org: org, APIKey: apiKey}
}

type langKey struct{}

// WithLang returns a context whose requests send lang instead of Client.Lang,
// for one document in the customer's language.
func WithLang(ctx context.Context, lang Lang) context.Context {
	return context.WithValue(ctx, langKey{}, lang)
}

var defaultHTTPClient = &http.Client{Timeout: 60 * time.Second}

func (c *Client) base() (string, error) {
	if c.BaseURL != "" {
		if strings.HasSuffix(c.BaseURL, "/") {
			return c.BaseURL, nil
		}
		return c.BaseURL + "/", nil
	}
	org := strings.TrimSpace(c.Org)
	if org == "" {
		return "", errors.New("cashctrl: Client.Org (or BaseURL) is empty")
	}
	if strings.ContainsAny(org, "./:@ ?#") {
		return "", fmt.Errorf("cashctrl: Client.Org %q is not a subdomain: write only the part before .cashctrl.com", org)
	}
	return "https://" + org + ".cashctrl.com/api/v1/", nil
}

func (c *Client) retries() int {
	switch {
	case c.MaxRetries < 0:
		return 0
	case c.MaxRetries == 0:
		return 3
	}
	return c.MaxRetries
}

func (c *Client) delay(attempt int, header http.Header) time.Duration {
	limit := c.MaxRetryDelay
	if limit <= 0 {
		limit = 30 * time.Second
	}
	if d, ok := retryAfter(header); ok {
		return min(d, limit)
	}
	base := c.RetryBaseDelay
	if base <= 0 {
		base = time.Second
	}
	d := base << attempt
	if d <= 0 || d > limit {
		d = limit
	}
	// Up to a quarter of jitter, so clients that were refused together do not
	// come back together.
	return d - time.Duration(rand.Int64N(int64(d)/4+1))
}

// retryAfter reads a Retry-After header in seconds or as an HTTP date.
func retryAfter(h http.Header) (time.Duration, bool) {
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0, false
	}
	if s, err := strconv.Atoi(v); err == nil && s >= 0 {
		return time.Duration(s) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(time.Until(t), 0), true
	}
	return 0, false
}

// retryable reports whether a status may be sent again. A write is repeated
// only after a 429, which CashCtrl answers before doing anything; after a 5xx
// the write may have happened.
func retryable(method string, status int) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	if method != http.MethodGet {
		return false
	}
	switch status {
	case http.StatusRequestTimeout, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// response is a 200 answer.
type response struct {
	header http.Header
	body   []byte
}

// Do sends one request to endpoint (a path below /api/v1/, such as
// "order/read.json") and returns the body of a 200 answer. It is the
// transport under the generated methods, for endpoints the specification
// lacks. A non-200 answer is an *APIError; Do does not look into the body.
func (c *Client) Do(ctx context.Context, method, endpoint string, params url.Values) ([]byte, error) {
	res, err := c.do(ctx, method, endpoint, params)
	if err != nil {
		return nil, err
	}
	return res.body, nil
}

func (c *Client) do(ctx context.Context, method, endpoint string, params url.Values) (*response, error) {
	base, err := c.base()
	if err != nil {
		return nil, err
	}
	if c.APIKey == "" {
		return nil, errors.New("cashctrl: Client.APIKey is empty")
	}
	endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "/api/v1/"), "/")
	form := url.Values{}
	for k, v := range params {
		form[k] = v
	}
	lang := c.Lang
	if l, ok := ctx.Value(langKey{}).(Lang); ok {
		lang = l
	}
	if lang != "" && !form.Has("lang") {
		form.Set("lang", string(lang))
	}
	target := base + endpoint
	encoded := form.Encode()
	if method == http.MethodGet && encoded != "" {
		target += "?" + encoded
	}
	httpc := c.HTTPClient
	if httpc == nil {
		httpc = defaultHTTPClient
	}
	limit := c.MaxResponseBytes
	if limit <= 0 {
		limit = 64 << 20
	}

	for attempt := 0; ; attempt++ {
		var body io.Reader
		if method != http.MethodGet {
			body = strings.NewReader(encoded)
		}
		req, err := http.NewRequestWithContext(ctx, method, target, body)
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(c.APIKey, "") // #auth: the key is the user, no password
		req.Header.Set("Accept", "application/json")
		if method != http.MethodGet {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		if c.UserAgent != "" {
			req.Header.Set("User-Agent", c.UserAgent)
		}
		res, err := httpc.Do(req)
		if err != nil {
			if ctx.Err() == nil && method == http.MethodGet && attempt < c.retries() {
				if werr := wait(ctx, c.delay(attempt, nil)); werr != nil {
					return nil, werr
				}
				continue
			}
			return nil, fmt.Errorf("cashctrl %s: %w", endpoint, err)
		}
		raw, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
		res.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("cashctrl %s: %w", endpoint, err)
		}
		if int64(len(raw)) > limit {
			return nil, fmt.Errorf("cashctrl %s: the answer is larger than %d bytes", endpoint, limit)
		}
		if res.StatusCode == http.StatusOK {
			return &response{header: res.Header, body: raw}, nil
		}
		if retryable(method, res.StatusCode) && attempt < c.retries() {
			if werr := wait(ctx, c.delay(attempt, res.Header)); werr != nil {
				return nil, werr
			}
			continue
		}
		e := &APIError{Endpoint: endpoint, Status: res.StatusCode, Message: trimMessage(raw), Body: raw}
		if d, ok := retryAfter(res.Header); ok {
			e.RetryAfter = d
		}
		return nil, e
	}
}

func wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func trimMessage(raw []byte) string {
	msg := strings.TrimSpace(string(raw))
	if r := []rune(msg); len(r) > 200 {
		msg = string(r[:200])
	}
	return msg
}

// envelope is the part of an answer that says whether it was refused.
type envelope struct {
	Success *bool           `json:"success"`
	Message string          `json:"message"`
	Errors  json.RawMessage `json:"errors"`
}

func (env envelope) refusal(endpoint string) error {
	if env.Success == nil || *env.Success {
		return nil
	}
	return &APIError{Endpoint: endpoint, Status: http.StatusOK, Message: env.Message, Fields: fieldErrors(env.Errors)}
}

// strictDecode makes decoding refuse fields a type does not have; the tests
// use it to hold the generated types to the probed shapes.
var strictDecode bool

func decode(endpoint string, raw []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if strictDecode {
		dec.DisallowUnknownFields()
	}
	if err := dec.Decode(out); err != nil {
		return &DecodeError{Endpoint: endpoint, Raw: raw, Err: err}
	}
	return nil
}

// getJSON GETs or POSTs endpoint and decodes the answer, refusing a 200 with
// "success": false. With unwrap it decodes the "data" member.
func getJSON[T any](ctx context.Context, c *Client, method, endpoint string, params url.Values, unwrap bool) (T, error) {
	var zero T
	res, err := c.do(ctx, method, endpoint, params)
	if err != nil {
		return zero, err
	}
	trimmed := bytes.TrimSpace(res.body)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var env envelope
		if json.Unmarshal(trimmed, &env) == nil {
			if err := env.refusal(endpoint); err != nil {
				return zero, err
			}
		}
	}
	if unwrap {
		var wrapped struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(trimmed, &wrapped); err != nil {
			return zero, &DecodeError{Endpoint: endpoint, Raw: res.body, Err: err}
		}
		if len(wrapped.Data) == 0 {
			return zero, &DecodeError{Endpoint: endpoint, Raw: res.body, Err: errors.New("no data member")}
		}
		trimmed = wrapped.Data
	}
	var out T
	if len(trimmed) == 0 {
		return out, nil
	}
	if err := decode(endpoint, trimmed, &out); err != nil {
		return zero, err
	}
	return out, nil
}

// List is the answer of a list or tree endpoint.
type List[T any] struct {
	Data []T `json:"data"`
	// Total is the number of rows the filter matches, which is more than
	// len(Data) when limit cut the page short. A tree answers no total; it is
	// then len(Data).
	Total int `json:"total"`
}

func getList[T any](ctx context.Context, c *Client, endpoint string, params url.Values) (*List[T], error) {
	res, err := c.do(ctx, http.MethodGet, endpoint, params)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(res.body)
	out := &List[T]{}
	if len(trimmed) > 0 && trimmed[0] == '[' {
		// A bare array, as some trees answer.
		if err := decode(endpoint, trimmed, &out.Data); err != nil {
			return nil, err
		}
		out.Total = len(out.Data)
		return out, nil
	}
	var env struct {
		envelope
		Data  json.RawMessage `json:"data"`
		Total *int            `json:"total"`
	}
	if err := json.Unmarshal(trimmed, &env); err != nil {
		return nil, &DecodeError{Endpoint: endpoint, Raw: res.body, Err: err}
	}
	if err := env.refusal(endpoint); err != nil {
		return nil, err
	}
	if len(env.Data) > 0 && string(env.Data) != "null" {
		if err := decode(endpoint, env.Data, &out.Data); err != nil {
			return nil, err
		}
	}
	out.Total = len(out.Data)
	if env.Total != nil {
		out.Total = *env.Total
	}
	return out, nil
}

// WriteResult is the answer of a create, update, delete or other POST.
type WriteResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	// InsertID is the id of the record a create made; 0 when it made none or
	// several (then see InsertIDs).
	InsertID int64 `json:"-"`
	// InsertIDs holds every id the answer named, such as the orders of one
	// create for several business partners.
	InsertIDs []int64 `json:"-"`
	// Data is the "data" member a few writes answer instead (file/prepare).
	Data json.RawMessage `json:"data,omitempty"`
	// Raw is the whole answer.
	Raw json.RawMessage `json:"-"`
}

func post(ctx context.Context, c *Client, endpoint string, params url.Values) (*WriteResult, error) {
	res, err := c.do(ctx, http.MethodPost, endpoint, params)
	if err != nil {
		return nil, err
	}
	var env struct {
		envelope
		InsertID json.RawMessage `json:"insertId"`
		Data     json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(res.body, &env); err != nil {
		return nil, &DecodeError{Endpoint: endpoint, Raw: res.body, Err: err}
	}
	if err := env.refusal(endpoint); err != nil {
		return nil, err
	}
	out := &WriteResult{Success: env.Success == nil || *env.Success, Message: env.Message, Data: env.Data, Raw: res.body}
	out.InsertIDs = parseIDs(env.InsertID)
	if len(out.InsertIDs) == 1 {
		out.InsertID = out.InsertIDs[0]
	}
	return out, nil
}

// parseIDs reads insertId as a number, a numeric string, a comma-separated
// string or an array of those.
func parseIDs(raw json.RawMessage) []int64 {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var list []json.RawMessage
	if raw[0] == '[' && json.Unmarshal(raw, &list) == nil {
		var out []int64
		for _, r := range list {
			out = append(out, parseIDs(r)...)
		}
		return out
	}
	s := string(raw)
	if raw[0] == '"' {
		if json.Unmarshal(raw, &s) != nil {
			return nil
		}
	}
	var out []int64
	for _, part := range strings.Split(s, ",") {
		if n, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// Download is a document an endpoint answered: a PDF, a spreadsheet, a ZIP.
type Download struct {
	ContentType string
	// Filename is the name from the Content-Disposition header, if any.
	Filename string
	Data     []byte
}

func download(ctx context.Context, c *Client, method, endpoint string, params url.Values) (*Download, error) {
	res, err := c.do(ctx, method, endpoint, params)
	if err != nil {
		return nil, err
	}
	ct := res.header.Get("Content-Type")
	trimmed := bytes.TrimSpace(res.body)
	// A refusal comes as the usual JSON envelope with status 200.
	if strings.HasPrefix(ct, "application/json") || (len(trimmed) > 0 && trimmed[0] == '{') {
		var env envelope
		if json.Unmarshal(trimmed, &env) == nil {
			if err := env.refusal(endpoint); err != nil {
				return nil, err
			}
		}
	}
	f := &Download{ContentType: ct, Data: res.body}
	if cd := res.header.Get("Content-Disposition"); cd != "" {
		if _, p, err := parseDisposition(cd); err == nil {
			f.Filename = p["filename"]
		}
	}
	return f, nil
}
