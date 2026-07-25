package domain

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
