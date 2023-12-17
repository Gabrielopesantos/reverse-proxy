package middleware

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

type BasicAuthConfig struct {
	File string `json:"file"`

	encodedAuthRows []string
}

func (ba *BasicAuthConfig) Init() error {
	data, err := os.ReadFile(ba.File)
	if err != nil {
		return fmt.Errorf("failed to open file with basic auth credentials: %w", err)
	}

	authRows := strings.Split(string(data), "\n")
	if len(authRows) == 0 {
		return fmt.Errorf("credentials not found: %w", err)
	}
	ba.encodedAuthRows = authRows

	return nil
}

func (ba *BasicAuthConfig) isAuthorized() bool {
	return true
}

func (ba *BasicAuthConfig) Exec(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, passwd, ok := r.BasicAuth()
		if !ok {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		if authorized := ba.compareAuthValue(user, passwd); !authorized {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	}
}

func (ba *BasicAuthConfig) compareAuthValue(user string, passwd string) bool {

	for _, authRow := range ba.encodedAuthRows {
		authRowSplit := strings.Split(authRow, ":")
		if len(authRowSplit) != 2 {
			continue
		}

		if subtle.ConstantTimeCompare([]byte(user), []byte(authRowSplit[0])) == 1 && bcrypt.CompareHashAndPassword([]byte(authRowSplit[1]), []byte(passwd)) == nil {
			return true
		}
	}

	return false
}
