package contracttest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerContract(t *testing.T) {
	c := Contract{Method: "POST", Path: "/users/{id}", Status: 201, RequestHeaders: http.Header{"x-client": []string{"test"}}, ResponseHeaders: http.Header{"X-Result": []string{"ok"}}, RequestBody: &Schema{Type: Object}, ResponseBody: &Schema{Type: Object, Required: []string{"id"}, Properties: map[string]*Schema{"id": {Type: Integer}}}}
	r, _ := http.NewRequest("POST", "/users/123", strings.NewReader(`{}`))
	r.Header.Set("X-Client", "test")
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil {
			t.Fatal("missing body")
		}
		w.Header().Set("X-Result", "ok")
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"id":123}`))
	})
	if err := c.Check(h, r); err != nil {
		t.Fatal(err)
	}
	if err := c.Check(h, r); err != nil {
		t.Fatalf("GetBody should preserve original: %v", err)
	}
	r.Method = "GET"
	if err := c.Check(h, r); err == nil {
		t.Fatal("method mismatch accepted")
	}
	r.Method = "POST"
	r.URL.Path = "/wrong/123"
	if err := c.Check(h, r); err == nil {
		t.Fatal("path mismatch accepted")
	}
}

func TestResponseFailures(t *testing.T) {
	c := Contract{Method: "GET", Path: "/", Status: 200, ResponseHeaders: http.Header{"X-Result": []string{"ok"}}, ResponseBody: &Schema{Type: String}}
	for _, tt := range []struct {
		status       int
		header, body string
	}{{201, "ok", `"hi"`}, {200, "bad", `"hi"`}, {200, "ok", `1`}} {
		r := httptest.NewRecorder()
		r.Header().Set("X-Result", tt.header)
		r.WriteHeader(tt.status)
		_, _ = r.WriteString(tt.body)
		if err := c.CheckResponse(r.Result()); err == nil {
			t.Fatalf("accepted %#v", tt)
		}
	}
}

func TestOpenAPI(t *testing.T) {
	c := Contract{Method: "GET", Path: "/users/{id}", Status: 200, ResponseBody: &Schema{Type: Object, DisallowUnknown: true, Required: []string{"name"}, Properties: map[string]*Schema{"name": {Type: String, Nullable: true}, "score": {Type: Number, Maximum: ptr(10)}}}}
	var out bytes.Buffer
	if err := ExportOpenAPI(&out, "Users", "1.0", c); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["openapi"] != "3.1.0" {
		t.Fatal(doc)
	}
	for _, fragment := range []string{`"additionalProperties": false`, `"required"`, `"maximum": 10`, `"in": "path"`, `"null"`} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("missing %s", fragment)
		}
	}
	if err := ExportOpenAPI(&out, "Users", "1", c, c); err == nil {
		t.Fatal("duplicate operation accepted")
	}
	other := c
	other.Path = "/users/{other}"
	if err := ExportOpenAPI(&out, "Users", "1", c, other); err == nil {
		t.Fatal("equivalent paths accepted")
	}
	c.ResponseHeaders = http.Header{"Content-Type": []string{"application/json"}}
	if err := ExportOpenAPI(&out, "Users", "1", c); err == nil {
		t.Fatal("ignored header accepted")
	}
}

func TestHeaderCaseAndExtremeNumbers(t *testing.T) {
	if err := headersMatch(http.Header{"X-Result": []string{"ok"}}, http.Header{"x-result": []string{"ok"}}); err != nil {
		t.Fatal(err)
	}
	s := Schema{Type: Number}
	if err := s.ValidateJSON([]byte(`1e1000000000`)); err == nil {
		t.Fatal("unbounded exponent accepted")
	}
}

func TestInvalidContracts(t *testing.T) {
	for _, c := range []Contract{{Method: "get", Path: "/", Status: 200}, {Method: "GET", Path: "relative", Status: 200}, {Method: "GET", Path: "/{id}/{id}", Status: 200}, {Method: "GET", Path: "/", Status: 600}, {Method: "GET", Path: "/", Status: 200, RequestHeaders: http.Header{"Bad Name": []string{"x"}}}} {
		if err := c.Validate(); err == nil {
			t.Fatalf("accepted %#v", c)
		}
	}
}
