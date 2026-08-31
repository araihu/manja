package htmlartifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// BuildKeyInput is the complete pre-render dependency surface for one HTML
// fragment. All members must be canonical identities or SHA-256 digests; none
// may depend on map iteration, worker completion order, timestamps, or paths on
// the build host.
type BuildKeyInput struct {
	Fragment               FragmentIdentity
	CanonicalPayloadSHA256 string
	ManjaVersion           string
	RendererFingerprint    string
	UIFingerprint          string
	CompilerIdentity       string
	NormalizerIdentity     string
	Profile                BuildProfile
}

type canonicalBuildKeyV1 struct {
	SchemaVersion          uint32           `json:"schemaVersion"`
	Fragment               FragmentIdentity `json:"fragment"`
	CanonicalPayloadSHA256 string           `json:"canonicalPayloadSHA256"`
	ManjaVersion           string           `json:"manjaVersion"`
	RendererFingerprint    string           `json:"rendererFingerprint"`
	UIFingerprint          string           `json:"uiFingerprint"`
	CompilerIdentity       string           `json:"compilerIdentity"`
	NormalizerIdentity     string           `json:"normalizerIdentity"`
	Profile                BuildProfile     `json:"profile"`
}

func NewBuildKey(input BuildKeyInput) (BuildKey, error) {
	if err := input.Fragment.Validate(); err != nil {
		return "", err
	}
	if !validSHA256(input.CanonicalPayloadSHA256) {
		return "", fmt.Errorf("htmlartifact: canonical payload SHA-256 is invalid")
	}
	for _, value := range []struct {
		name string
		text string
	}{
		{"Manja version", input.ManjaVersion},
		{"renderer fingerprint", input.RendererFingerprint},
		{"UI fingerprint", input.UIFingerprint},
		{"compiler identity", input.CompilerIdentity},
		{"normalizer identity", input.NormalizerIdentity},
	} {
		if err := boundedText(value.name, value.text); err != nil {
			return "", err
		}
	}
	canonical := canonicalBuildKeyV1{
		SchemaVersion:          BuildKeySchemaVersion,
		Fragment:               input.Fragment,
		CanonicalPayloadSHA256: input.CanonicalPayloadSHA256,
		ManjaVersion:           input.ManjaVersion,
		RendererFingerprint:    input.RendererFingerprint,
		UIFingerprint:          input.UIFingerprint,
		CompilerIdentity:       input.CompilerIdentity,
		NormalizerIdentity:     input.NormalizerIdentity,
		Profile:                input.Profile.forFragment(input.Fragment.Kind),
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("htmlartifact: encode canonical build key: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return BuildKey("fragment-build-sha256:" + hex.EncodeToString(digest[:])), nil
}

func PayloadSHA256(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}
