package main

import (
	"fmt"
	"net/http"
)

func main() {
	http.HandleFunc("/ok", hOk)
	http.Handle("/", http.FileServer(http.Dir("./res/")))

	fmt.Println("Starting in port :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Failed to start server", err)
		return
	}
}

func hOk(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Ok")
}
