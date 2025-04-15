package auth

import (
	"golang.org/x/crypto/bcrypt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"time"
	"fmt"
	"net/http"
	"strings"
	"crypto/rand"
	"encoding/hex"
)

func HashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func CheckPasswordHash(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	claim := jwt.RegisteredClaims{
		Issuer: "chirpy",
		IssuedAt: jwt.NewNumericDate(time.Now().UTC()),
		ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(expiresIn)),
		Subject: userID.String(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claim)
	return token.SignedString([]byte(tokenSecret))
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	claim := jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(tokenString, &claim, func(token *jwt.Token) (interface{}, error) { return []byte(tokenSecret), nil })
	if err != nil {
		return uuid.Nil, err
	}
	userIDStr, err := token.Claims.GetSubject()
	if err != nil {
		return uuid.Nil, err
	}
	whoGave, err := token.Claims.GetIssuer()
	if err != nil {
		return uuid.Nil, err
	}
	if whoGave != "chirpy" {
		return uuid.Nil, fmt.Errorf("Invalid issuer: %v", whoGave)
	}
	id, err := uuid.Parse(userIDStr)
	if err != nil {
		return uuid.Nil, fmt.Errorf("Invalid user ID: %v", err)
	}
	return id, nil
}

func GetBearerToken(headers http.Header) (string, error) {
	authHeader := headers.Get("Authorization")
	if authHeader == "" {
		return "", fmt.Errorf("Error: no authorization in header")
	}
	authString := strings.Split(authHeader, " ")
	if len(authString) < 2 {
		return "", fmt.Errorf("Error: authorization header format must be: Bearer {TOKEN}")
	}
	if authString[0] != "Bearer" {
		return "", fmt.Errorf("Error: authorization header must start with 'Bearer'")
	}
	return authString[1], nil
}

func MakeRefreshToken() (string, error) {
	key := make([]byte, 32)
	rand.Read(key)
	if _, err := rand.Read(key); err != nil {
        return "", err
    }
	return hex.EncodeToString(key), nil
}