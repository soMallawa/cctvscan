// Package secrets provides AES-256-GCM envelopes with caller-owned keys.
// Keys must come from deployment configuration, never the database. Scope is
// authenticated (not encrypted): use stable record identity, e.g. proxy/<id>.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
)

var ErrInvalidKey = errors.New("secret encryption requires a 32-byte key")
var ErrInvalidCiphertext = errors.New("secret authentication failed")

type Box struct{ aead cipher.AEAD }

func New(key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalidKey
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrInvalidKey
	}
	return &Box{aead: aead}, nil
}

// Seal produces version || random nonce || authenticated ciphertext.
func (b *Box) Seal(scope string, plain []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, errors.New("secret encryption unavailable")
	}
	out := append([]byte{1}, nonce...)
	return b.aead.Seal(out, nonce, plain, []byte(scope)), nil
}

func (b *Box) Open(scope string, sealed []byte) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(sealed) < 1+n+b.aead.Overhead() || sealed[0] != 1 {
		return nil, ErrInvalidCiphertext
	}
	plain, err := b.aead.Open(nil, sealed[1:1+n], sealed[1+n:], []byte(scope))
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	return plain, nil
}
