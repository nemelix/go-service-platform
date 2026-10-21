package main

import (
	"fmt"
	"net/http"
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "BAD")
}

func main() {
	http.HandleFunc("/health", healthHandler)
	fmt.Println("Listening on :8080")
	http.ListenAndServe(":8080", nil)
}
