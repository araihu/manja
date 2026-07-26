package application

import (
	"fmt"
	"unicode/utf8"

	"github.com/araihu/manja/domain"
)

func validatePortSpecFile(file domain.SpecFile, requireComplete bool) error {
	for _, identity := range []struct {
		name       string
		value      string
		allowEmpty bool
	}{
		{name: "spec source id", value: file.SourceID, allowEmpty: true},
		{name: "spec path", value: file.Path, allowEmpty: !requireComplete},
		{name: "spec format", value: file.Format, allowEmpty: !requireComplete},
	} {
		if err := domain.ValidateCanonicalIdentity(identity.name, identity.value, identity.allowEmpty); err != nil {
			return err
		}
	}
	return nil
}

func validatePortRevision(revision domain.ContractRevision, requireID bool) error {
	for _, identity := range []struct {
		name       string
		value      string
		allowEmpty bool
	}{
		{name: "revision id", value: revision.ID, allowEmpty: !requireID},
		{name: "revision contract id", value: revision.ContractID, allowEmpty: true},
		{name: "revision source id", value: revision.SourceID, allowEmpty: true},
		{name: "revision blob key", value: revision.SpecBlobKey, allowEmpty: true},
		{name: "revision ref", value: revision.Ref, allowEmpty: true},
		{name: "revision commit sha", value: revision.CommitSHA, allowEmpty: true},
	} {
		if err := domain.ValidateCanonicalIdentity(identity.name, identity.value, identity.allowEmpty); err != nil {
			return err
		}
	}
	for _, display := range []struct {
		name  string
		value string
	}{
		{name: "revision version", value: revision.Version},
		{name: "revision author name", value: revision.AuthorName},
		{name: "revision author email", value: revision.AuthorEmail},
		{name: "revision message", value: revision.Message},
	} {
		if !utf8.ValidString(display.value) {
			return fmt.Errorf("%s must contain valid UTF-8", display.name)
		}
	}
	if revision.ReviewSnapshot != nil {
		if err := domain.ValidateContractSnapshot(*revision.ReviewSnapshot); err != nil {
			return fmt.Errorf("revision review snapshot: %w", err)
		}
	}
	return nil
}
