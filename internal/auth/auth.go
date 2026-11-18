// Package auth supplies the token lmao sends to the Lunch Money API.
package auth

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// TokenSource gives the bearer token for an API request. The client asks for
// a token on every request, so a source can refresh tokens that expire, as
// one using OAuth would.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// TokenEnv is the environment variable EnvSource reads by default.
const TokenEnv = "LUNCHMONEY_TOKEN"

// TokenURL is where Lunch Money users create access tokens.
const TokenURL = "https://my.lunchmoney.app/developers"

// EnvSource reads a long-lived access token from an environment variable.
type EnvSource struct {
	Var string // defaults to TokenEnv
}

func (s EnvSource) name() string {
	if s.Var == "" {
		return TokenEnv
	}
	return s.Var
}

// Token returns the variable's value, or an error saying how to set it.
func (s EnvSource) Token(context.Context) (string, error) {
	if t := strings.TrimSpace(os.Getenv(s.name())); t != "" {
		return t, nil
	}
	return "", fmt.Errorf("%s isn't set: create an access token at %s and put it in %s", s.name(), TokenURL, s.name())
}

// Check returns the same error as Token, without a context, so a missing
// token can be reported before anything else starts.
func (s EnvSource) Check() error {
	_, err := s.Token(context.Background())
	return err
}
