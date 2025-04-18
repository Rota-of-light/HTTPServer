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
	"errors"

	"github.com/Rota-of-light/HTTPServer/internal/database"
	"github.com/Rota-of-light/HTTPServer/internal/auth"
)

type apiConfig struct {
	db		*database.Queries
	fileserverHits atomic.Int32
	platform	string
	secret 		string
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
	Token	  string	`json:"token"`
	RefreshToken string `json:"refresh_token"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body     string    `json:"body"`
	UserID	 uuid.UUID    `json:"user_id"`
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
		Password string `json:"password"`
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
	hash, err := auth.HashPassword(params.Password)
	if err != nil {
		errorString := "Something went wrong when working with password"
        respondWithError(w, http.StatusInternalServerError, errorString)
		return
	}
	userParams := database.CreateUserParams{
		Email:	params.Email,
		HashedPassword: hash,
	}
	user, err := cfg.db.CreateUser(r.Context(), userParams)
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
    authString, err := auth.GetBearerToken(r.Header)
	if err != nil {
        respondWithError(w, http.StatusUnauthorized, err.Error())
		return
    }
	userID, err := auth.ValidateJWT(authString, cfg.secret)
	if err != nil {
        respondWithError(w, http.StatusUnauthorized, err.Error())
		return
    }
	type parameters struct {
        Body string `json:"body"`
    }
	if r.Body == nil {
		respondWithError(w, http.StatusBadRequest, "Request body missing")
		return
	}
    decoder := json.NewDecoder(r.Body)
    params := parameters{}
    err = decoder.Decode(&params)
    if err != nil {
		errorString := "Something went wrong when decoding request"
        respondWithError(w, http.StatusInternalServerError, errorString)
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
		UserID: userID,
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
		UserID: chirpRes.UserID,
	}
    respondWithJSON(w, http.StatusCreated, chirpJSON)
}

func (cfg *apiConfig) getChirpsHandler(w http.ResponseWriter, r *http.Request) {
	dbChirps, err := cfg.db.AllChirps(r.Context())
	if err != nil {
		errorString := "Error when attempting to retrive all chirps"
        respondWithError(w, http.StatusInternalServerError, errorString)
		return
    }
	chirps := make([]Chirp, len(dbChirps))
	for i, chirp := range dbChirps {
		chirps[i] = Chirp{
			ID:			chirp.ID,
			CreatedAt:	chirp.CreatedAt,
			UpdatedAt:	chirp.UpdatedAt,
			Body:		chirp.Body,
			UserID:		chirp.UserID,
		}
	}
	respondWithJSON(w, http.StatusOK, chirps)
}

func (cfg *apiConfig) getChirpByIDHandler(w http.ResponseWriter, r *http.Request) {
	chirpIDStr := r.PathValue("chirpID")
	chirpID, err := uuid.Parse(chirpIDStr)
    if err != nil {
        respondWithError(w, http.StatusBadRequest, "Invalid chirp ID format")
        return
    }
	chirp, err := cfg.db.GetChirp(r.Context(), chirpID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			errorString := "Chirp not found"
        	respondWithError(w, http.StatusNotFound, errorString)
			return
		}
		errorString := "Error when attempting to retrive chirp"
        respondWithError(w, http.StatusInternalServerError, errorString)
		return
    }
	chirpJSON := Chirp{
		ID:			chirp.ID,
		CreatedAt:	chirp.CreatedAt,
		UpdatedAt:	chirp.UpdatedAt,
		Body:		chirp.Body,
		UserID:		chirp.UserID,
	}
	respondWithJSON(w, http.StatusOK, chirpJSON)
}

func (cfg *apiConfig) loginHandler(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
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
        respondWithError(w, http.StatusBadRequest, errorString)
		return
    }
	
	user, err := cfg.db.GetUserByEmail(r.Context(), params.Email)
	if err != nil {
		errorString := "Incorrect email or password"
        respondWithError(w, http.StatusUnauthorized, errorString)
		return
    }
	err = auth.CheckPasswordHash(user.HashedPassword, params.Password)
	if err != nil {
		errorString := "Incorrect email or password"
        respondWithError(w, http.StatusUnauthorized, errorString)
		return
    }
	
	expiresIn := time.Duration(3600) * time.Second
	token, err := auth.MakeJWT(user.ID, cfg.secret, expiresIn)
	if err != nil {
		errorString := "Failure when attempting to create authentication token"
        respondWithError(w, http.StatusInternalServerError, errorString)
		return
	}
	refreshString, err := auth.MakeRefreshToken()
	if err != nil {
		errorString := "Failure when attempting to create refresh token"
        respondWithError(w, http.StatusInternalServerError, errorString)
		return
	}
	refreshExpiresIn := time.Now().Add(60 * 24 * time.Hour)
	refreshParams := database.CreateRefreshTokenParams{
		Token: refreshString,
		UserID: user.ID,
		ExpiresAt: refreshExpiresIn,
	}
	refreshToken, err := cfg.db.CreateRefreshToken(r.Context(), refreshParams)
	if err != nil {
		errorString := "Failure when attempting to insert refresh token"
        respondWithError(w, http.StatusInternalServerError, errorString)
		return
	}

	newUser := User{
		ID:	user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email: user.Email,
		Token: token,
		RefreshToken: refreshToken.Token,
	}
	respondWithJSON(w, http.StatusOK, newUser)
}

func (cfg *apiConfig) refreshHandler(w http.ResponseWriter, r *http.Request) {
	authString, err := auth.GetBearerToken(r.Header)
	if err != nil {
        respondWithError(w, http.StatusUnauthorized, err.Error())
		return
    }
	refreshToken, err := cfg.db.GetRefreshByToken(r.Context(), authString)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			errorString := "Authentication token not found"
        	respondWithError(w, http.StatusUnauthorized, errorString)
			return
		}
		errorString := "Failure when attempting to query for authentication token"
        respondWithError(w, http.StatusInternalServerError, errorString)
		return
	}
	if refreshToken.ExpiresAt.Before(time.Now()) {
		errorString := "Authentication token expired"
        respondWithError(w, http.StatusUnauthorized, errorString)
		return
	}
	user, err := cfg.db.GetUserByRefreshToken(r.Context(), refreshToken.Token)
	if err != nil {
		errorString := "Failure when attempting to query for user data"
        respondWithError(w, http.StatusInternalServerError, errorString)
		return
	}
	expiresIn := time.Duration(3600) * time.Second
	token, err := auth.MakeJWT(user.ID, cfg.secret, expiresIn)
	if err != nil {
		errorString := "Failure when attempting to create authentication token"
        respondWithError(w, http.StatusInternalServerError, errorString)
		return
	}
	type returnToken struct {
		Token string `json:"token"`
	}
	tokenJSON := returnToken{
		Token: token,
	}
	respondWithJSON(w, http.StatusOK, tokenJSON)
}

func (cfg *apiConfig) revokeHandler(w http.ResponseWriter, r *http.Request) {
	authString, err := auth.GetBearerToken(r.Header)
	if err != nil {
        respondWithError(w, http.StatusUnauthorized, err.Error())
		return
    }
	revokeStatus, err := cfg.db.CheckRevokeStatus(r.Context(), authString)
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithError(w, http.StatusUnauthorized, "Invalid or non-existent token")
			return
		}
		errorString := "Failed to fetch token status"
        respondWithError(w, http.StatusInternalServerError, errorString)
		return
	}
	if revokeStatus.Valid {
		respondWithError(w, http.StatusConflict, "Authentication has already been revoked")
		return
	}
	revokeParams := database.RevokeRefreshParams{
		UpdatedAt: time.Now(),
		Token:	   authString,
	}
	err = cfg.db.RevokeRefresh(r.Context(), revokeParams)
	if err != nil {
		errorString := "Something went wrong when trying to revoke authentication"
        respondWithError(w, http.StatusInternalServerError, errorString)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (cfg *apiConfig) updateUserPassHandler(w http.ResponseWriter, r *http.Request) {
	authString, err := auth.GetBearerToken(r.Header)
	if err != nil {
        respondWithError(w, http.StatusUnauthorized, err.Error())
		return
    }
	userID, err := auth.ValidateJWT(authString, cfg.secret)
	if err != nil {
        respondWithError(w, http.StatusUnauthorized, err.Error())
		return
    }

	type parameters struct {
		Password string `json:"password"`
        Email string `json:"email"`
    }
	if r.Body == nil {
		respondWithError(w, http.StatusBadRequest, "Request body missing")
		return
	}
    decoder := json.NewDecoder(r.Body)
    params := parameters{}
    err = decoder.Decode(&params)
    if err != nil {
		errorString := "Something went wrong"
        respondWithError(w, http.StatusInternalServerError, errorString)
		return
    }

	hash, err := auth.HashPassword(params.Password)
	if err != nil {
		errorString := "Something went wrong when working with password"
        respondWithError(w, http.StatusInternalServerError, errorString)
		return
	}
	userParams := database.UpdateUserPassParams{
		ID:		userID,
		Email:	params.Email,
		HashedPassword: hash,
	}
	err = cfg.db.UpdateUserPass(r.Context(), userParams)
	if err != nil {
		errorString := "Something went wrong when attempting to update user's email and password"
        respondWithError(w, http.StatusInternalServerError, errorString)
		return
	}
	user, err := cfg.db.GetUserByID(r.Context(), userID)
	if err != nil {
		errorString := "Something went wrong when attempting to retrieve user's information"
        respondWithError(w, http.StatusInternalServerError, errorString)
		return
	}
	newUser := User{
		ID:	user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email: user.Email,
	}
	respondWithJSON(w, http.StatusOK, newUser)
}

func (cfg *apiConfig) deleteChirpsHandler(w http.ResponseWriter, r *http.Request){
    authString, err := auth.GetBearerToken(r.Header)
	if err != nil {
        respondWithError(w, http.StatusUnauthorized, err.Error())
		return
    }
	userID, err := auth.ValidateJWT(authString, cfg.secret)
	if err != nil {
        respondWithError(w, http.StatusUnauthorized, err.Error())
		return
    }
	chirpIDStr := r.PathValue("chirpID")
	chirpID, err := uuid.Parse(chirpIDStr)
    if err != nil {
        respondWithError(w, http.StatusBadRequest, "Invalid chirp ID format")
        return
    }
	chirp, err := cfg.db.GetChirp(r.Context(), chirpID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			errorString := "Chirp not found"
        	respondWithError(w, http.StatusNotFound, errorString)
			return
		}
		errorString := "Error when attempting to find chirp"
        respondWithError(w, http.StatusInternalServerError, errorString)
		return
    }
	if chirp.UserID != userID {
		respondWithError(w, http.StatusForbidden, "User does not own this chirp")
		return
	}
	err = cfg.db.DeleteChirp(r.Context(), chirpID)
	if err != nil {
		errorString := "Error when attempting to delete chirp"
        respondWithError(w, http.StatusInternalServerError, errorString)
		return
    }
    w.WriteHeader(http.StatusNoContent)
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
		secret:	os.Getenv("JWT_SECRET"),
	}
	server := http.NewServeMux()
	server.HandleFunc("GET /api/healthz", healthCheckHandler)
	dir := http.Dir(".")
	fServer := http.FileServer(dir)
	server.Handle("/app/", config.middlewareMetricsInc(http.StripPrefix("/app", fServer)))
	server.HandleFunc("GET /admin/metrics", config.metricCountHandler)
	server.HandleFunc("GET /api/chirps", config.getChirpsHandler)
	server.HandleFunc("GET /api/chirps/{chirpID}", config.getChirpByIDHandler)
	server.HandleFunc("POST /admin/reset", config.adminResetHandler)
	server.HandleFunc("POST /api/users", config.createUserHandler)
	server.HandleFunc("POST /api/chirps", config.chirpsHandler)
	server.HandleFunc("POST /api/login", config.loginHandler)
	server.HandleFunc("POST /api/refresh", config.refreshHandler)
	server.HandleFunc("POST /api/revoke", config.revokeHandler)
	server.HandleFunc("PUT /api/users", config.updateUserPassHandler)
	server.HandleFunc("DELETE /api/chirps/{chirpID}", config.deleteChirpsHandler)
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