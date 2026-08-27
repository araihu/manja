package localdocs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/araihu/manja/application/catalog"
)

func marshalIdentity(identity catalog.SnapshotIdentityV1) ([]byte, error) {
	return json.Marshal(identity)
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func sameChildIdentities(left, right []catalog.ChildIdentityV1) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func descriptorPublicationBase(value string) (string, bool) {
	if value != "/" && !strings.HasSuffix(value, "/") {
		value += "/"
	}
	if !validPublicationBase(value) {
		return "", false
	}
	if value == "/" {
		return value, true
	}
	return strings.TrimSuffix(value, "/") + "/", true
}

func descriptorURL(base string, segments ...string) string {
	return strings.TrimSuffix(base, "/") + "/" + strings.Join(segments, "/")
}
