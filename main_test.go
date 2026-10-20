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
