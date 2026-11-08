package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeStore struct {
	count   int64
	pingErr error
	err     error
}

func (f *fakeStore) Ping(context.Context) error { return f.pingErr }

func (f *fakeStore) Visits(context.Context) (int64, error) { return f.count, f.err }

func (f *fakeStore) AddVisit(context.Context) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.count++
	return f.count, nil
}

func do(t *testing.T, store visitStore, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	newServer(store).routes().ServeHTTP(rec, req)
	return rec
}

func TestHealthz(t *testing.T) {
	rec := do(t, &fakeStore{}, http.MethodGet, "/healthz")

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "OK") {
		t.Errorf("body = %q, want it to contain OK", rec.Body.String())
	}
}

func TestReadyz(t *testing.T) {
	t.Run("db up -> 200", func(t *testing.T) {
		rec := do(t, &fakeStore{}, http.MethodGet, "/readyz")
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("db down -> 503", func(t *testing.T) {
		store := &fakeStore{pingErr: errors.New("connection refused")}
		rec := do(t, store, http.MethodGet, "/readyz")
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
		}
	})
}

func TestGetVisits(t *testing.T) {
	rec := do(t, &fakeStore{count: 7}, http.MethodGet, "/api/visits")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp visitsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON %q: %v", rec.Body.String(), err)
	}
	if resp.Visits != 7 {
		t.Errorf("visits = %d, want 7", resp.Visits)
	}
}

func TestAddVisit(t *testing.T) {
	store := &fakeStore{}

	for want := int64(1); want <= 2; want++ {
		rec := do(t, store, http.MethodPost, "/api/visits")

		var resp visitsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid JSON %q: %v", rec.Body.String(), err)
		}
		if resp.Visits != want {
			t.Errorf("visits = %d, want %d", resp.Visits, want)
		}
	}
}

func TestAddVisitStoreError(t *testing.T) {
	store := &fakeStore{err: errors.New("secret table details")}
	rec := do(t, store, http.MethodPost, "/api/visits")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Errorf("response leaks internal error: %q", rec.Body.String())
	}
}

func TestMethodNotAllowed(t *testing.T) {
	rec := do(t, &fakeStore{}, http.MethodDelete, "/api/visits")

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestMetrics(t *testing.T) {
	store := &fakeStore{}
	do(t, store, http.MethodGet, "/healthz")

	rec := do(t, store, http.MethodGet, "/metrics")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "http_requests_total") {
		t.Errorf("metrics output has no http_requests_total")
	}
}
