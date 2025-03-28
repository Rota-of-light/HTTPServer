package main

import (
	"net/http"
	"log"
)

func main() {
	server := http.NewServeMux()
	s := &http.Server{
		Addr:	":8080",
		Handler: server,
	}
	dir := http.Dir(".")
	fServer := http.FileServer(dir)
	server.Handle("/", fServer)
	err := s.ListenAndServe()
	if err != nil {
		log.Printf("Error when running server. Error: %v", err)
		return
	}
	return
}