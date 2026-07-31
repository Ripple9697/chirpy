package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"

	"github.com/Ripple9697/chirpy/internal/database"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

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
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(http.StatusText(http.StatusOK)))
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

	conf := apiConfig{
		fileserverHits: atomic.Int32{},
		db:             database.New(db),
	}

	fileServ := http.StripPrefix("/app", http.FileServer(http.Dir(filepathRoot)))
	mux := http.NewServeMux()
	mux.Handle("/app/", conf.middlewareMetricsInc(fileServ))
	mux.HandleFunc("GET /api/healthz", handlerReadiness)
	mux.HandleFunc("GET /admin/metrics", conf.handlerPrint)
	mux.HandleFunc("POST /admin/reset", conf.handlerReset)
	mux.HandleFunc("POST /api/validate_chirp", handlerValidate)

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

func handlerValidate(w http.ResponseWriter, r *http.Request) {
	blacklist := map[string]struct{}{
		"kerfuffle": {},
		"sharbert":  {},
		"fornax":    {},
	}
	type Req struct {
		Body string `json:"body"`
	}
	type validResp struct {
		Valid bool `json:"valid"`
	}
	type CleanedResp struct {
		CleanedBody string `json:"cleaned_body"`
	}

	respReq := Req{}

	err := json.NewDecoder(r.Body).Decode(&respReq)
	if err != nil {
		log.Printf("Error decoding parameters %s", err)
		//w.WriteHeader(500)
		respondWithError(w, 500, fmt.Sprintf("Error decoding parameters %s", err))
		return
	}
	if len(respReq.Body) > 140 {
		respondWithError(w, 400, "Chirp is too long")
		return
	}
	words := strings.Split(respReq.Body, " ")
	for i := range words {
		_, banned := blacklist[strings.ToLower(words[i])]
		if banned {
			words[i] = "****"
		}
	}
	finalString := strings.Join(words, " ")
	fmt.Println(words)
	respondWithJSON(w, 200, CleanedResp{CleanedBody: finalString})
}

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
