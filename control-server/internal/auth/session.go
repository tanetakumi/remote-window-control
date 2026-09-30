package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var ErrUnauthorized = errors.New("unauthorized")

type Session struct {
	Token     string    `json:"access_token"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	secret   string
	sessions map[string]Session
	mu       sync.RWMutex
}

func NewStore(secret string) *Store {
	return &Store{
		secret:   secret,
		sessions: make(map[string]Session),
	}
}

func (s *Store) SecretLink(host string) string {
	return host + "/?secret=" + s.secret
}

func (s *Store) Exchange(secret string) (Session, error) {
	if subtle.ConstantTimeCompare([]byte(secret), []byte(s.secret)) != 1 {
		return Session{}, ErrUnauthorized
	}

	session := Session{
		Token:     randomToken(24),
		CreatedAt: time.Now().UTC(),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for key, old := range s.sessions {
		if time.Since(old.CreatedAt) > 12*time.Hour {
			delete(s.sessions, key)
		}
	}
	if len(s.sessions) >= 1024 {
		return Session{}, errors.New("session limit reached")
	}
	s.sessions[session.Token] = session
	return session, nil
}

func (s *Store) Validate(token string) (Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[token]
	if !ok || time.Since(session.CreatedAt) > 12*time.Hour {
		return Session{}, ErrUnauthorized
	}

	return session, nil
}

func randomToken(bytes int) string {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		panic("could not generate a session token: " + err.Error())
	}

	return hex.EncodeToString(buffer)
}
