package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
)

// Cookie names. Both cookies are encrypted; the browser can't read them.
const (
	// SessionCookie holds the encrypted Session. It has no Max-Age, so it
	// ends with the browser session (ADR-009).
	SessionCookie = "setlisted_session"
	// loginCookie holds the state and PKCE verifier during the round trip
	// to Spotify. It is scoped to /auth and deleted on return.
	loginCookie = "setlisted_login"
)

// errInvalidCookie covers every reason a cookie can't be used: bad encoding,
// wrong key, tampering, or content that doesn't parse.
var errInvalidCookie = errors.New("invalid cookie")

// sealer encrypts and authenticates cookie values with AES-GCM.
type sealer struct {
	aead cipher.AEAD
}

func newSealer(key []byte) (*sealer, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &sealer{aead: aead}, nil
}

// seal JSON-encodes v and encrypts it with a random nonce. The cookie name is
// authenticated as additional data, so one cookie's value can't be replayed
// as the other's. The result is base64url (no padding): nonce || ciphertext.
func (s *sealer) seal(name string, v any) (string, error) {
	plaintext, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, s.aead.NonceSize(), s.aead.NonceSize()+len(plaintext)+s.aead.Overhead())
	rand.Read(nonce) // never returns an error (crypto/rand, Go 1.24+)
	return base64.RawURLEncoding.EncodeToString(s.aead.Seal(nonce, nonce, plaintext, []byte(name))), nil
}

// open reverses seal into v. Any failure is errInvalidCookie.
func (s *sealer) open(name, value string, v any) error {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(data) < s.aead.NonceSize() {
		return errInvalidCookie
	}
	nonce, ciphertext := data[:s.aead.NonceSize()], data[s.aead.NonceSize():]
	plaintext, err := s.aead.Open(nil, nonce, ciphertext, []byte(name))
	if err != nil {
		return errInvalidCookie
	}
	if err := json.Unmarshal(plaintext, v); err != nil {
		return errInvalidCookie
	}
	return nil
}

// newCookie returns an HttpOnly, Secure, SameSite=Lax cookie. maxAge 0 means
// no Max-Age (a browser-session cookie).
func newCookie(name, path, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}

func setCookie(w http.ResponseWriter, name, path, value string, maxAge int) {
	http.SetCookie(w, newCookie(name, path, value, maxAge))
}

// clearCookie tells the browser to delete a cookie (Max-Age=0).
func clearCookie(w http.ResponseWriter, name, path string) {
	setCookie(w, name, path, "", -1) // net/http writes MaxAge < 0 as "Max-Age=0"
}
