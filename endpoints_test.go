package cashctrl

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/zweiundeins/cashctrl-go-sdk/internal/gen"
)

// genEndpoint is one generated method, as endpoints_gen_test.go lists them.
type genEndpoint struct {
	Method, Path string
	// Call is how the answer is read: download, post, list, unwrap or raw.
	Call string
	// Probe is the spec/responses.json key when the probe recorded a shape.
	Probe string
	Do    func(ctx context.Context, c *Client) (any, error)
}

// sample builds a JSON value of the probed shape, every field present.
func sample(s gen.Shape) any {
	switch s.Kind {
	case "scalar":
		for _, t := range s.Types {
			switch t {
			case "string":
				return "x"
			case "number":
				return 7
			case "boolean":
				return true
			}
		}
		return nil
	case "object":
		m := map[string]any{}
		for _, f := range s.Fields {
			m[f.Name] = sample(f.Shape)
		}
		return m
	case "array":
		if s.Items == nil || s.Items.Kind == "self" {
			return []any{}
		}
		return []any{sample(*s.Items)}
	}
	return nil
}

func loadResponses(t *testing.T) gen.ResponseSpec {
	t.Helper()
	b, err := os.ReadFile("spec/responses.json")
	if err != nil {
		t.Fatal(err)
	}
	var r gen.ResponseSpec
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

// TestEveryEndpoint calls every generated method against a server that checks
// the method, path and auth, and answers the probed shape. Decoding is strict,
// so a generated type that lacks a field the probe saw fails here.
func TestEveryEndpoint(t *testing.T) {
	probed := loadResponses(t)
	strictDecode = true
	t.Cleanup(func() { strictDecode = false })

	var mu sync.Mutex
	var current genEndpoint
	var got struct{ method, path string }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ep := current
		got.method, got.path = r.Method, strings.TrimPrefix(r.URL.Path, "/api/v1/")
		mu.Unlock()
		if u, p, ok := r.BasicAuth(); !ok || u != "key" || p != "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		var body any
		switch ep.Call {
		case "download":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = io.WriteString(w, "%PDF-1.7\n")
			return
		case "post":
			body = map[string]any{"success": true, "message": "saved", "insertId": 1}
		default:
			resp, ok := probed.Responses[ep.Probe]
			switch {
			case !ok && ep.Call == "list":
				body = map[string]any{"data": []any{}, "total": 0}
			case !ok && ep.Call == "unwrap":
				body = map[string]any{"data": map[string]any{}}
			case !ok:
				body = map[string]any{}
			case resp.Envelope == "data" && ep.Call == "list":
				body = map[string]any{"data": sample(resp.Shape), "total": 1}
			case resp.Envelope == "data":
				body = map[string]any{"data": sample(resp.Shape)}
			default:
				body = sample(resp.Shape)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer srv.Close()
	c := &Client{APIKey: "key", BaseURL: srv.URL + "/api/v1/", MaxRetries: -1}

	if len(generatedEndpoints) != 376 {
		t.Errorf("%d generated endpoints, the spec has 376", len(generatedEndpoints))
	}
	typed := 0
	for _, ep := range generatedEndpoints {
		mu.Lock()
		current = ep
		mu.Unlock()
		res, err := ep.Do(context.Background(), c)
		if err != nil {
			t.Errorf("%s %s: %v", ep.Method, ep.Path, err)
			continue
		}
		mu.Lock()
		if got.method != ep.Method || got.path != ep.Path {
			t.Errorf("%s %s sent %s %s", ep.Method, ep.Path, got.method, got.path)
		}
		mu.Unlock()
		if ep.Probe != "" && ep.Call != "post" && ep.Call != "download" {
			if _, raw := res.(json.RawMessage); !raw {
				typed++
			}
		}
		if d, ok := res.(*Download); ok && !strings.HasPrefix(string(d.Data), "%PDF") {
			t.Errorf("%s: download = %q", ep.Path, d.Data)
		}
	}
	if typed < 90 {
		t.Errorf("only %d probed endpoints decode into a typed answer", typed)
	}
}
