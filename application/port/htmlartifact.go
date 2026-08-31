package port

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const HTMLArtifactManifestSchemaVersion uint32 = 1

type HTMLFragmentKind string

const (
	HTMLFragmentOperation HTMLFragmentKind = "operation"
	HTMLFragmentSchema    HTMLFragmentKind = "schema"
	HTMLFragmentExample   HTMLFragmentKind = "example"
	HTMLFragmentPath      HTMLFragmentKind = "path"
	HTMLFragmentSidebar   HTMLFragmentKind = "sidebar"
)

func (kind HTMLFragmentKind) Valid() bool {
	switch kind {
	case HTMLFragmentOperation, HTMLFragmentSchema, HTMLFragmentExample, HTMLFragmentPath, HTMLFragmentSidebar:
		return true
	default:
		return false
	}
}

// HTMLFragmentIdentity identifies one versioned logical rendering unit.
// Resource is the caller's canonical identity, not a display name or path.
type HTMLFragmentIdentity struct {
	Format   string           `json:"format"`
	Kind     HTMLFragmentKind `json:"kind"`
	Resource string           `json:"resource"`
}

func (identity HTMLFragmentIdentity) Validate() error {
	if err := validateHTMLArtifactText("fragment format", identity.Format); err != nil {
		return err
	}
	if !identity.Kind.Valid() {
		return fmt.Errorf("htmlartifact: fragment kind %q is invalid", identity.Kind)
	}
	return validateHTMLArtifactText("resource identity", identity.Resource)
}

type HTMLArtifactBuildKey string

func (key HTMLArtifactBuildKey) Valid() bool {
	const prefix = "fragment-build-sha256:"
	value, ok := strings.CutPrefix(string(key), prefix)
	return ok && validHTMLArtifactSHA256(value)
}

type HTMLArtifactContentIdentity struct {
	Length uint64 `json:"length"`
	SHA256 string `json:"sha256"`
}

func (identity HTMLArtifactContentIdentity) Validate() error {
	if !validHTMLArtifactSHA256(identity.SHA256) {
		return fmt.Errorf("htmlartifact: content SHA-256 is invalid")
	}
	return nil
}

// HTMLArtifactManifest is an adjacent, versioned sidecar. A schema reader is
// strict about its member set; evolution requires a new schema version and a
// reader that explicitly understands it.
type HTMLArtifactManifest struct {
	SchemaVersion uint32                      `json:"schemaVersion"`
	Fragment      HTMLFragmentIdentity        `json:"fragment"`
	BuildKey      HTMLArtifactBuildKey        `json:"buildKey"`
	Content       HTMLArtifactContentIdentity `json:"content"`
}

func (manifest HTMLArtifactManifest) Validate() error {
	if manifest.SchemaVersion != HTMLArtifactManifestSchemaVersion {
		return fmt.Errorf("htmlartifact: manifest schema version %d is unsupported", manifest.SchemaVersion)
	}
	if err := manifest.Fragment.Validate(); err != nil {
		return err
	}
	if !manifest.BuildKey.Valid() {
		return fmt.Errorf("htmlartifact: build key is invalid")
	}
	return manifest.Content.Validate()
}

type HTMLArtifactExpectation struct {
	Fragment HTMLFragmentIdentity
	BuildKey HTMLArtifactBuildKey
}

func (expectation HTMLArtifactExpectation) Validate() error {
	if err := expectation.Fragment.Validate(); err != nil {
		return err
	}
	if !expectation.BuildKey.Valid() {
		return fmt.Errorf("htmlartifact: build key is invalid")
	}
	return nil
}

type HTMLArtifactVerificationStatus string

const (
	HTMLArtifactVerificationHit              HTMLArtifactVerificationStatus = "hit"
	HTMLArtifactVerificationManifestMissing  HTMLArtifactVerificationStatus = "manifest-missing"
	HTMLArtifactVerificationManifestInvalid  HTMLArtifactVerificationStatus = "manifest-invalid"
	HTMLArtifactVerificationIdentityMismatch HTMLArtifactVerificationStatus = "identity-mismatch"
	HTMLArtifactVerificationBuildKeyMismatch HTMLArtifactVerificationStatus = "build-key-mismatch"
	HTMLArtifactVerificationArtifactMissing  HTMLArtifactVerificationStatus = "artifact-missing"
	HTMLArtifactVerificationArtifactCorrupt  HTMLArtifactVerificationStatus = "artifact-corrupt"
)

type HTMLArtifactVerification struct {
	Status   HTMLArtifactVerificationStatus
	Manifest HTMLArtifactManifest
}

func (verification HTMLArtifactVerification) Hit() bool {
	return verification.Status == HTMLArtifactVerificationHit
}

// HTMLArtifactStore verifies and atomically commits generated fragments. The
// path is a publication-relative slash path; concrete filesystem layout and
// sidecar naming remain adapter details.
type HTMLArtifactStore interface {
	VerifyHTML(context.Context, string, HTMLArtifactExpectation) (HTMLArtifactVerification, error)
	CommitHTML(context.Context, string, HTMLArtifactExpectation, HTMLArtifactRender) (HTMLArtifactManifest, error)
}

// HTMLArtifactRender streams a fragment into store-owned staging storage. A
// successful return commits exactly the bytes written; an error publishes
// neither a partial artifact nor its sidecar.
type HTMLArtifactRender func(io.Writer) error

func validHTMLArtifactSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validateHTMLArtifactText(name, value string) error {
	if value == "" || len(value) > 4096 || !utf8.ValidString(value) {
		return fmt.Errorf("htmlartifact: %s is invalid", name)
	}
	return nil
}
