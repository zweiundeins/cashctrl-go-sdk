package cashctrl

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"sort"
	"strings"
	"time"
)

// FieldError is one field CashCtrl refused. Field is empty for an error about
// the request as a whole.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// APIError is a request CashCtrl refused: an HTTP status other than 200 (401
// no valid key, 403 the key's role lacks the right, 404, 429 too many
// requests, 5xx) or a 200 with "success": false (#errors), which then carries
// the field errors.
type APIError struct {
	// Endpoint is the path below /api/v1/, such as "order/create.json".
	Endpoint string
	// Status is the HTTP status; 200 for a validation error.
	Status  int
	Message string
	Fields  []FieldError
	// RetryAfter is the wait the server asked for, if it sent Retry-After.
	RetryAfter time.Duration
	// Body is the answer of a non-200 status, as sent.
	Body []byte
}

func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "cashctrl %s: ", e.Endpoint)
	if e.Status != http.StatusOK {
		fmt.Fprintf(&b, "HTTP %d", e.Status)
	} else {
		b.WriteString("refused")
	}
	if e.Message != "" {
		b.WriteString(": " + e.Message)
	}
	for _, f := range e.Fields {
		field := f.Field
		if field == "" {
			field = "_"
		}
		fmt.Fprintf(&b, "; %s: %s", field, f.Message)
	}
	return b.String()
}

// Temporary reports whether a later try may succeed: a rate limit or a fault
// at CashCtrl, not a refusal of the request itself.
func (e *APIError) Temporary() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= 500
}

// Validation reports whether CashCtrl refused the request's content (a 200
// with "success": false).
func (e *APIError) Validation() bool { return e.Status == http.StatusOK }

// ByField groups the field errors by field name, for form binding.
func (e *APIError) ByField() map[string][]string {
	out := map[string][]string{}
	for _, f := range e.Fields {
		out[f.Field] = append(out[f.Field], f.Message)
	}
	return out
}

// DecodeError is an answer that does not fit the type the specification
// gives it. Raw holds the answer, so a caller can still read it.
type DecodeError struct {
	Endpoint string
	Raw      []byte
	Err      error
}

func (e *DecodeError) Error() string {
	return fmt.Sprintf("cashctrl %s: cannot decode the answer: %v", e.Endpoint, e.Err)
}

func (e *DecodeError) Unwrap() error { return e.Err }

// fieldErrors reads "errors" as documented, [{field, message}], or as the
// {field: [messages]} map some endpoints send instead.
func fieldErrors(raw json.RawMessage) []FieldError {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var list []FieldError
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	var byField map[string]json.RawMessage
	if json.Unmarshal(raw, &byField) != nil {
		return []FieldError{{Message: string(raw)}}
	}
	fields := make([]string, 0, len(byField))
	for f := range byField {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	for _, f := range fields {
		var msgs []string
		if json.Unmarshal(byField[f], &msgs) != nil {
			var one string
			if json.Unmarshal(byField[f], &one) != nil {
				one = string(byField[f])
			}
			msgs = []string{one}
		}
		for _, m := range msgs {
			list = append(list, FieldError{Field: f, Message: m})
		}
	}
	return list
}

func parseDisposition(v string) (string, map[string]string, error) {
	return mime.ParseMediaType(v)
}
