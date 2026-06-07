package httpclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPostJSONSetsApiKeyAndDecodes(t *testing.T) {
	var key, method, ct string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, method, ct = r.Header.Get("X-Api-Key"), r.Method, r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":7}`))
	}))
	defer srv.Close()
	var out struct {
		ID int `json:"id"`
	}
	if err := New(srv.URL, "secret", nil).PostJSON(context.Background(), "/x", map[string]any{"a": 1}, &out); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	if key != "secret" || method != http.MethodPost || ct != "application/json" || out.ID != 7 {
		t.Fatalf("got key=%q method=%q ct=%q id=%d", key, method, ct, out.ID)
	}
}

func TestStatusErrorParsesMessageWithRawFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json":
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"message":"dup"}`))
		default:
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`boom`))
		}
	}))
	defer srv.Close()
	c := New(srv.URL, "k", nil)

	err := c.GetJSON(context.Background(), "/json", nil)
	var se *StatusError
	if !errors.As(err, &se) || se.StatusCode != 409 || se.Message != "dup" {
		t.Fatalf("want StatusError{409,dup}, got %v", err)
	}
	err = c.GetJSON(context.Background(), "/raw", nil)
	if !errors.As(err, &se) || se.StatusCode != 500 || se.Message != "boom" || se.Body != "boom" {
		t.Fatalf("want StatusError{500,boom,boom}, got %v", err)
	}
}

func TestEmpty2xxBodyIsSuccessButTruncatedIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/empty":
			w.WriteHeader(http.StatusCreated) // no body
		case "/truncated":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":1`)) // cut off mid-JSON
		}
	}))
	defer srv.Close()
	c := New(srv.URL, "k", nil)

	var out struct {
		ID int `json:"id"`
	}
	if err := c.PostJSON(context.Background(), "/empty", nil, &out); err != nil {
		t.Fatalf("empty 2xx body should be success, got %v", err)
	}
	if out.ID != 0 {
		t.Fatalf("empty body should leave zero value, got %d", out.ID)
	}
	out.ID = 5
	if err := c.PostJSON(context.Background(), "/truncated", nil, &out); err == nil {
		t.Fatal("truncated body should be a decode error, got nil")
	}
}

func TestRequiresBaseURLAndKeyAndTrimsBaseURL(t *testing.T) {
	if err := New("", "k", nil).GetJSON(context.Background(), "/x", nil); err == nil {
		t.Fatal("want error for empty base url")
	}
	if err := New("http://x", "", nil).GetJSON(context.Background(), "/x", nil); err == nil {
		t.Fatal("want error for empty api key")
	}
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	// trailing slash on base url must not double up with the leading slash on path
	var out map[string]any
	if err := New(srv.URL+"/", "k", nil).GetJSON(context.Background(), "/api/v1/x", &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if gotPath != "/api/v1/x" || strings.Contains(gotPath, "//") {
		t.Fatalf("bad joined path %q", gotPath)
	}
}
