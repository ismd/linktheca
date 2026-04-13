package auth

import (
	"github.com/alexedwards/argon2id"
)

// argonParams matches OWASP 2024 recommendations for argon2id
var argonParams = &argon2id.Params{
	Memory:      64 * 1024, // 64 MB
	Iterations:  2,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

func HashPassword(plaintext string) (string, error) {
	return argon2id.CreateHash(plaintext, argonParams)
}

// VerifyPassword reports whether the plaintext matches the stored hash
// Returns (false, nil) for a valid hash that does not match, and a non-nil error only if the hash itself is malformed
func VerifyPassword(plaintext, hash string) (bool, error) {
	return argon2id.ComparePasswordAndHash(plaintext, hash)
}
