package token

import (
	"crypto/rand"
	"encoding/base64"
)

type SecureGenerator struct{}

func NewSecureGenerator() *SecureGenerator {
	return &SecureGenerator{}
}

func (g *SecureGenerator) NewToken(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
