package core

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

type Publication struct {
	ProjectID  string
	RevisionID string
	Public     bool
	Path       string
	Hostname   string
}

type Actor struct {
	Anonymous  bool
	UserID     string
	ProjectIDs []string
}

func (p Publication) VisibleTo(actor Actor) bool {
	if p.Public {
		return true
	}
	if actor.Anonymous {
		return false
	}
	for _, id := range actor.ProjectIDs {
		if id == p.ProjectID {
			return true
		}
	}
	return false
}
