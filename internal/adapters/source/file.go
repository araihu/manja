package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"

	"github.com/araihu/manja/internal/core"
)

type File struct {
	Path string
}

func (f File) Fetch(ctx context.Context) (core.SpecFile, core.Revision, error) {
	if err := ctx.Err(); err != nil {
		return core.SpecFile{}, core.Revision{}, err
	}
	data, err := os.ReadFile(f.Path)
	if err != nil {
		return core.SpecFile{}, core.Revision{}, err
	}
	sum := sha256.Sum256(data)
	id := "file-" + hex.EncodeToString(sum[:])[:16]
	return core.SpecFile{Path: f.Path, Format: "yaml", Bytes: data}, core.Revision{ID: id, Ref: "file"}, nil
}
