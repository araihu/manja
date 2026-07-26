package port

import "testing"

func TestSecretRefRejectsNonCanonicalIdentity(t *testing.T) {
	for _, name := range []string{" secret-main ", "secret\x00shadow", "secret-\xff"} {
		t.Run(name, func(t *testing.T) {
			if err := (SecretRef{Name: name}).Validate(); err == nil {
				t.Fatal("SecretRef accepted non-canonical identity")
			}
		})
	}
}
