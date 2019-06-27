package openapimux

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type httpResult struct {
	body string
	code int
}

type httpTest struct {
	name     string
	method   string
	url      string
	body     io.Reader
	expected httpResult
}

func TestNewRouter(t *testing.T) {
	tests := []struct {
		name   string
		schema []string
		assert func(*OpenAPIMux, error)
	}{
		{
			name:   "empty schema",
			schema: []string{"file"},
			assert: func(r *OpenAPIMux, e error) {
				if r != nil {
					t.Errorf("router must be nil")
				}

				if e == nil {
					t.Error("error must be nil")
				}
			},
		},
		{
			name:   "wrong schema",
			schema: []string{"./testdata/invalid.json"},
			assert: func(r *OpenAPIMux, e error) {
				if r != nil {
					t.Errorf("router must be nil, got %+v", r)
				}

				if e == nil {
					t.Error("error must not be nil")
				}
			},
		},
		{
			name:   "valid schema",
			schema: []string{"./testdata/v1.yaml"},
			assert: func(r *OpenAPIMux, e error) {
				if r == nil {
					t.Error("router must not be nil - valid")
				}

				if e != nil {
					t.Errorf("error must be nil, got %s", e.Error())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Logf("Running %s", tt.name)
		router, e := NewRouter(tt.schema...)

		tt.assert(router, e)
		t.Logf("OK")
	}
}

type testGet1 struct{}

func (h testGet1) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("GET OK 1"))
}

type testGet2 struct{}

func (h testGet2) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("GET OK 2"))
}

type testPost struct{}

func (h testPost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := PathParam(r, "id")
	if id == "" {
		w.Write([]byte("POST NOT OK"))
		return
	}

	name := PathParam(r, "name")
	if name != "" {
		w.Write([]byte("POST NOT OK"))
		return
	}

	w.Write([]byte("POST OK"))
}

var handlers = map[string]http.Handler{
	"testGet1": testGet1{},
	"testGet2": testGet2{},
	"testPost": testPost{},
}

func TestServeOneVersion(t *testing.T) {
	router, _ := NewRouter("./testdata/v1.yaml")
	router.UseHandlers(handlers)

	tests := []httpTest{
		httpTest{
			name:   "test get",
			method: http.MethodGet,
			url:    "/v1",
			expected: httpResult{
				code: http.StatusOK,
				body: "GET OK 1",
			},
		},
		httpTest{
			name:   "post root",
			method: http.MethodPost,
			url:    "/v1",
			expected: httpResult{
				code: http.StatusNotFound,
			},
		},
		httpTest{
			name:   "test no path",
			method: http.MethodGet,
			url:    "/noPath",
			expected: httpResult{
				code: http.StatusNotFound,
				body: "Path not found",
			},
		},
		httpTest{
			name:   "test no handler",
			method: http.MethodGet,
			url:    "/v1/noHandler",
			expected: httpResult{
				code: http.StatusMethodNotAllowed,
				body: "Handler not found",
			},
		},
	}

	runHTTPTest(t, router, tests)
}

func TestServeTwoVersions(t *testing.T) {
	router, _ := NewRouter("./testdata/v1.yaml", "./testdata/v2.yaml")
	router.UseHandlers(handlers)

	tests := []httpTest{
		httpTest{
			name:   "test get 1",
			method: http.MethodGet,
			url:    "/v1",
			expected: httpResult{
				code: http.StatusOK,
				body: "GET OK 1",
			},
		},
		httpTest{
			name:   "test get 2",
			method: http.MethodGet,
			url:    "/v2",
			expected: httpResult{
				code: http.StatusOK,
				body: "GET OK 2",
			},
		},
		httpTest{
			name:   "post invalid",
			method: http.MethodPost,
			url:    "/v2/testPost/1",
			body:   bytes.NewReader([]byte("")),
			expected: httpResult{
				code: http.StatusBadRequest,
			},
		},
		httpTest{
			name:   "missing path param",
			method: http.MethodPost,
			url:    "/v2/testPost/",
			body:   bytes.NewReader([]byte(`{"name": "test"}`)),
			expected: httpResult{
				code: http.StatusBadRequest,
			},
		},
		httpTest{
			name:   "post valid",
			method: http.MethodPost,
			url:    "/v2/testPost/1",
			body:   bytes.NewReader([]byte(`{"name": "test"}`)),
			expected: httpResult{
				code: http.StatusOK,
				body: "POST OK",
			},
		},
	}

	runHTTPTest(t, router, tests)
}

func TestUseMiddleware(t *testing.T) {
	getMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("get middleware"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	postMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("post middleware"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	router, _ := NewRouter("./testdata/v1.yaml")
	router.UseHandlers(handlers)
	router.UseMiddleware(getMiddleware, postMiddleware)

	tests := []httpTest{
		httpTest{
			name:   "test get",
			method: http.MethodGet,
			url:    "/v1",
			expected: httpResult{
				code: http.StatusOK,
				body: "get middleware",
			},
		},
		httpTest{
			name:   "test post",
			method: http.MethodPost,
			url:    "/v1",
			expected: httpResult{
				code: http.StatusOK,
				body: "post middleware",
			},
		},
	}

	runHTTPTest(t, router, tests)
}

func runHTTPTest(t *testing.T, router *OpenAPIMux, tests []httpTest) {
	for _, tt := range tests {
		req, err := http.NewRequest(tt.method, tt.url, tt.body)

		req.Header["Content-Type"] = []string{
			"application/json",
		}

		if err != nil {
			t.Error(err)
		}

		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		if res.Code != tt.expected.code {
			t.Errorf("wrong status code: got %v want %v", res.Code, tt.expected.code)
		}

		if tt.expected.body != "" && res.Body.String() != tt.expected.body {
			t.Errorf("wrong resp body: got %s want %s", res.Body.String(), tt.expected.body)
		}
	}
}
