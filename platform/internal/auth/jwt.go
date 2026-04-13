package auth

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID          string `json:"id"`
	Email           string `json:"email"`
	Name            string `json:"name"`
	IsPlatformAdmin bool   `json:"isPlatformAdmin"`
	ImpersonatedBy  string `json:"impersonatedBy,omitempty"`
}

// ImpersonationData is the JSON structure stored in the bridge-impersonate cookie.
type ImpersonationData struct {
	OriginalUserID string `json:"originalUserId"`
	TargetUserID   string `json:"targetUserId"`
	TargetName     string `json:"targetName"`
	TargetEmail    string `json:"targetEmail"`
}

func VerifyToken(tokenString string, secret string) (*Claims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("jwt parse: %w", err)
	}

	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	claims := &Claims{}
	if v, ok := mapClaims["id"].(string); ok {
		claims.UserID = v
	}
	if v, ok := mapClaims["email"].(string); ok {
		claims.Email = v
	}
	if v, ok := mapClaims["name"].(string); ok {
		claims.Name = v
	}
	if v, ok := mapClaims["isPlatformAdmin"].(bool); ok {
		claims.IsPlatformAdmin = v
	}

	return claims, nil
}
