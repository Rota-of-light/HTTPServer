package main

import (
	"net/http"
	"log"
	"sync/atomic"
	"fmt"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func (w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (cfg *apiConfig) metricCountHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	count := cfg.fileserverHits.Load()
	res := fmt.Sprintf("Hits: %d", count)
	w.Write([]byte(res))
}

func (cfg *apiConfig) metricResetHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	cfg.fileserverHits.Store(0)
	w.Write([]byte("Reset successful!"))
}

func main() {
	config := &apiConfig{}
	server := http.NewServeMux()
	server.HandleFunc("GET /healthz", healthCheckHandler)
	dir := http.Dir(".")
	fServer := http.FileServer(dir)
	server.Handle("/app/", config.middlewareMetricsInc(http.StripPrefix("/app", fServer)))
	server.HandleFunc("GET /metrics", config.metricCountHandler)
	server.HandleFunc("POST /reset", config.metricResetHandler)
	s := &http.Server{
		Addr:	":8080",
		Handler: server,
	}
	log.Println("Server starting on :8080")
	err := s.ListenAndServe()
	if err != nil {
		log.Printf("Error when running server. Error: %v", err)
		return
	}
	return
}