package main

import (
	_ "github.com/lib/pq" //Needed for side-effect
	"github.com/joho/godotenv"
	"github.com/google/uuid"

	"os"
	"database/sql"
	"net/http"
	"log"
	"sync/atomic"
	"fmt"
	"encoding/json"
	"strings"
	"time"

	"github.com/Rota-of-light/HTTPServer/internal/database"
)

type apiConfig struct {
	db		*database.Queries
	fileserverHits atomic.Int32
	platform	string
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body     string    `json:"body"`
	UserId	 uuid.UUID    `json:"user_id"`
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

func (cfg *apiConfig) adminResetHandler(w http.ResponseWriter, r *http.Request) {
	if cfg.platform != "dev" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("I can't let you do that."))
		return
	}
	err := cfg.db.Reset(r.Context())
    if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Something went wrong when attempting to delete all users"))
		return
    }
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	cfg.fileserverHits.Store(0)
	w.Write([]byte("Reset successful!"))
}

func (cfg *apiConfig) createUserHandler(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
        Email string `json:"email"`
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
	user, err := cfg.db.CreateUser(r.Context(), params.Email)
	if err != nil {
		errorString := "Something went wrong when attempting to create user"
        respondWithError(w, http.StatusInternalServerError, errorString)
		return
	}
	newUser := User{
		ID:	user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email: user.Email,
	}
	respondWithJSON(w, http.StatusCreated, newUser)
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

func (cfg *apiConfig) chirpsHandler(w http.ResponseWriter, r *http.Request){
    type parameters struct {
        Body string `json:"body"`
		UserId uuid.UUID `json:"user_id"`
    }
	if r.Body == nil {
		respondWithError(w, http.StatusBadRequest, "Request body missing")
		return
	}
    decoder := json.NewDecoder(r.Body)
    params := parameters{}
    err := decoder.Decode(&params)
    if err != nil {
		errorString := "Something went wrong when decoding request"
        respondWithError(w, http.StatusInternalServerError, errorString)
		return
    }
	user, err := cfg.db.GetUserByID(r.Context(), params.UserId)
	if len(user.ID) == 0 || err != nil {
		errorString := "Invalid user ID"
        respondWithError(w, http.StatusBadRequest, errorString)
		return
    }
    if len(params.Body) > 140 {
		errorString := "Chirp is too long"
        respondWithError(w, http.StatusBadRequest, errorString)
		return
	}
	cleanedString := profaneChecker(params.Body)
	chirpParam := database.CreateChirpParams{
		Body:   cleanedString,
		UserID: user.ID,
	}
	chirpRes, err := cfg.db.CreateChirp(r.Context(), chirpParam)
	if err != nil {
		errorString := "Error when attempting to create chirp"
        respondWithError(w, http.StatusInternalServerError, errorString)
		return
    }
	chirpJSON := Chirp{
		ID: chirpRes.ID,
		CreatedAt: chirpRes.CreatedAt,
		UpdatedAt: chirpRes.UpdatedAt,
		Body: chirpRes.Body,
		UserId: chirpRes.UserID,
	}
    respondWithJSON(w, http.StatusCreated, chirpJSON)
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
		platform: os.Getenv("PLATFORM"),
	}
	server := http.NewServeMux()
	server.HandleFunc("GET /api/healthz", healthCheckHandler)
	dir := http.Dir(".")
	fServer := http.FileServer(dir)
	server.Handle("/app/", config.middlewareMetricsInc(http.StripPrefix("/app", fServer)))
	server.HandleFunc("GET /admin/metrics", config.metricCountHandler)
	server.HandleFunc("POST /admin/reset", config.adminResetHandler)
	server.HandleFunc("POST /api/users", config.createUserHandler)
	server.HandleFunc("POST /api/chirps", config.chirpsHandler)
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