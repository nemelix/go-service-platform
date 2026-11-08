package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type visitStore interface {
	Ping(ctx context.Context) error
	Visits(ctx context.Context) (int64, error)
	AddVisit(ctx context.Context) (int64, error)
}

type server struct {
	store visitStore
}

func newServer(store visitStore) *server {
	return &server{store: store}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.HandleFunc("GET /api/visits", s.handleGetVisits)
	mux.HandleFunc("POST /api/visits", s.handleAddVisit)
	mux.Handle("GET /metrics", promhttp.Handler())
	return instrument(logRequests(mux))
}

func (s *server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte("OK\n"))
}

func (s *server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.store.Ping(ctx); err != nil {
		slog.WarnContext(ctx, "not ready", "err", err)
		http.Error(w, "db unavailable", http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ready\n"))
}

type visitsResponse struct {
	Visits int64 `json:"visits"`
}

func (s *server) handleGetVisits(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.Visits(r.Context())
	if err != nil {
		s.internalError(w, r, "get visits", err)
		return
	}
	writeJSON(w, http.StatusOK, visitsResponse{Visits: n})
}

func (s *server) handleAddVisit(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.AddVisit(r.Context())
	if err != nil {
		s.internalError(w, r, "add visit", err)
		return
	}
	visitsTotal.Inc()
	writeJSON(w, http.StatusOK, visitsResponse{Visits: n})
}

func (s *server) internalError(w http.ResponseWriter, r *http.Request, msg string, err error) {
	slog.ErrorContext(r.Context(), msg, "err", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write response", "err", err)
	}
}
