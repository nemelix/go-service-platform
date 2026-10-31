package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	healthHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("healthHandler returned wrong status code: got %d want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "OK") {
		t.Errorf("body = %q, want it to contain OK", rec.Body.String())
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
