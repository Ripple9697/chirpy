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
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Ripple9697/chirpy/internal/auth"
	"github.com/Ripple9697/chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"

	_ "github.com/lib/pq"
)

type UserResponse struct {
	ID          uuid.UUID `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Email       string    `json:"email"`
	IsChirpyRed bool      `json:"is_chirpy_red"`
}
type ResponceLogin struct {
	ID           uuid.UUID `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Email        string    `json:"email"`
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token"`
	IsChirpyRed  bool      `json:"is_chirpy_red"`
}

type ChirpResponse struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

type RequestEmail struct {
	Password string `json:"password"`
	Email    string `json:"email"`
}

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
	secret         string
	polkaSecret    string
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
	Secret := os.Getenv("Secret")
	PolkaSecret := os.Getenv("POLKA_KEY")

	const filepathRoot = "."
	const port = "8080"
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Error connecting to db; %d", err)
	}

	cfg := apiConfig{
		fileserverHits: atomic.Int32{},
		db:             database.New(db),
		secret:         Secret,
		polkaSecret:    PolkaSecret,
	}

	fileServ := http.StripPrefix("/app", http.FileServer(http.Dir(filepathRoot)))
	mux := http.NewServeMux()
	mux.Handle("/app/", cfg.middlewareMetricsInc(fileServ))
	mux.HandleFunc("GET /api/healthz", handlerReadiness)
	mux.HandleFunc("GET /admin/metrics", cfg.handlerPrint)
	mux.HandleFunc("POST /admin/reset", cfg.handlerReset)
	mux.HandleFunc("POST /api/users", cfg.handlerUserCreate)
	mux.HandleFunc("POST /api/chirps", cfg.handlerChirpCreate)
	mux.HandleFunc("GET /api/chirps", cfg.handlerGetChirps)
	mux.HandleFunc("GET /api/chirps/{chirpID}", cfg.handlerGetChirp)
	mux.HandleFunc("POST /api/login", cfg.handlerUserLogin)
	mux.HandleFunc("POST /api/refresh", cfg.handlerRefreshToken)
	mux.HandleFunc("POST /api/revoke", cfg.handlerRefreshTokenRevoke)
	mux.HandleFunc("PUT /api/users", cfg.handlerUpdateUser)
	mux.HandleFunc("DELETE /api/chirps/{chirpID}", cfg.handlerDeleteChirp)
	mux.HandleFunc("POST /api/polka/webhooks", cfg.handlerUpgradeUserChirpRed)

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
	reqEmail := RequestEmail{}
	err := json.NewDecoder(r.Body).Decode(&reqEmail)
	if err != nil {
		log.Printf("Error decoding parameters %s", err)
		respondWithError(w, 500, fmt.Sprintf("Error decoding parameters %s", err))
		return
	}
	hashed, err := auth.HashPassword(reqEmail.Password)
	if err != nil {
		log.Printf("Error decoding parameters %s", err)
		respondWithError(w, 500, fmt.Sprintf("Error decoding parameters %s", err))
		return
	}
	dbUser, err := cfg.db.CreateUser(context.Background(), database.CreateUserParams{
		Email:          reqEmail.Email,
		HashedPassword: hashed,
	})
	if err != nil {
		log.Printf("Failed to CreateUser: %s", err)
		respondWithError(w, 500, "Failed to Create user")
		return
	}
	respondWithJSON(w, 201, UserResponse{
		ID:          dbUser.ID,
		CreatedAt:   dbUser.CreatedAt,
		UpdatedAt:   dbUser.UpdatedAt,
		Email:       dbUser.Email,
		IsChirpyRed: dbUser.IsChirpyRed,
	})
}

func (cfg *apiConfig) handlerUpdateUser(w http.ResponseWriter, r *http.Request) {
	aTokenString, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("Error GetBearerToken %s", err)
		respondWithError(w, 401, "bad")
		return
	}
	userID, err := auth.ValidateJWT(aTokenString, cfg.secret)
	if err != nil {
		log.Printf("Error GetUserFromRefreshToken %s", err)
		respondWithError(w, 401, "bad")

		return
	}
	reqEmail := RequestEmail{}
	err = json.NewDecoder(r.Body).Decode(&reqEmail)
	if err != nil {
		log.Printf("Error decoding parameters %s", err)
		respondWithError(w, 401, "bad")

		return
	}
	hashed, err := auth.HashPassword(reqEmail.Password)
	if err != nil {
		log.Printf("Error decoding parameters %s", err)
		respondWithError(w, 401, "bad")
		return
	}
	user, err := cfg.db.UpdateUserCreds(context.Background(), database.UpdateUserCredsParams{
		ID:             userID,
		Email:          reqEmail.Email,
		HashedPassword: hashed,
	})
	if err != nil {
		log.Printf("Error decoding parameters %s", err)
		respondWithError(w, 401, "bad")
		return
	}
	respondWithJSON(w, 200, struct {
		Email string `json:"email"`
	}{Email: user.Email})
}

