package gitlab

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/just-tomdo/housenka/internal/atomicfile"
)

// TokenEnv overrides the token file.
const TokenEnv = "HOUSENKA_GITLAB_TOKEN"

// TokenPath returns the token file in the housenka home. The token never
// goes into a configuration file, which the user may share or show.
func TokenPath(home string) string {
	return filepath.Join(home, "gitlab-token")
}

// ErrNoToken means no token is set.
var ErrNoToken = errors.New("chybí token GitLabu – nastav ho příkazem `housenka issues token`, v Nastavení webového rozhraní, nebo proměnnou " + TokenEnv)

// Token returns the token from TokenEnv, or from the token file.
func Token(home string) (string, error) {
	if t := strings.TrimSpace(os.Getenv(TokenEnv)); t != "" {
		return t, nil
	}
	data, err := os.ReadFile(TokenPath(home))
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNoToken
	}
	if err != nil {
		return "", err
	}
	if t := strings.TrimSpace(string(data)); t != "" {
		return t, nil
	}
	return "", ErrNoToken
}

// TokenFromEnv reports whether TokenEnv is set (a saved file is then unused).
func TokenFromEnv() bool {
	return strings.TrimSpace(os.Getenv(TokenEnv)) != ""
}

// SaveToken writes the token file readable only by the user; an empty
// token removes it.
func SaveToken(home, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		err := os.Remove(TokenPath(home))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if strings.ContainsAny(token, " \t\r\n") {
		return errors.New("token nesmí obsahovat mezery ani konce řádků")
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return err
	}
	if err := atomicfile.Write(TokenPath(home), []byte(token+"\n"), 0o600); err != nil {
		return err
	}
	// Write keeps the mode of an existing file; a token must not stay readable for others.
	return os.Chmod(TokenPath(home), 0o600)
}
