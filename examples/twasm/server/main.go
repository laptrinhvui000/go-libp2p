package main

import (
	"fmt"
	"net/http"
)

func main() {
	err := http.ListenAndServe(":9000", http.FileServer(http.Dir("./out")))
	if err != nil {
		fmt.Println("Failed to start server", err)
		return
	}
}