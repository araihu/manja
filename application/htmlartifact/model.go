// Package htmlartifact builds canonical pre-render cache keys for incremental
// HTML artifacts. Filesystem policy remains behind application ports.
package htmlartifact

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/araihu/manja/application/port"
)

const (
	ManifestSchemaVersion          = port.HTMLArtifactManifestSchemaVersion
	BuildKeySchemaVersion   uint32 = 1
	DefaultSidebarChunkSize uint32 = 12
)

type FragmentKind = port.HTMLFragmentKind

const (
	FragmentOperation = port.HTMLFragmentOperation
	FragmentSchema    = port.HTMLFragmentSchema
	FragmentExample   = port.HTMLFragmentExample
	FragmentPath      = port.HTMLFragmentPath
	FragmentSidebar   = port.HTMLFragmentSidebar
)

type FragmentIdentity = port.HTMLFragmentIdentity
type BuildKey = port.HTMLArtifactBuildKey
type ContentIdentity = port.HTMLArtifactContentIdentity
type Manifest = port.HTMLArtifactManifest
type Expectation = port.HTMLArtifactExpectation
type VerificationStatus = port.HTMLArtifactVerificationStatus
type Verification = port.HTMLArtifactVerification
type Render = port.HTMLArtifactRender

const (
	VerificationHit              = port.HTMLArtifactVerificationHit
	VerificationManifestMissing  = port.HTMLArtifactVerificationManifestMissing
	VerificationManifestInvalid  = port.HTMLArtifactVerificationManifestInvalid
	VerificationIdentityMismatch = port.HTMLArtifactVerificationIdentityMismatch
	VerificationBuildKeyMismatch = port.HTMLArtifactVerificationBuildKeyMismatch
	VerificationArtifactMissing  = port.HTMLArtifactVerificationArtifactMissing
	VerificationArtifactCorrupt  = port.HTMLArtifactVerificationArtifactCorrupt
)

// BuildProfile contains output-affecting build options. A zero chunk size is
// resolved to the public default before it participates in a sidebar key.
type BuildProfile struct {
	SidebarChunkSize uint32 `json:"sidebarChunkSize"`
}

func (profile BuildProfile) Resolved() BuildProfile {
	if profile.SidebarChunkSize == 0 {
		profile.SidebarChunkSize = DefaultSidebarChunkSize
	}
	return profile
}

func (profile BuildProfile) forFragment(kind FragmentKind) BuildProfile {
	// Sidebar pagination cannot affect the bytes of any other fragment kind.
	if kind != FragmentSidebar {
		return BuildProfile{}
	}
	return profile.Resolved()
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func boundedText(name, value string) error {
	if value == "" || len(value) > 4096 || !utf8.ValidString(value) {
		return fmt.Errorf("htmlartifact: %s is invalid", name)
	}
	return nil
}
