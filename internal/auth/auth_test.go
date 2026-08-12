package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

//"time"

//"github.com/alexedwards/argon2id"
//"github.com/golang-jwt/jwt/v5"
//"github.com/google/uuid"

/*
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
*/

func TestMakeJWTAndValidateJWT(t *testing.T) {
	// we test if user matches
	userid := uuid.New()
	secret := "test-secret"
	expiration := time.Hour

	cJTW, err := MakeJWT(userid, secret, expiration)
	if err != nil {
		t.Fatalf("failed to create jwt; ERR: %s", err)
	}
	// created jwt
	vUUID, err := ValidateJWT(cJTW, secret)
	if err != nil {
		t.Fatalf("failed to create read jwt; ERR: %s", err)
	}

	// test
	if userid != vUUID {
		t.Errorf("non matching uuid;: %s %s", userid, vUUID)
	}
}

func TestValidateJWTExpired(t *testing.T) {
	// we test if user matches
	userid := uuid.New()
	secret := "test-secret"
	expiration := -time.Hour
	cJTW, err := MakeJWT(userid, secret, expiration)
	if err != nil {
		t.Fatalf("failed to create jwt; ERR: %s", err)
	}
	// created jwt
	_, err = ValidateJWT(cJTW, secret)
	if err == nil {
		t.Errorf("failed to return valid jwt; ERR: %s", err)
	}
}

func TestValidateJWTWrongSecret(t *testing.T) {
	// we test if user matches
	userid := uuid.New()
	secret := "test-secret"
	expiration := time.Hour

	cJTW, err := MakeJWT(userid, secret, expiration)
	if err != nil {
		t.Fatalf("failed to create jwt; ERR: %s", err)
	}
	// try to validate with diferetn secret
	_, err = ValidateJWT(cJTW, "badSECRET")
	if err == nil {
		t.Errorf("failed to read jwt sismatch secret; ERR: %s", err)
	}
}
