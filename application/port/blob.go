package port

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

type BlobKey string

func ContentAddressedBlobKey(data []byte) BlobKey {
	sum := sha256.Sum256(data)
	return BlobKey("sha256:" + hex.EncodeToString(sum[:]))
}

func (k BlobKey) Valid() bool {
	value := string(k)
	digest, ok := strings.CutPrefix(value, "sha256:")
	if !ok || len(digest) != sha256.Size*2 || digest != strings.ToLower(digest) {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

// BlobStore stores immutable bytes by content identity. Put must return exactly
// ContentAddressedBlobKey(data), be idempotent, and may reuse the existing
// write for identical bytes.
//
// Application services write a blob before entering UnitOfWork, then commit the
// referencing metadata in the transaction. A failed transaction may leave an
// unreachable blob; that blob is replay-safe and may be garbage-collected only
// after proving no committed operational record references it. A successful
// operational commit must never point at a missing blob.
type BlobStore interface {
	Put(context.Context, []byte) (BlobKey, error)
	Get(context.Context, BlobKey) ([]byte, error)
}
