package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"

	contract "github.com/ulrichheringer/contracttest-go"
)

func main() {
	c := contract.Contract{Method: "GET", Path: "/users/{id}", Status: 200,
		ResponseBody: &contract.Schema{Type: contract.Object, Required: []string{"id", "name"}, DisallowUnknown: true,
			Properties: map[string]*contract.Schema{"id": {Type: contract.Integer}, "name": {Type: contract.String}}}}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":42,"name":"Ada"}`))
	})
	if err := c.Check(h, httptest.NewRequest("GET", "/users/42", nil)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := contract.ExportOpenAPI(os.Stdout, "Users", "1.0.0", c); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
