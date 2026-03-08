package middleware

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const BASIC_AUTH_ROW_DELIMITER = "\n"

type BasicAuthConfig struct {
	File string `yaml:"file"`

	encodedAuthRows []string
	logger          *slog.Logger
}

func (ba *BasicAuthConfig) Init(ctx context.Context) error {
	ba.logger = LoggerFromContext(ctx)
	data, err := os.ReadFile(ba.File)
	if err != nil {
		return fmt.Errorf("failed to open file with basic auth credentials: %w", err)
	}

	ba.encodedAuthRows = strings.Split(string(data), BASIC_AUTH_ROW_DELIMITER)

	return nil
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

		rowUser := authRowSplit[0]
		rowPasswdHash := authRowSplit[1]
		if subtle.ConstantTimeCompare([]byte(user), []byte(rowUser)) == 1 && bcrypt.CompareHashAndPassword([]byte(rowPasswdHash), []byte(passwd)) == nil {
			return true
		}
	}

	return false
}
