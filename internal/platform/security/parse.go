package security

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v4"
)

var ErrInvalidToken = errors.New("invalid or expired token")

// ParseAccessToken validates a Bearer JWT and returns the subject (user id).
func ParseAccessToken(secret, raw string) (string, error) {
	token, err := jwt.Parse(raw, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || token == nil || !token.Valid {
		return "", ErrInvalidToken
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", ErrInvalidToken
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", ErrInvalidToken
	}
	return sub, nil
}
