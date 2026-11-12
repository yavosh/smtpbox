package http

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
)

type basicAuth struct {
	usernameHash [sha256.Size]byte
	passwordHash [sha256.Size]byte
}

func WithBasicAuth(username, password string) Option {
	return func(s *Server) {
		s.auth = nil
		s.authErr = nil
		if username == "" && password == "" {
			return
		}
		if username == "" || password == "" {
			s.authErr = errors.New("HTTP username and password must both be set")
			return
		}
		if strings.Contains(username, ":") {
			s.authErr = errors.New("HTTP username must not contain a colon")
			return
		}
		s.auth = &basicAuth{
			usernameHash: sha256.Sum256([]byte(username)),
			passwordHash: sha256.Sum256([]byte(password)),
		}
	}
}

func basicAuthMiddleware(auth *basicAuth, next http.Handler) http.Handler {
	if auth == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		usernameHash := sha256.Sum256([]byte(username))
		passwordHash := sha256.Sum256([]byte(password))
		valid := subtle.ConstantTimeCompare(usernameHash[:], auth.usernameHash[:]) & subtle.ConstantTimeCompare(passwordHash[:], auth.passwordHash[:])
		if !ok || valid != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="smtpbox"`)
			jsonResponse(w, http.StatusUnauthorized, apiError{Error: "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
