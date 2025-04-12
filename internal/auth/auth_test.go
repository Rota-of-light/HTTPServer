package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/golang-jwt/jwt/v5"
)

func TestValidateJWT_ValidToken(t *testing.T) {
	tokenSecret := "supersecretkey"
	userID := uuid.New() // Generate a new random ID
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:   "chirpy",
		Subject:  userID.String(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)), // Token expires in 1 hour
	})
	tokenString, err := token.SignedString([]byte(tokenSecret))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	// Call the function
	returnedUUID, err := ValidateJWT(tokenString, tokenSecret)

	// Assertions
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if returnedUUID != userID {
		t.Errorf("expected UUID %v, got %v", userID, returnedUUID)
	}
}