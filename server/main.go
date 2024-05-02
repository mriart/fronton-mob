package main

import (
	"fmt"
	"net/http"
)

func main() {
	fmt.Println("Starting in port :8080")
	err := http.ListenAndServe(":8080", http.FileServer(http.Dir("res")))
	if err != nil {
		fmt.Println("Failed to start server", err)
		return
	}
}
