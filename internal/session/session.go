package session

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

type Claims struct {
	TwitchUserID string `json:"sub"`
	TwitchLogin  string `json:"login"`
	Role         string `json:"role"`
	ExpiresAt    int64  `json:"exp"`
}

type Signer struct {
	secret []byte
	ttl    time.Duration
}

func New(secret string, ttl time.Duration) *Signer {
	return &Signer{secret: []byte(secret), ttl: ttl}
}

func (s *Signer) Issue(userID, login, role string) (string, error) {
	payload := Claims{
		TwitchUserID: userID,
		TwitchLogin:  login,
		Role:         role,
		ExpiresAt:    time.Now().Add(s.ttl).Unix(),
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	signature := s.sign(rawPayload)
	return base64.RawURLEncoding.EncodeToString(rawPayload) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (s *Signer) Parse(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return Claims{}, errors.New("invalid token format")
	}

	rawPayload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, errors.New("invalid token payload")
	}

	rawSignature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, errors.New("invalid token signature")
	}

	expected := s.sign(rawPayload)
	if !hmac.Equal(expected, rawSignature) {
		return Claims{}, errors.New("invalid token signature")
	}

	var claims Claims
	if err := json.Unmarshal(rawPayload, &claims); err != nil {
		return Claims{}, errors.New("invalid token json")
	}

	if time.Now().Unix() > claims.ExpiresAt {
		return Claims{}, errors.New("session expired")
	}

	return claims, nil
}

func ReadToken(r *http.Request, cookieName string) string {
	if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	}

	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (s *Signer) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}
