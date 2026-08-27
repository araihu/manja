package distribution

import (
	"errors"
	"os"
	"path/filepath"
)

// LoadRepositoryLegalEvidence reads the three repository-root legal files as
// immutable byte evidence. It deliberately leaves Holder and YearRange empty:
// file presence and hashes cannot establish first-party attribution.
func LoadRepositoryLegalEvidence(root string) (LegalEvidence, error) {
	if root == "" {
		return LegalEvidence{}, &InventoryError{Code: "artifact.root.missing", Detail: "legal material root is required"}
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return LegalEvidence{}, &InventoryError{Code: "artifact.root.missing", Path: root, Detail: err.Error()}
		}
		return LegalEvidence{}, &InventoryError{Code: "artifact.root.unreadable", Path: root, Detail: err.Error()}
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return LegalEvidence{}, &InventoryError{Code: "artifact.root.link", Path: root, Detail: "legal material root must not be a symbolic link"}
	}
	if !rootInfo.IsDir() {
		return LegalEvidence{}, &InventoryError{Code: "artifact.root.not_directory", Path: root, Detail: "legal material root must be a directory"}
	}

	license, err := loadRepositoryLegalFile(root, "LICENSE")
	if err != nil {
		return LegalEvidence{}, err
	}
	notice, err := loadRepositoryLegalFile(root, "NOTICE")
	if err != nil {
		return LegalEvidence{}, err
	}
	thirdParty, err := loadRepositoryLegalFile(root, "THIRD_PARTY_NOTICES.md")
	if err != nil {
		return LegalEvidence{}, err
	}
	return LegalEvidence{License: license, Notice: notice, ThirdParty: thirdParty}, nil
}

func loadRepositoryLegalFile(root, relative string) (FileEvidence, error) {
	pathValue := filepath.Join(root, filepath.FromSlash(relative))
	info, err := os.Lstat(pathValue)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FileEvidence{}, &InventoryError{Code: "artifact.file.missing", Path: relative, Detail: err.Error()}
		}
		return FileEvidence{}, &InventoryError{Code: "artifact.file.unreadable", Path: relative, Detail: err.Error()}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return FileEvidence{}, &InventoryError{Code: "artifact.file.link", Path: relative, Detail: "legal material must be a regular file, not a symbolic link"}
	}
	if info.Mode()&os.ModeType != 0 || !info.Mode().IsRegular() {
		return FileEvidence{}, &InventoryError{Code: "artifact.file.type_invalid", Path: relative, Detail: "legal material must be a regular file"}
	}
	file, err := inventoryFile(pathValue, relative)
	if err != nil {
		return FileEvidence{}, err
	}
	if file.Size <= 0 {
		return FileEvidence{}, &InventoryError{Code: "artifact.file.empty", Path: relative, Detail: "legal material must not be empty"}
	}
	return file, nil
}
