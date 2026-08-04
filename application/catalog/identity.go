package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
)

func snapshotIdentity(identity SnapshotIdentityV1) (SnapshotID, []byte, error) {
	data, err := json.Marshal(identity)
	if err != nil {
		return "", nil, fmt.Errorf("encode snapshot identity: %w", err)
	}
	digest := sha256.Sum256(data)
	return SnapshotID("snapshot-sha256-" + hex.EncodeToString(digest[:])), data, nil
}

func newChild(pathValue, kind string, data []byte) (ChildArtifact, error) {
	if pathValue == "" || strings.HasPrefix(pathValue, "/") || strings.Contains(pathValue, `\`) || path.Clean(pathValue) != pathValue || strings.HasPrefix(pathValue, "../") {
		return ChildArtifact{}, fmt.Errorf("child path %q is invalid", pathValue)
	}
	if kind == "" || len(data) == 0 {
		return ChildArtifact{}, fmt.Errorf("child %q kind and bytes are required", pathValue)
	}
	digest := sha256.Sum256(data)
	return ChildArtifact{
		Path: pathValue, Kind: kind, Length: uint64(len(data)), SHA256: hex.EncodeToString(digest[:]), Bytes: append([]byte(nil), data...),
	}, nil
}

func childIdentities(children []ChildArtifact) ([]ChildIdentityV1, error) {
	children = append([]ChildArtifact(nil), children...)
	sort.Slice(children, func(i, j int) bool { return children[i].Path < children[j].Path })
	result := make([]ChildIdentityV1, len(children))
	for index, child := range children {
		if index > 0 && children[index-1].Path == child.Path {
			return nil, fmt.Errorf("child path %q is duplicated", child.Path)
		}
		result[index] = ChildIdentityV1{Path: child.Path, Kind: child.Kind, Length: child.Length, SHA256: child.SHA256}
	}
	return result, nil
}
