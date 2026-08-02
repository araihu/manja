package domain

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"path"
	"strings"
)

func ValidateCatalogIndex(index CatalogIndex) error {
	return validateCatalogIndexWithDetailHasher(index, sha256.Sum256)
}

func ValidateCatalogCandidate(candidate CatalogCandidate) error {
	if err := validateUTF8Strings("catalog candidate", candidate); err != nil {
		return err
	}
	if err := validateCatalogKey("catalog id", candidate.ID); err != nil {
		return err
	}
	if err := ValidateCanonicalIdentity("catalog profile", string(candidate.ProfileID), false); err != nil {
		return err
	}
	if err := validateCatalogRevision(candidate.Revision); err != nil {
		return err
	}
	if len(candidate.Documents) == 0 {
		return fmt.Errorf("catalog requires at least one document")
	}
	if len(candidate.Documents) > maxCatalogDocuments {
		return fmt.Errorf("catalog documents exceed %d", maxCatalogDocuments)
	}
	keys := make(map[string]struct{}, len(candidate.Documents))
	paths := make(map[string]struct{}, len(candidate.Documents))
	for index, document := range candidate.Documents {
		if err := validateCatalogDocument(index, document); err != nil {
			return err
		}
		if _, exists := keys[document.Key]; exists {
			return fmt.Errorf("catalog document key %q is duplicated", document.Key)
		}
		keys[document.Key] = struct{}{}
		if _, exists := paths[document.SourcePath]; exists {
			return fmt.Errorf("catalog document source path %q is duplicated", document.SourcePath)
		}
		paths[document.SourcePath] = struct{}{}
	}
	if candidate.DefaultDocumentKey != "" {
		if err := validateCatalogKey("default document key", candidate.DefaultDocumentKey); err != nil {
			return err
		}
		if _, exists := keys[candidate.DefaultDocumentKey]; !exists {
			return fmt.Errorf("default document key %q does not exist", candidate.DefaultDocumentKey)
		}
	}
	return nil
}

func validateCatalogIndexWithDetailHasher(index CatalogIndex, hasher detailHasher) error {
	if err := validateUTF8Strings("catalog index", index); err != nil {
		return err
	}
	if err := validateCatalogKey("catalog index id", index.CatalogID); err != nil {
		return err
	}
	if err := ValidateCanonicalIdentity("catalog index revision id", index.RevisionID, false); err != nil {
		return err
	}
	if err := ValidateCanonicalIdentity("catalog index profile", string(index.ProfileID), false); err != nil {
		return err
	}
	if len(index.Documents) == 0 || len(index.Documents) > maxCatalogDocuments {
		return fmt.Errorf("catalog index document count is invalid")
	}
	keys := make(map[string]struct{}, len(index.Documents))
	paths := make(map[string]struct{}, len(index.Documents))
	identities := make(map[DetailID][]byte)
	for documentIndex, document := range index.Documents {
		if err := validateCatalogKey(fmt.Sprintf("catalog index document %d key", documentIndex), document.Key); err != nil {
			return err
		}
		if err := validateCatalogSourcePath(fmt.Sprintf("catalog index document %d source path", documentIndex), document.SourcePath); err != nil {
			return err
		}
		if _, exists := keys[document.Key]; exists {
			return fmt.Errorf("catalog index document key %q is duplicated", document.Key)
		}
		keys[document.Key] = struct{}{}
		if _, exists := paths[document.SourcePath]; exists {
			return fmt.Errorf("catalog index source path %q is duplicated", document.SourcePath)
		}
		paths[document.SourcePath] = struct{}{}
		if err := ValidateSpecIndex(document.Index); err != nil {
			return fmt.Errorf("catalog document %q: %w", document.Key, err)
		}
		for _, operation := range document.Index.Operations {
			identity, preimage, err := newOperationDetailIdentity(
				index.CatalogID, document.Key, operation.Method, operation.Path, hasher,
			)
			if err != nil {
				return err
			}
			if err := registerDetailIdentity(identities, identity, preimage); err != nil {
				return err
			}
		}
		for _, schema := range document.Index.Schemas {
			identity, preimage, err := newSchemaDetailIdentity(index.CatalogID, document.Key, schema.Name, hasher)
			if err != nil {
				return err
			}
			if err := registerDetailIdentity(identities, identity, preimage); err != nil {
				return err
			}
		}
	}
	return nil
}

func registerDetailIdentity(identities map[DetailID][]byte, identity DetailID, preimage []byte) error {
	if existing, exists := identities[identity]; exists && !bytes.Equal(existing, preimage) {
		return fmt.Errorf("detail identity collision for %s", identity)
	}
	identities[identity] = append([]byte(nil), preimage...)
	return nil
}

func validateCatalogRevision(revision CatalogRevision) error {
	if err := ValidateCanonicalIdentity("catalog revision id", revision.ID, false); err != nil {
		return err
	}
	if !isLowerSHA256(revision.ManifestDigest) {
		return fmt.Errorf("catalog revision manifest digest must be lowercase SHA-256")
	}
	switch revision.Kind {
	case CatalogRevisionFiles:
		if revision.CommitSHA != "" {
			return fmt.Errorf("file catalog revision must not contain a commit SHA")
		}
	case CatalogRevisionGit:
		if !isLowerHex(revision.CommitSHA, 40) && !isLowerHex(revision.CommitSHA, 64) {
			return fmt.Errorf("Git catalog revision commit SHA must be a full lowercase object ID")
		}
	default:
		return fmt.Errorf("catalog revision kind %q is unsupported", revision.Kind)
	}
	return nil
}

func validateCatalogDocument(index int, document CatalogDocument) error {
	if err := validateCatalogKey(fmt.Sprintf("catalog document %d key", index), document.Key); err != nil {
		return err
	}
	if err := validateCatalogSourcePath(fmt.Sprintf("catalog document %d source path", index), document.SourcePath); err != nil {
		return err
	}
	switch document.Format {
	case CatalogFormatJSON, CatalogFormatYAML:
	default:
		return fmt.Errorf("catalog document %d format %q is unsupported", index, document.Format)
	}
	if len(document.Bytes) == 0 {
		return fmt.Errorf("catalog document %d bytes are required", index)
	}
	if len(document.Bytes) > maxCatalogDocumentBytes {
		return fmt.Errorf("catalog document %d exceeds %d bytes", index, maxCatalogDocumentBytes)
	}
	return nil
}

func validateCatalogKey(name, value string) error {
	if err := ValidateCanonicalIdentity(name, value, false); err != nil {
		return err
	}
	if len(value) > 64 || !isLowerCatalogKeyCharacter(value[0]) || !isLowerCatalogKeyCharacter(value[len(value)-1]) {
		return fmt.Errorf("%s is invalid", name)
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '.' && character != '_' && character != '-' {
			return fmt.Errorf("%s is invalid", name)
		}
	}
	return nil
}

func isLowerCatalogKeyCharacter(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9'
}

func validateCatalogSourcePath(name, value string) error {
	if err := ValidateCanonicalIdentity(name, value, false); err != nil {
		return err
	}
	if strings.HasPrefix(value, "/") || strings.Contains(value, `\`) || value == "." || path.Clean(value) != value || strings.HasPrefix(value, "../") {
		return fmt.Errorf("%s is invalid", name)
	}
	return nil
}

func isLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
