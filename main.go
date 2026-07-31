package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Ripple9697/chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"

	_ "github.com/lib/pq"
)

type UserResponse struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type ChirpResponse struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//log.Printf("%s %s", r.Method, r.URL.Path)
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	cfg.fileserverHits.Store(0)
	err := cfg.db.DeleteUsers(context.Background())
	if err != nil {
		log.Printf("Couldent Delete: %S", err)
	}
	respondWithJSON(w, http.StatusOK, "")
}

func (cfg *apiConfig) handlerPrint(w http.ResponseWriter, r *http.Request) {
	count := cfg.fileserverHits.Load()
	w.Header().Add("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	w.Write(fmt.Appendf(nil, "<html><body><h1>Welcome, Chirpy Admin</h1><p>Chirpy has been visited %d times!</p></body></html>", count))
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	const filepathRoot = "."
	const port = "8080"
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Error connecting to db; %d", err)
	}

	cfg := apiConfig{
		fileserverHits: atomic.Int32{},
		db:             database.New(db),
	}

	fileServ := http.StripPrefix("/app", http.FileServer(http.Dir(filepathRoot)))
	mux := http.NewServeMux()
	mux.Handle("/app/", cfg.middlewareMetricsInc(fileServ))
	mux.HandleFunc("GET /api/healthz", handlerReadiness)
	mux.HandleFunc("GET /admin/metrics", cfg.handlerPrint)
	mux.HandleFunc("POST /admin/reset", cfg.handlerReset)
	mux.HandleFunc("POST /api/users", cfg.handlerUserCreate)
	mux.HandleFunc("POST /api/chirps", cfg.handlerChirpCreate)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	log.Printf("Serving files from %s on port: %s\n", filepathRoot, port)
	log.Fatal(srv.ListenAndServe())
}

func handlerReadiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(http.StatusText(http.StatusOK)))
}

func (cfg *apiConfig) handlerUserCreate(w http.ResponseWriter, r *http.Request) {
	type requestEmail struct {
		Email string `json:"email"`
	}
	reqEmail := requestEmail{}
	err := json.NewDecoder(r.Body).Decode(&reqEmail)
	if err != nil {
		log.Printf("Error decoding parameters %s", err)
		respondWithError(w, 500, fmt.Sprintf("Error decoding parameters %s", err))
		return
	}

	dbUser, err := cfg.db.CreateUser(context.Background(), reqEmail.Email)
	createdUser := UserResponse{
		ID:        dbUser.ID,
		CreatedAt: dbUser.CreatedAt,
		UpdatedAt: dbUser.UpdatedAt,
		Email:     dbUser.Email,
	}
	if err != nil {
		log.Printf("Failed to CreateUser: %s", err)
	}
	respondWithJSON(w, 201, createdUser)
}

func (cfg *apiConfig) handlerChirpCreate(w http.ResponseWriter, r *http.Request) {
	type chirp struct {
		Body   string `json:"body"`
		UserID string `json:"user_id"`
	}
	reqChirp := chirp{}
	err := json.NewDecoder(r.Body).Decode(&reqChirp)
	if err != nil {
		log.Printf("Error decoding parameters %s", err)
		respondWithError(w, 500, fmt.Sprintf("Error decoding parameters %s", err))
		return
	}
	cleanBody, err := cleanChirp(reqChirp.Body)
	if err != nil {
		log.Printf("failed to clean string: %s", err)
		respondWithError(w, 401, "failed to clean string")
		return
	}
	user, err := uuid.Parse(reqChirp.UserID)
	if err != nil {
		log.Printf("failed to parse UserID: %s", err)
		respondWithError(w, 401, "failed to parse UserID")
		return
	}
	dbChirp, err := cfg.db.CreateChirp(context.Background(), database.CreateChirpParams{
		Body:   cleanBody,
		UserID: user,
	})
	if err != nil {
		log.Printf("failed to Create Chirp: %s", err)
		respondWithError(w, 401, "failed to create Chirp")
		return
	}
	valid := ChirpResponse{
		ID:        dbChirp.ID,
		CreatedAt: dbChirp.CreatedAt,
		UpdatedAt: dbChirp.UpdatedAt,
		Body:      dbChirp.Body,
		UserID:    dbChirp.UserID,
	}
	respondWithJSON(w, 201, valid)
}

// tools
func respondWithError(w http.ResponseWriter, code int, msg string) {
	type errResp struct {
		Error string `json:"error"`
	}
	respondWithJSON(w, code, errResp{Error: msg})
}

func respondWithJSON(w http.ResponseWriter, code int, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}

func cleanChirp(body string) (string, error) {
	blacklist := map[string]struct{}{
		"kerfuffle": {},
		"sharbert":  {},
		"fornax":    {},
	}

	if len(body) > 140 {
		return "", errors.New("Body to long")
	}
	words := strings.Split(body, " ")
	for i := range words {
		_, banned := blacklist[strings.ToLower(words[i])]
		if banned {
			words[i] = "****"
		}
	}
	finalBody := strings.Join(words, " ")
	return finalBody, nil
}
