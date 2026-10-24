package main

import (
	"fmt"
	"log"
	"net/http"
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	_, _ = fmt.Fprintln(w, "OK")
}

func main() {
	http.HandleFunc("/health", healthHandler)
	fmt.Println("Listening on :8080")
	log.Fatalln("error:", http.ListenAndServe(":8080", nil))
}
