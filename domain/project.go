package domain

import "fmt"

type Project struct {
	ID        string
	Name      string
	Slug      string
	SEO       ProjectSEO
	Theme     ThemeSettings
	SourceIDs []string
}

type ProjectSEO struct {
	TitleTemplate string
	Description   string
	CanonicalBase string
	SocialImage   string
	Robots        string
}

type ThemeSettings struct {
	Theme    string
	DarkMode string
}

type Source struct {
	ID           string
	ProjectID    string
	Kind         string
	SpecPath     string
	CredentialID string
}

type Credential struct {
	ID        string
	SourceID  string
	Kind      string
	SecretRef string
}

// ValidateProject verifies persisted project identities and requires every
// display/configuration string to round-trip as UTF-8 without normalizing
// otherwise legitimate Unicode, whitespace, or newlines.
func ValidateProject(project Project) error {
	if err := validateUTF8Strings("project", project); err != nil {
		return err
	}
	if err := ValidateCanonicalIdentity("project id", project.ID, false); err != nil {
		return err
	}
	if err := ValidateCanonicalIdentity("project slug", project.Slug, true); err != nil {
		return err
	}
	for index, sourceID := range project.SourceIDs {
		if err := ValidateCanonicalIdentity(
			fmt.Sprintf("project source id %d", index),
			sourceID,
			false,
		); err != nil {
			return err
		}
	}
	return nil
}
