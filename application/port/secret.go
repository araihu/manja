package port

import (
	"github.com/araihu/manja/domain"
)

// SecretRef is an opaque lookup reference. It never contains the token,
// password, private key, or other secret material itself.
type SecretRef struct {
	Name string `json:"name" yaml:"name"`
}

func (r SecretRef) Validate() error {
	return domain.ValidateCanonicalIdentity("secret reference name", r.Name, false)
}
