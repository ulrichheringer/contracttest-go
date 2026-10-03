package contracttest

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
)

// Contract describes one method/path/status combination. Header values are exact
// and names are case-insensitive. Extra headers are allowed. Nil body schemas
// disable body validation. A path may contain whole-segment {name} parameters.
type Contract struct {
	Method          string
	Path            string
	Status          int
	RequestHeaders  http.Header
	ResponseHeaders http.Header
	RequestBody     *Schema
	ResponseBody    *Schema
}

// Validate validates the contract definition without executing a handler.
func (c Contract) Validate() error {
	switch c.Method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "TRACE", "CONNECT":
	default:
		return fmt.Errorf("unsupported HTTP method %q", c.Method)
	}
	if c.Status < 100 || c.Status > 599 {
		return fmt.Errorf("invalid status %d", c.Status)
	}
	if _, err := pathParts(c.Path); err != nil {
		return err
	}
	for _, headers := range []http.Header{c.RequestHeaders, c.ResponseHeaders} {
		seen := map[string]bool{}
		for name, values := range headers {
			key := strings.ToLower(name)
			if name == "" || seen[key] {
				return fmt.Errorf("empty or duplicate header %q", name)
			}
			seen[key] = true
			for _, ch := range name {
				if !strings.ContainsRune("!#$%&'*+-.^_`|~0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", ch) {
					return fmt.Errorf("invalid header name %q", name)
				}
			}
			if len(values) == 0 {
				return fmt.Errorf("header %q has no values", name)
			}
			for _, v := range values {
				if strings.ContainsAny(v, "\r\n") {
					return fmt.Errorf("invalid header value for %q", name)
				}
			}
		}
	}
	for _, s := range []*Schema{c.RequestBody, c.ResponseBody} {
		if s != nil {
			if err := s.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

func pathParts(path string) ([]string, error) {
	if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "?#") {
		return nil, fmt.Errorf("path must be absolute without query or fragment")
	}
	parts := strings.Split(path, "/")
	seen := map[string]bool{}
	for _, part := range parts {
		if strings.ContainsAny(part, "{}") {
			if len(part) < 3 || part[0] != '{' || part[len(part)-1] != '}' || strings.ContainsAny(part[1:len(part)-1], "{}") {
				return nil, fmt.Errorf("invalid path parameter %q", part)
			}
			name := part[1 : len(part)-1]
			if seen[name] {
				return nil, fmt.Errorf("duplicate path parameter %q", name)
			}
			seen[name] = true
		}
	}
	return parts, nil
}

func headersMatch(want, got http.Header) error {
	for _, name := range sortedKeys(want) {
		expected := want[name]
		var actual []string
		for _, key := range sortedKeys(got) {
			if strings.EqualFold(key, name) {
				actual = append(actual, got[key]...)
			}
		}
		if len(actual) != len(expected) {
			return fmt.Errorf("header %q: expected %q, got %q", name, expected, actual)
		}
		for i, v := range expected {
			if actual[i] != v {
				return fmt.Errorf("header %q: expected %q, got %q", name, expected, actual)
			}
		}
	}
	return nil
}

// Check runs a handler in process, validating the request first and its response.
// It clones the request and consumes only the clone's body when GetBody exists;
// otherwise request-body validation consumes and closes the original body.
// The handler must return before Check can return; use context-aware handlers.
func (c Contract) Check(handler http.Handler, request *http.Request) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if handler == nil || request == nil || request.URL == nil {
		return fmt.Errorf("handler and request URL are required")
	}
	if request.Method != c.Method {
		return fmt.Errorf("request method: expected %s, got %s", c.Method, request.Method)
	}
	want, _ := pathParts(c.Path)
	got := strings.Split(request.URL.Path, "/")
	if len(want) != len(got) {
		return fmt.Errorf("request path does not match %s", c.Path)
	}
	for i, p := range want {
		if strings.HasPrefix(p, "{") {
			if got[i] == "" {
				return fmt.Errorf("empty path parameter")
			}
		} else if p != got[i] {
			return fmt.Errorf("request path does not match %s", c.Path)
		}
	}
	if err := headersMatch(c.RequestHeaders, request.Header); err != nil {
		return fmt.Errorf("request: %w", err)
	}
	r := request.Clone(request.Context())
	if c.RequestBody != nil {
		var body io.ReadCloser
		if request.GetBody != nil {
			var err error
			body, err = request.GetBody()
			if err != nil {
				return err
			}
		} else {
			body = request.Body
		}
		if body == nil {
			return fmt.Errorf("request body is required")
		}
		data, err := io.ReadAll(body)
		closeErr := body.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if err = c.RequestBody.ValidateJSON(data); err != nil {
			return fmt.Errorf("request: %w", err)
		}
		r.Body = io.NopCloser(strings.NewReader(string(data)))
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, r)
	response := recorder.Result()
	defer response.Body.Close()
	return c.CheckResponse(response)
}

// CheckResponse validates an HTTP response and consumes its body when a response
// schema is present. The caller owns and must close the response body.
func (c Contract) CheckResponse(response *http.Response) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if response == nil {
		return fmt.Errorf("response is required")
	}
	if response.StatusCode != c.Status {
		return fmt.Errorf("status: expected %d, got %d", c.Status, response.StatusCode)
	}
	if err := headersMatch(c.ResponseHeaders, response.Header); err != nil {
		return fmt.Errorf("response: %w", err)
	}
	if c.ResponseBody != nil {
		if response.Body == nil {
			return fmt.Errorf("response body is required")
		}
		data, err := io.ReadAll(response.Body)
		if err != nil {
			return err
		}
		if err = c.ResponseBody.ValidateJSON(data); err != nil {
			return fmt.Errorf("response: %w", err)
		}
	}
	return nil
}
