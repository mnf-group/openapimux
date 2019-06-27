# OpenAPIMux
OpenAPIMux is a "schema-first" HTTP router. It takes one or multiple
[OpenAPI  (Swagger)](https://swagger.io/specification/) schema files as an input
and then matches, validates and handles all incoming HTTP based on these schemas.
Under the hood, it uses [kin-openapi](https://github.com/getkin/kin-openapi/) for
OpenAPI schema parsing and validation.

# Motivation
OpenAPI offers a great way of documenting API. However, none of
[existing go routers](https://github.com/avelino/awesome-go#routers) offers
"schema first" approach. In each router, you need to initialize a list of available
routes and then do the request validation manually. OpenAPIMux fills this gap by
allowing to initialize a router directly from the OpenAPI schema definition file.

# Features
* Works with both OpenAPI 3.0 and OpenAPI 2.0 (aka Swagger). As well as both json and yaml schemas
* Multiple OpenAPI schema files can be used at the same router to support API versioning
* Implements `http.Handler` interface, so it is compatible with the standard http.ServeMux
* Supports global level `http.Handler` middlewares, so it is compatible with third-party middlewares
* Supports custom error handler for more control

# Routing
* `operationId` attribute of an OpenAPI path is used to resolve it to an appropriate handler
* OpenAPIMux can encapsulate one or more swagger routers. Each router could
be created from an OpenAPI schema file or directly as a swagger object
* To handle multiple versions, use the `servers.url` attribute in OpenAPI schema. Eg
 ```yaml
 servers:
  - url: "/v1.2"
```
* When finding a matching route, routers with `servers` attribute set take priority

# Install
`go get -u github.com/MNFGroup/openapimux`

# Full Example
Assuming `openapi.yaml` has the following schema
```yaml
openapi: 3.0.0

paths:
  /foo:
    get:
      operationId: getFoo
```

It will create and start a server on 8080
```go
package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/middleware"
)

type fooHandler struct{}

func (f fooHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "Hello")
}

func main() {
	r, err := NewRouter("./openapi.yaml")
	if err != nil {
		panic(err)
	}

	r.UseHandlers(map[string]http.Handler{
		"getFoo": fooHandler{},
	})

	r.UseMiddleware(
		middleware.Recoverer,
		middleware.RequestID,
		middleware.DefaultCompress,
	)

	r.ErrorHandler = func(w http.ResponseWriter, r *http.Request, data string, code int) {
		w.WriteHeader(code)
		if code == http.StatusInternalServerError {
			fmt.Println("Fatal:", data)
			w.Write([]byte("Oops"))
		} else {
			w.Write([]byte(data))
		}
	}

	log.Fatal(http.ListenAndServe(":8080", r))
}
```