func (cfg *apiConfig) handlerUserLogin(w http.ResponseWriter, r *http.Request) {
	reqEmail := RequestEmail{}
	err := json.NewDecoder(r.Body).Decode(&reqEmail)
	if err != nil {
		log.Printf("Error decoding parameters %s", err)
		respondWithError(w, 401, "Incorrect email or password")
		return
	}
	user, err := cfg.db.GetUser(context.Background(), reqEmail.Email)
	if err != nil {
		log.Printf("Error getting user by email %s", err)
		respondWithError(w, 401, "Incorrect email or password")
		return
	}
	t, err := auth.CheckPasswordHash(reqEmail.Password, user.HashedPassword)
	if !t {
		log.Printf("Hash mismatch %s", err)
		respondWithError(w, 401, "Incorrect email or password")
		return
	}
	authString, err := auth.MakeJWT(user.ID, cfg.secret, time.Hour)
	if err != nil {
		log.Printf("failed to make authstring JWT %s", err)
		respondWithError(w, 401, "failure")
		return
	}
	rToken, err := cfg.db.CreateRefreshToken(context.Background(), database.CreateRefreshTokenParams{
		Token:     auth.MakeRefreshToken(),
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(60 * 24 * time.Hour),
	})
	if err != nil {
		log.Printf("failed to make refreshtoken: %s", err)
		respondWithError(w, 401, "failure")
		return
	}

	respondWithJSON(w, 200, ResponceLogin{
		ID:           user.ID,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
		Email:        user.Email,
		Token:        authString,
		RefreshToken: rToken.Token,
		IsChirpyRed:  user.IsChirpyRed,
	})
}

func (cfg *apiConfig) handlerChirpCreate(w http.ResponseWriter, r *http.Request) {
	type chirp struct {
		Body string `json:"body"`
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
	jwtAuthString, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("failed get GetBearerToken: %s", err)
		respondWithError(w, 401, "failed")
		return

	}
	user, err := auth.ValidateJWT(jwtAuthString, cfg.secret)
	if err != nil {
		log.Printf("failed get user id: %s", err)
		respondWithError(w, 401, "failed to get UserID")
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

func (cfg *apiConfig) handlerRefreshToken(w http.ResponseWriter, r *http.Request) {
	type responceToken struct {
		Token string `json:"token"`
	}
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("failed get GetBearerToken: %s", err)
		respondWithError(w, 401, "failed")
		return
	}
	rToken, err := cfg.db.GetUserFromRefreshToken(context.Background(), refreshToken)
	if err != nil {
		log.Printf("failed to GetUserFromRefreshToken DB: %s", err)
		respondWithError(w, 401, "failed")
		return
	}
	if rToken.RevokedAt.Valid || time.Now().After(rToken.ExpiresAt) {
		log.Printf("failed to validate rtoken: %s", err)
		respondWithError(w, 401, "failed")
		return
	}

	aToken, err := auth.MakeJWT(rToken.UserID, cfg.secret, time.Hour)
	if err != nil {
		log.Printf("failed to make jwt: %s", err)
		respondWithError(w, 401, "failed")
		return
	}
	respondWithJSON(w, 200, responceToken{Token: aToken})
}

func (cfg *apiConfig) handlerRefreshTokenRevoke(w http.ResponseWriter, r *http.Request) {
	type responceToken struct {
		Token string `json:"token"`
	}
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("failed get GetBearerToken: %s", err)
		respondWithError(w, 401, "failed")
		return
	}
	err = cfg.db.RevokeRefreshToken(context.Background(), refreshToken)
	if err != nil {
		log.Printf("failed to RevokeRefreshToken: %s", err)
		respondWithError(w, 401, "failed")
		return
	}
	respondWithJSON(w, 204, nil)
}

