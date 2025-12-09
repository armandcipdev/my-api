package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello from Render!")
	})

	port := os.Getenv("PORT") // Render memberi PORT via env
	http.ListenAndServe(":"+port, nil)
}
