package framework

import "crypto/rand"

const (
	idAlphabet = "abcdefghijklmnopqrstuvwxyz"
	idLength   = 28
)

// GenerateID returns a 28-char lowercase alpha ID.
func GenerateID() string {
	b := make([]byte, idLength)
	_, _ = rand.Read(b)
	for i := 0; i < idLength; i++ {
		b[i] = idAlphabet[int(b[i])%len(idAlphabet)]
	}
	return string(b)
}
