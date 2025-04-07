package main

import (
	_ "github.com/lib/pq" //Needed for side-effect
	"github.com/joho/godotenv"

	"os"
	"database/sql"
	"net/http"
	"log"
	"sync/atomic"
	"fmt"
	"encoding/json"
	"strings"

	"github.com/Rota-of-light/HTTPServer/internal/database"
)

type apiConfig struct {
	db		*database.Queries
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
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	count := cfg.fileserverHits.Load()
	res := fmt.Sprintf(
		`<!DOCTYPE html>
	<html>
	  <body>
		<h1>Welcome, Chirpy Admin</h1>
		<p>Chirpy has been visited %d times!</p>
	  </body>
	</html>`, count)
	w.Write([]byte(res))
}

func (cfg *apiConfig) metricResetHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	cfg.fileserverHits.Store(0)
	w.Write([]byte("Reset successful!"))
}

func profaneChecker(s string) string {
	profaneWords := map[string]bool{
		"kerfuffle": true,
		"sharbert":  true,
		"fornax":    true,
	}
	splitS := strings.Split(s, " ")
	for i, word := range splitS {
		lower_word := strings.ToLower(word)
		if profaneWords[lower_word] {
			splitS[i] = "****"
		}
	}
	return strings.Join(splitS, " ")
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	type returnErr struct {
		Error string `json:"error"`
	}
	resErr := returnErr{
		Error: msg,
	}
	dat, err := json.Marshal(resErr)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "Internal Server Error"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "Internal Server Error"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}

func validateHandler(w http.ResponseWriter, r *http.Request){
    type parameters struct {
        Body string `json:"body"`
    }
	if r.Body == nil {
		respondWithError(w, http.StatusBadRequest, "Request body missing")
		return
	}
    decoder := json.NewDecoder(r.Body)
    params := parameters{}
    err := decoder.Decode(&params)
    if err != nil {
		errorString := "Something went wrong"
        respondWithError(w, http.StatusInternalServerError, errorString)
		return
    }
    if len(params.Body) > 140 {
		errorString := "Chirp is too long"
        respondWithError(w, http.StatusBadRequest, errorString)
		return
	}
	cleanedString := profaneChecker(params.Body)
	type returnString struct {
		CleanedBody string `json:"cleaned_body"`
	}
	resString := returnString{
		CleanedBody: cleanedString,
	}
    respondWithJSON(w, http.StatusOK, resString)
}

func main() {
	err := godotenv.Load()
    if err != nil {
        log.Fatal("Error loading .env file")
    }
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
        log.Fatal("Error accessing database")
    }
	dbQueries := database.New(db)
	config := &apiConfig{
		db: dbQueries,
	}
	server := http.NewServeMux()
	server.HandleFunc("GET /api/healthz", healthCheckHandler)
	dir := http.Dir(".")
	fServer := http.FileServer(dir)
	server.Handle("/app/", config.middlewareMetricsInc(http.StripPrefix("/app", fServer)))
	server.HandleFunc("GET /admin/metrics", config.metricCountHandler)
	server.HandleFunc("POST /admin/reset", config.metricResetHandler)
	server.HandleFunc("POST /api/validate_chirp", validateHandler)
	s := &http.Server{
		Addr:	":8080",
		Handler: server,
	}
	log.Println("Server starting on :8080")
	err = s.ListenAndServe()
	if err != nil {
		log.Printf("Error when running server. Error: %v", err)
		return
	}
	return
}