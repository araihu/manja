package htmlartifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	artifact "github.com/araihu/manja/application/htmlartifact"
	"github.com/araihu/manja/application/port"
	storeprimitives "github.com/araihu/manja/internal/adapters/store"
)

const (
	sidecarSuffix       = ".meta.json"
	maximumManifestSize = 64 << 10
)

var (
	errNonRegularFile   = errors.New("htmlartifact: candidate is not a regular file")
	errManifestTooLarge = errors.New("htmlartifact: sidecar exceeds maximum size")
)

type durableWriter func(string, fs.FileMode, func(io.Writer) error) error

type Store struct {
	root    string
	rootErr error
	write   durableWriter
}

func New(root string) *Store {
	absolute, err := filepath.Abs(root)
	if root == "" {
		err = fmt.Errorf("htmlartifact: root is empty")
	}
	return &Store{root: filepath.Clean(absolute), rootErr: err, write: storeprimitives.DurableAtomicWriteFunc}
}

func (store *Store) VerifyHTML(ctx context.Context, artifactPath string, expected artifact.Expectation) (artifact.Verification, error) {
	if err := ctx.Err(); err != nil {
		return artifact.Verification{}, err
	}
	if err := expected.Validate(); err != nil {
		return artifact.Verification{}, err
	}
	resolved, err := store.resolve(artifactPath)
	if err != nil {
		return artifact.Verification{}, err
	}
	manifestBytes, err := readBoundedRegular(resolved+sidecarSuffix, maximumManifestSize)
	if errors.Is(err, os.ErrNotExist) {
		return artifact.Verification{Status: artifact.VerificationManifestMissing}, nil
	}
	if errors.Is(err, errNonRegularFile) || errors.Is(err, errManifestTooLarge) {
		return artifact.Verification{Status: artifact.VerificationManifestInvalid}, nil
	}
	if err != nil {
		return artifact.Verification{}, fmt.Errorf("htmlartifact: read sidecar: %w", err)
	}
	manifest, err := decodeManifest(manifestBytes)
	if err != nil || manifest.Validate() != nil {
		return artifact.Verification{Status: artifact.VerificationManifestInvalid}, nil
	}
	result := artifact.Verification{Manifest: manifest}
	if manifest.Fragment != expected.Fragment {
		result.Status = artifact.VerificationIdentityMismatch
		return result, nil
	}
	if manifest.BuildKey != expected.BuildKey {
		result.Status = artifact.VerificationBuildKeyMismatch
		return result, nil
	}
	file, err := openRegular(resolved)
	if errors.Is(err, os.ErrNotExist) {
		result.Status = artifact.VerificationArtifactMissing
		return result, nil
	}
	if errors.Is(err, errNonRegularFile) {
		result.Status = artifact.VerificationArtifactCorrupt
		return result, nil
	}
	if err != nil {
		return artifact.Verification{}, fmt.Errorf("htmlartifact: open HTML: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	written, err := io.Copy(hash, contextReader{ctx: ctx, reader: file})
	if err != nil {
		return artifact.Verification{}, fmt.Errorf("htmlartifact: verify HTML: %w", err)
	}
	if uint64(written) != manifest.Content.Length || hex.EncodeToString(hash.Sum(nil)) != manifest.Content.SHA256 {
		result.Status = artifact.VerificationArtifactCorrupt
		return result, nil
	}
	result.Status = artifact.VerificationHit
	return result, nil
}

func (store *Store) CommitHTML(ctx context.Context, artifactPath string, expected artifact.Expectation, render artifact.Render) (artifact.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return artifact.Manifest{}, err
	}
	if render == nil {
		return artifact.Manifest{}, fmt.Errorf("htmlartifact: render callback is nil")
	}
	if err := expected.Validate(); err != nil {
		return artifact.Manifest{}, err
	}
	resolved, err := store.resolve(artifactPath)
	if err != nil {
		return artifact.Manifest{}, err
	}
	if err := validateCommitCandidate(resolved); err != nil {
		return artifact.Manifest{}, err
	}
	if err := validateCommitCandidate(resolved + sidecarSuffix); err != nil {
		return artifact.Manifest{}, err
	}
	hash := sha256.New()
	var length uint64
	if err := store.write(resolved, 0o644, func(staging io.Writer) error {
		writer := &countingWriter{writer: io.MultiWriter(staging, hash), count: &length}
		if err := render(contextWriter{ctx: ctx, writer: writer}); err != nil {
			return err
		}
		return ctx.Err()
	}); err != nil {
		return artifact.Manifest{}, fmt.Errorf("htmlartifact: commit HTML: %w", err)
	}
	manifest := artifact.Manifest{
		SchemaVersion: artifact.ManifestSchemaVersion,
		Fragment:      expected.Fragment,
		BuildKey:      expected.BuildKey,
		Content: artifact.ContentIdentity{
			Length: length,
			SHA256: hex.EncodeToString(hash.Sum(nil)),
		},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return artifact.Manifest{}, fmt.Errorf("htmlartifact: encode sidecar: %w", err)
	}
	manifestBytes = append(manifestBytes, '\n')

	// Commit the sidecar last. An interruption after the HTML replacement leaves
	// either no manifest or a manifest whose content identity fails verification.
	if err := ctx.Err(); err != nil {
		return artifact.Manifest{}, err
	}
	if err := ensureNoSymlinkParents(store.root, resolved+sidecarSuffix); err != nil {
		return artifact.Manifest{}, err
	}
	if err := validateCommitCandidate(resolved + sidecarSuffix); err != nil {
		return artifact.Manifest{}, err
	}
	if err := store.write(resolved+sidecarSuffix, 0o644, func(writer io.Writer) error {
		_, err := writer.Write(manifestBytes)
		return err
	}); err != nil {
		return artifact.Manifest{}, fmt.Errorf("htmlartifact: commit sidecar: %w", err)
	}
	return manifest, nil
}

func (store *Store) resolve(relative string) (string, error) {
	if store == nil || store.root == "" || store.write == nil || store.rootErr != nil {
		return "", fmt.Errorf("htmlartifact: store is not configured")
	}
	if relative == "" || strings.Contains(relative, `\`) || filepath.IsAbs(relative) || filepath.VolumeName(relative) != "" || path.Clean(relative) != relative || relative == "." || relative == ".." || strings.HasPrefix(relative, "../") {
		return "", fmt.Errorf("htmlartifact: artifact path %q is invalid", relative)
	}
	resolved := filepath.Join(store.root, filepath.FromSlash(relative))
	withinRoot, err := filepath.Rel(store.root, resolved)
	if err != nil || withinRoot == ".." || strings.HasPrefix(withinRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("htmlartifact: artifact path %q escapes the store root", relative)
	}
	if err := ensureNoSymlinkParents(store.root, resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

func decodeManifest(data []byte) (artifact.Manifest, error) {
	if err := validateUniqueJSONMembers(data); err != nil {
		return artifact.Manifest{}, err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil || object == nil {
		return artifact.Manifest{}, fmt.Errorf("htmlartifact: sidecar must be a JSON object")
	}
	if err := rejectUnknownMembers(object, "sidecar", "schemaVersion", "fragment", "buildKey", "content"); err != nil {
		return artifact.Manifest{}, err
	}
	var manifest artifact.Manifest
	if err := decodeRequiredMember(object, "schemaVersion", &manifest.SchemaVersion); err != nil {
		return artifact.Manifest{}, err
	}
	if err := decodeFragment(object["fragment"], &manifest.Fragment); err != nil {
		return artifact.Manifest{}, err
	}
	if err := decodeRequiredMember(object, "buildKey", &manifest.BuildKey); err != nil {
		return artifact.Manifest{}, err
	}
	if err := decodeContent(object["content"], &manifest.Content); err != nil {
		return artifact.Manifest{}, err
	}
	return manifest, nil
}

func decodeFragment(data json.RawMessage, fragment *artifact.FragmentIdentity) error {
	object, err := decodeRequiredObject("fragment", data)
	if err != nil {
		return err
	}
	if err := rejectUnknownMembers(object, "fragment", "format", "kind", "resource"); err != nil {
		return err
	}
	if err := decodeRequiredMember(object, "format", &fragment.Format); err != nil {
		return err
	}
	if err := decodeRequiredMember(object, "kind", &fragment.Kind); err != nil {
		return err
	}
	return decodeRequiredMember(object, "resource", &fragment.Resource)
}

func decodeContent(data json.RawMessage, content *artifact.ContentIdentity) error {
	object, err := decodeRequiredObject("content", data)
	if err != nil {
		return err
	}
	if err := rejectUnknownMembers(object, "content", "length", "sha256"); err != nil {
		return err
	}
	if err := decodeRequiredMember(object, "length", &content.Length); err != nil {
		return err
	}
	return decodeRequiredMember(object, "sha256", &content.SHA256)
}

func decodeRequiredObject(name string, data json.RawMessage) (map[string]json.RawMessage, error) {
	if len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, fmt.Errorf("htmlartifact: sidecar member %q is required", name)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil || object == nil {
		return nil, fmt.Errorf("htmlartifact: sidecar member %q must be an object", name)
	}
	return object, nil
}

func decodeRequiredMember(object map[string]json.RawMessage, name string, destination any) error {
	data, ok := object[name]
	if !ok || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("htmlartifact: sidecar member %q is required", name)
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("htmlartifact: decode sidecar member %q: %w", name, err)
	}
	return nil
}

func rejectUnknownMembers(object map[string]json.RawMessage, objectName string, allowed ...string) error {
	known := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		known[name] = struct{}{}
	}
	for name := range object {
		if _, ok := known[name]; !ok {
			return fmt.Errorf("htmlartifact: unknown %s member %q", objectName, name)
		}
	}
	return nil
}

func validateUniqueJSONMembers(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := readUniqueJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("htmlartifact: sidecar contains trailing JSON values")
		}
		return err
	}
	return nil
}

func readUniqueJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			member, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := member.(string)
			if !ok {
				return fmt.Errorf("htmlartifact: sidecar object member is not a string")
			}
			if _, duplicate := seen[name]; duplicate {
				return fmt.Errorf("htmlartifact: duplicate sidecar member %q", name)
			}
			seen[name] = struct{}{}
			if err := readUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return fmt.Errorf("htmlartifact: invalid sidecar object")
		}
	case '[':
		for decoder.More() {
			if err := readUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return fmt.Errorf("htmlartifact: invalid sidecar array")
		}
	default:
		return fmt.Errorf("htmlartifact: invalid sidecar delimiter %q", delimiter)
	}
	return nil
}

func readBoundedRegular(path string, maximum int64) ([]byte, error) {
	file, err := openRegular(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("%w: %d bytes", errManifestTooLarge, maximum)
	}
	return data, nil
}

func openRegular(path string) (*os.File, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, errNonRegularFile
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) {
		_ = file.Close()
		return nil, errNonRegularFile
	}
	return file, nil
}

func ensureNoSymlinkParents(root, candidate string) error {
	rootInfo, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("htmlartifact: inspect store root: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return fmt.Errorf("htmlartifact: store root must be a real directory")
	}
	relativeParent, err := filepath.Rel(root, filepath.Dir(candidate))
	if err != nil {
		return fmt.Errorf("htmlartifact: resolve artifact parent: %w", err)
	}
	current := root
	for _, component := range strings.Split(relativeParent, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("htmlartifact: inspect artifact parent: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("htmlartifact: artifact parent %q is not a real directory", current)
		}
	}
	return nil
}

func validateCommitCandidate(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("htmlartifact: commit candidate %q is not a regular file", path)
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

type contextWriter struct {
	ctx    context.Context
	writer io.Writer
}

func (writer contextWriter) Write(buffer []byte) (int, error) {
	if err := writer.ctx.Err(); err != nil {
		return 0, err
	}
	return writer.writer.Write(buffer)
}

type countingWriter struct {
	writer io.Writer
	count  *uint64
}

func (writer *countingWriter) Write(buffer []byte) (int, error) {
	written, err := writer.writer.Write(buffer)
	*writer.count += uint64(written)
	return written, err
}

func (reader contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}

var _ port.HTMLArtifactStore = (*Store)(nil)
