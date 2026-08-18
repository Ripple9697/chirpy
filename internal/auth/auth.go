package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func HashPassword(password string) (string, error) {
	return argon2id.CreateHash(password, argon2id.DefaultParams)
}

func CheckPasswordHash(password, hash string) (bool, error) {
	return argon2id.ComparePasswordAndHash(password, hash)
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	tNow := time.Now().UTC()
	tNowJWT := jwt.NewNumericDate(tNow)
	tExpiresAtJWT := jwt.NewNumericDate(tNow.Add(expiresIn))

	claims := jwt.RegisteredClaims{
		Issuer:    "chirpy-access",
		IssuedAt:  tNowJWT,
		ExpiresAt: tExpiresAtJWT,
		Subject:   userID.String(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	thing, err := token.SignedString([]byte(tokenSecret))
	return thing, err
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	claim := jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(tokenString, &claim, func(token *jwt.Token) (any, error) {
		return []byte(tokenSecret), nil
		//return []byte(token.Signature), nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	sUUID, err := token.Claims.GetSubject()
	if err != nil {
		return uuid.Nil, err
	}

	id, err := uuid.Parse(sUUID)
	if err != nil {
		return uuid.Nil, err
	}

	return id, nil
}

func GetBearerToken(headers http.Header) (string, error) {
	authString := headers.Get("Authorization")
	if len(authString) <= 7 {
		return "", errors.New("no jwt in authorization header")
	}
	token, ok := strings.CutPrefix(authString, "Bearer ")
	if !ok {
		return "", errors.New("couldent cutPrefix")
	}
	if token == "" {
		return "", errors.New("no jwt in authorization header")
	}

	return token, nil
}

func MakeRefreshToken() string {
	key := make([]byte, 32)
	rand.Read(key)
	return hex.EncodeToString(key)
}

func GetAPIKey(headers http.Header) (string, error) { // untested
	authString := headers.Get("Authorization")
	if len(authString) <= 7 {
		return "", errors.New("no jwt in authorization header")
	}
	token, ok := strings.CutPrefix(authString, "ApiKey ")
	if !ok {
		return "", errors.New("couldent cutPrefix")
	}
	if token == "" {
		return "", errors.New("no jwt in authorization header")
	}

	return token, nil
}
