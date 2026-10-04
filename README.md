# contracttest-go

Small, dependency-free HTTP contract tests for Go. Declare a method, path, status,
headers and JSON shape once; check a `net/http` handler in process or check an
actual HTTP response. Export the supported subset as OpenAPI 3.1 JSON.

Requires Go 1.24 or newer. MIT licensed.

## Install

```sh
go get github.com/ulrichheringer/contracttest-go
```

## Test a handler

```go
package users_test

import (
    "net/http"
    "net/http/httptest"
    "testing"
    contract "github.com/ulrichheringer/contracttest-go"
)

func TestUsers(t *testing.T) {
    c := contract.Contract{
        Method: "GET", Path: "/users/{id}", Status: http.StatusOK,
        ResponseBody: &contract.Schema{
            Type: contract.Object, Required: []string{"id"}, DisallowUnknown: true,
            Properties: map[string]*contract.Schema{"id": {Type: contract.Integer}},
        },
    }
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        _, _ = w.Write([]byte(`{"id":42}`))
    })
    if err := c.Check(handler, httptest.NewRequest("GET", "/users/42", nil)); err != nil {
        t.Fatal(err)
    }
}
```

For a real server, use your context-bound `http.Client`, close the returned
response body, and call `c.CheckResponse(response)`. `CheckResponse` checks status,
headers and body; method and path are checked by `Check` against its request.
Request headers and `RequestBody` can be declared with the same semantics.
`Schema.ValidateJSON(data)` is useful for standalone fixtures.

## JSON rules

| Declaration | Behavior |
| --- | --- |
| Empty `Type` | Any JSON value, including null |
| `Object` + `Properties` | Recursively validate declared properties when present |
| `Required` | Keys must exist; null still needs `Nullable` |
| `DisallowUnknown` | Reject undeclared object keys (default allows them) |
| `Array` + `Items` | Validate every element; nil `Items` allows anything |
| `String`, `Boolean`, `Null` | Exact JSON types |
| `Integer` | Mathematical integer, including `1e3`; decimal precision retained |
| `Number` + `Minimum`/`Maximum` | Inclusive bounds; use pointers for explicit zero |
| `Nullable` | Additionally allow null for the declared type |

Bounds use the shortest decimal representation of their float64 values. Numbers
are compared as exact rationals; large IDs do not silently round. Numeric tokens
are limited to 10,000 characters and exponents to ±10,000 to bound allocation.
JSON must contain exactly one value. Go's JSON decoding semantics apply: duplicate
object keys use the last value. Errors include nested property/array locations.
Invalid definitions, cycles and schema nesting deeper than 128 are rejected.

Headers are case-insensitive names with exact, ordered value lists. Extra headers
are allowed. Paths match literal segments or nonempty whole-segment `{name}`
parameters; queries do not participate. Schemas and contracts are safe to share
between parallel tests when callers do not mutate them.

## Export OpenAPI

```go
err := contract.ExportOpenAPI(writer, "Users API", "1.0.0", c)
```

Export emits [OpenAPI 3.1](https://spec.openapis.org/oas/v3.1.0.html) JSON schemas,
required keys, nullable type unions, numeric bounds, request bodies, responses,
path parameters and constant header values. Nil body schemas leave content
unspecified. Each method/path accepts one contract; duplicates are errors.
Multi-valued headers and headers ignored by OpenAPI (`Content-Type` response;
`Accept`, `Authorization`, `Content-Type` request) are rejected on export rather
than silently dropped. `CONNECT` contracts can be checked but cannot be exported.
This library does not import arbitrary OpenAPI or implement the entire JSON
Schema specification. There are no refs, regexes, enum constraints or coercion.

## Ownership and scope

`Check` uses `httptest.ResponseRecorder` and runs the handler synchronously; it
does not emulate socket behavior or enforce deadlines on an uncooperative
handler. Supply context-aware handlers for cancellation tests. Request validation
uses `GetBody` if available. Otherwise it consumes and closes the request body,
then supplies a buffered replacement to the handler. `CheckResponse` consumes
the response body when a schema is present; its caller closes it.
Bodies are buffered in memory: use bounded fixtures and client/server limits for
network responses. Panics propagate to the test. No retries or hidden network
requests occur.

## Development

```sh
gofmt -w .
test -z "$(gofmt -l .)"
go vet ./...
go install honnef.co/go/tools/cmd/staticcheck@v0.6.1
staticcheck ./...
go test -race -cover ./...
go build ./...
go run ./examples/users
```

The race detector needs a C compiler. CI executes these checks on Ubuntu for Go
1.24 and 1.26. Tests exercise nested payloads, exact numeric constraints,
required/null/unknown behavior, handler checks and export errors. The runnable
example prints a contract's OpenAPI document. Contributions should include a
regression test for behavior changes and pass the checks above.
