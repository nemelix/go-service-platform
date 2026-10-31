package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	_, _ = fmt.Fprintln(w, "OK")
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	addr := getenv("APP_ADDR", ":8080")

	http.HandleFunc("/health", healthHandler)

	slog.Info("listening", "addr", addr)

	if err := http.ListenAndServe(addr, nil); err != nil {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}
