package port

import (
	"context"
	"fmt"
	"strings"
)

// SecretRef is an opaque lookup reference. It never contains the token,
// password, private key, or other secret material itself.
type SecretRef struct {
	Name string `json:"name" yaml:"name"`
}

func (r SecretRef) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("secret reference name is required")
	}
	if r.Name != strings.TrimSpace(r.Name) {
		return fmt.Errorf("secret reference name must be normalized")
	}
	return nil
}

type SecretResolver interface {
	Resolve(context.Context, SecretRef) ([]byte, error)
}
