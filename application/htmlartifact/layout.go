package htmlartifact

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"path"
	"strings"

	"github.com/araihu/manja/domain"
)

const FragmentFormatV2 = "manja-html-fragment-v2"

// FragmentLocation returns the publication-relative HTML and adjacent
// sidecar paths for one portable resource fragment. Raw OpenAPI paths and
// display names never participate in filesystem layout.
func FragmentLocation(catalogMount, specKey string, identity FragmentIdentity) (string, string, error) {
	if err := domain.ValidateCanonicalPublicPath("HTML fragment catalog mount", catalogMount, false); err != nil {
		return "", "", err
	}
	if catalogMount != "/" && strings.HasSuffix(catalogMount, "/") {
		return "", "", fmt.Errorf("htmlartifact: catalog mount must not end in a slash")
	}
	if err := domain.ValidateCatalogDocumentKey(specKey); err != nil {
		return "", "", err
	}
	if err := identity.Validate(); err != nil {
		return "", "", err
	}
	if identity.Format != FragmentFormatV2 {
		return "", "", fmt.Errorf("htmlartifact: fragment format %q is unsupported", identity.Format)
	}
	directory, ok := fragmentDirectory(identity.Kind)
	if !ok {
		return "", "", fmt.Errorf("htmlartifact: fragment kind %q has no resource directory", identity.Kind)
	}
	resourceKey := fragmentResourceKey(identity)
	parts := make([]string, 0, 7)
	if catalogMount != "/" {
		parts = append(parts, strings.TrimPrefix(catalogMount, "/"))
	}
	parts = append(parts, "documents", specKey, "_manja", "fragments", directory, resourceKey+".html")
	htmlPath := path.Join(parts...)
	return htmlPath, htmlPath + ".meta.json", nil
}

func fragmentDirectory(kind FragmentKind) (string, bool) {
	switch kind {
	case FragmentOperation:
		return "operations", true
	case FragmentSchema:
		return "schemas", true
	case FragmentExample:
		return "examples", true
	case FragmentPath:
		return "paths", true
	case FragmentSidebar:
		return "sidebar/operations", true
	default:
		return "", false
	}
}

func fragmentResourceKey(identity FragmentIdentity) string {
	preimage := make([]byte, 0, len(identity.Format)+len(identity.Kind)+len(identity.Resource)+32)
	for _, field := range []string{"manja.html.fragment.resource.v2", identity.Format, string(identity.Kind), identity.Resource} {
		preimage = binary.BigEndian.AppendUint32(preimage, uint32(len(field)))
		preimage = append(preimage, field...)
	}
	digest := sha256.Sum256(preimage)
	return string(identity.Kind) + "-sha256-" + hex.EncodeToString(digest[:])
}
