package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/araihu/manja/internal/core"
)

var errUnsafeStorePath = errors.New("unsafe store path")

type FileStore struct {
	root string
}

func NewFileStore(root string) *FileStore {
	return &FileStore{root: root}
}

func (s *FileStore) SaveProject(ctx context.Context, p core.Project) error {
	if err := validateID(p.ID); err != nil {
		return err
	}
	return s.writeJSON(ctx, "projects", p.ID+".json", p)
}

func (s *FileStore) Project(ctx context.Context, id string) (core.Project, error) {
	var p core.Project
	if err := validateID(id); err != nil {
		return p, err
	}
	err := s.readJSON(ctx, "projects", id+".json", &p)
	return p, err
}

func (s *FileStore) SaveRevision(ctx context.Context, r core.Revision) error {
	if err := validateID(r.ID); err != nil {
		return err
	}
	return s.writeJSON(ctx, "revisions", r.ID+".json", r)
}

func (s *FileStore) Revision(ctx context.Context, id string) (core.Revision, error) {
	var r core.Revision
	if err := validateID(id); err != nil {
		return r, err
	}
	err := s.readJSON(ctx, "revisions", id+".json", &r)
	return r, err
}

func (s *FileStore) SavePublication(ctx context.Context, p core.Publication) error {
	if err := validateID(p.ProjectID); err != nil {
		return err
	}
	if err := validateID(p.RevisionID); err != nil {
		return err
	}
	name := p.ProjectID + "-" + p.RevisionID + ".json"
	return s.writeJSON(ctx, "publications", name, p)
}

func (s *FileStore) Put(ctx context.Context, key string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.safeNamespacePath("blobs", key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (s *FileStore) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := s.safeNamespacePath("blobs", key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (s *FileStore) writeJSON(ctx context.Context, namespace, name string, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.safeNamespacePath(namespace, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func (s *FileStore) readJSON(ctx context.Context, namespace, name string, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.safeNamespacePath(namespace, name)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func (s *FileStore) safeNamespacePath(namespace, name string) (string, error) {
	clean := filepath.Clean(name)
	if clean != name || clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.Contains(name, `\`) {
		return "", errUnsafeStorePath
	}

	root := filepath.Join(s.root, namespace)
	path := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errUnsafeStorePath
	}
	return path, nil
}

func validateID(id string) error {
	if id == "" || id == "." || id == ".." || filepath.IsAbs(id) {
		return errUnsafeStorePath
	}
	if strings.ContainsAny(id, `/\`) || filepath.Clean(id) != id {
		return errUnsafeStorePath
	}
	return nil
}
