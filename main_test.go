package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func doRequest(t *testing.T, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	newMux().ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestHealthz(t *testing.T) {
	rec := doRequest(t, http.MethodGet, "/healthz")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestReadyz(t *testing.T) {
	rec := doRequest(t, http.MethodGet, "/readyz")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	rec := doRequest(t, http.MethodPost, "/healthz")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestGetenv(t *testing.T) {
	const key = "TEST_GETENV_KEY"

	t.Run("not set -> fallback", func(t *testing.T) {
		if got := getenv(key, "def"); got != "def" {
			t.Errorf("got %q, want %q", got, "def")
		}
	})

	t.Run("set -> value", func(t *testing.T) {
		t.Setenv(key, "custom")
		if got := getenv(key, "def"); got != "custom" {
			t.Errorf("got %q, want %q", got, "custom")
		}
	})

	t.Run("set but empty -> empty, not fallback", func(t *testing.T) {
		t.Setenv(key, "")
		if got := getenv(key, "def"); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}