func (cfg *apiConfig) handlerGetChirps(w http.ResponseWriter, r *http.Request) {
	var dbChirps []database.Chirp
	var err error

	order := r.URL.Query().Get("sort")
	if order != "desc" {
		order = "asc"
	}

	authorID := r.URL.Query().Get("author_id")
	if authorID == "" {
		dbChirps, err = cfg.db.GetChirps(r.Context())
	} else {
		userID, parseErr := uuid.Parse(authorID)
		if parseErr != nil {
			respondWithError(w, 400, "invalid author_id")
			return
		}
		dbChirps, err = cfg.db.GetChirpsByAuthor(r.Context(), userID)
	}
	if err != nil {
		log.Printf("failed to read DB: %s", err)
		respondWithError(w, 500, "failed to read DB")
		return
	}
	if order == "asc" {
		sort.Slice(dbChirps, func(i, j int) bool { return dbChirps[i].CreatedAt.Before(dbChirps[j].CreatedAt) })
	} else {
		sort.Slice(dbChirps, func(i, j int) bool { return dbChirps[i].CreatedAt.After(dbChirps[j].CreatedAt) })
	}
	chirps := make([]ChirpResponse, len(dbChirps))
	for i, c := range dbChirps {
		chirps[i] = ChirpResponse{
			ID:        c.ID,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
			Body:      c.Body,
			UserID:    c.UserID,
		}
	}
	respondWithJSON(w, 200, chirps)
}

func (cfg *apiConfig) handlerUpgradeUserChirpRed(w http.ResponseWriter, r *http.Request) {
	apiKey, err := auth.GetAPIKey(r.Header)
	if apiKey != cfg.polkaSecret {
		respondWithError(w, 401, "bad")
		return
	}

	if err != nil {
		respondWithError(w, 500, "bad")
		return
	}
	RequestUpgrade := struct {
		Event string `json:"event"`
		Data  struct {
			UserID uuid.UUID `json:"user_id"`
		} `json:"data"`
	}{}
	err = json.NewDecoder(r.Body).Decode(&RequestUpgrade)
	if err != nil {
		respondWithError(w, 500, "bad")
		return
	}
	if RequestUpgrade.Event != "user.upgraded" {
		respondWithError(w, 204, "invalid input")
		return
	}
	err = cfg.db.UpgradeUserChirpyRed(context.Background(), RequestUpgrade.Data.UserID)
	if err != nil {
		respondWithError(w, 404, "bad")
		return
	}

	respondWithJSON(w, 204, "")
}

func (cfg *apiConfig) handlerGetChirp(w http.ResponseWriter, r *http.Request) {
	chirpID, err := uuid.Parse(r.PathValue("chirpID"))
	if err != nil {
		log.Printf("failed to parse UserID: %s", err)
		respondWithError(w, 401, "failed to parse UserID")
		return
	}
	chirp, err := cfg.db.GetChirp(context.Background(), chirpID)
	if err != nil {
		log.Printf("failed to read DB: %s", err)
		respondWithError(w, http.StatusNotFound, "failed to read DB")
		return
	}
	respChirp := ChirpResponse{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	}
	respondWithJSON(w, 200, respChirp)
}

func (cfg *apiConfig) handlerDeleteChirp(w http.ResponseWriter, r *http.Request) {
	chirpID, err := uuid.Parse(r.PathValue("chirpID"))
	if err != nil {
		log.Printf("failed to uuid.parse chirpid: %s", err)
		respondWithError(w, 400, "failed")
		return
	}
	aTokenString, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("failed to GetBearerToken: %s", err)
		respondWithError(w, 401, "failed")
		return
	}
	userID, err := auth.ValidateJWT(aTokenString, cfg.secret)
	if err != nil {
		log.Printf("failed to ValidateJWT: %s", err)
		respondWithError(w, 401, "failed")
		return
	}
	chirp, err := cfg.db.GetChirp(context.Background(), chirpID)
	if err != nil {
		log.Printf("failed to GetChirp %s", err)
		respondWithError(w, 404, "bad")
		return
	}
	if chirp.UserID != userID {
		log.Printf("mistach chirp user pair; %s %s", chirp.UserID, userID)
		respondWithError(w, 403, "bad")
		return
	}
	_, err = cfg.db.DeleteChirp(context.Background(), chirpID)
	if err != nil {
		log.Printf("Error DeleteChirp %s", err)
		respondWithError(w, 500, "bad")
		return
	}
	respondWithJSON(w, 204, "")
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
