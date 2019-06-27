package openapimux

import (
	"context"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
)

//OpenAPIMux is a "schema first" HTTP router. It takes one or multiple OpenAPI schema
//files as an input and then matches, validates and handles all incoming HTTP based
//on these schemas. Under the hood, it uses https://github.com/getkin/kin-openapi/
//for OpenAPI schema parsing and validation.
//
//Notes on routing:
//* Routers is a []*Router slice
//* Each router could be created from a openapi schema file or directly swagger object
//* "operationId" attribute of a path is used to resolve it to a handler
//* Multiple routers could be used to handle multiple schema versions
//* To handle multiple versions, use "servers.url" attribute in openapi schema. Eg
// servers:
//  - url: "/v1.2"
//* When finding a matching route, routers with "servers" attribute set take priority
type OpenAPIMux struct {
	handler       http.Handler
	handlers      map[string]http.Handler
	middlewares   []func(http.Handler) http.Handler
	Routers       *openapi3filter.Routers
	ErrorHandler  func(http.ResponseWriter, *http.Request, string, int)
	DetailedError bool
}

// contextKey is a value for use with context.WithValue. It's used as
// a pointer so it fits in an interface{} without allocation.
type contextKey struct {
	name string
}

var pathParamsKey = &contextKey{"pathParams"}

//NewRouter creates a OpenAPIMux from API definitions
func NewRouter(apis ...string) (*OpenAPIMux, error) {
	routers := make(openapi3filter.Routers, len(apis))

	for i, api := range apis {
		swagger, e := openapi3.NewSwaggerLoader().LoadSwaggerFromFile(api)
		if e != nil {
			return nil, e
		}

		routers[i] = openapi3filter.NewRouter().WithSwagger(swagger)
	}

	return &OpenAPIMux{
		Routers:       &routers,
		ErrorHandler:  Respond,
		DetailedError: true,
	}, nil
}

// ServeHTTP is the single method of the http.Handler interface that makes
// swagger router interoperable with the standard library.
// It will run request through all regestered middlewares and finally
// pass to handler.
func (sr *OpenAPIMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if sr.handler == nil {
		sr.handler = chain(sr.middlewares, http.HandlerFunc(sr.handleRequest))
	}

	sr.handler.ServeHTTP(w, r)
}

// UseHandlers appends a handlers to the handlers stack.
func (sr *OpenAPIMux) UseHandlers(handlers map[string]http.Handler) {
	if sr.handlers == nil {
		sr.handlers = make(map[string]http.Handler, len(handlers))
	}

	for key, h := range handlers {
		sr.handlers[key] = h
	}
}

// UseMiddleware appends a middleware handler to the middleware stack.
func (sr *OpenAPIMux) UseMiddleware(middlewares ...func(http.Handler) http.Handler) {
	sr.middlewares = append(sr.middlewares, middlewares...)
}

//PathParam returns the in-context path params for a request by name.
func PathParam(r *http.Request, key string) string {
	pathParams, ok := r.Context().Value(pathParamsKey).(map[string]string)
	if !ok {
		return ""
	}

	val, ok := pathParams[key]
	if !ok {
		return ""
	}

	return val
}

func (sr *OpenAPIMux) handleRequest(w http.ResponseWriter, r *http.Request) {
	_, route, pathParams, e := sr.Routers.FindRoute(r.Method, r.URL)
	if route == nil || route.Operation == nil || route.Operation.OperationID == "" || e != nil {
		sr.ErrorHandler(w, r, "Path not found", http.StatusNotFound)
		return
	}

	handler, ok := sr.handlers[route.Operation.OperationID]
	if ok == false {
		sr.ErrorHandler(w, r, "Handler not found", http.StatusMethodNotAllowed)
		return
	}

	input := &openapi3filter.RequestValidationInput{
		Request:    r,
		PathParams: pathParams,
		Route:      route,
	}

	e = openapi3filter.ValidateRequest(r.Context(), input)

	if e != nil {
		openapi3.SchemaErrorDetailsDisabled = !sr.DetailedError
		userError := e.Error()

		if !sr.DetailedError {
			userError = "Invalid Request"
		}

		sr.ErrorHandler(w, r, userError, http.StatusBadRequest)
		return
	}

	handler.ServeHTTP(w, WithPathParams(r, pathParams))
}

//Respond sends HTTP response
func Respond(w http.ResponseWriter, r *http.Request, data string, code int) {
	w.WriteHeader(code)
	w.Write([]byte(data))
}

//chain builds a http.Handler composed of a middleware stack and request handler
//in the order they are passed.
func chain(middlewares []func(http.Handler) http.Handler, endpoint http.Handler) http.Handler {
	if len(middlewares) == 0 {
		return endpoint
	}

	h := middlewares[len(middlewares)-1](endpoint)
	for i := len(middlewares) - 2; i >= 0; i-- {
		h = middlewares[i](h)
	}

	return h
}

//WithPathParams sets the in-context path params for a request.
func WithPathParams(r *http.Request, pathParams map[string]string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), pathParamsKey, pathParams))
}
