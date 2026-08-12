package auth

import (
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
