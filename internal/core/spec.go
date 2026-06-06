package core

type SpecFile struct {
	SourceID string
	Path     string
	Format   string
	Bytes    []byte
}

type Revision struct {
	ID        string
	SourceID  string
	Ref       string
	CommitSHA string
	Version   string
}

type SpecIndex struct {
	ProjectID    string
	RevisionID   string
	Title        string
	Version      string
	Operations   []Operation
	Schemas      []Schema
	Search       []SearchDocument
	PublicRoutes []PublicRoute
}

type Operation struct {
	ID          string
	Anchor      string
	Method      string
	Path        string
	Summary     string
	Description string
	Tags        []string
	Deprecated  bool
}

type Schema struct {
	Name        string
	Description string
}

type SearchDocument struct {
	ID          string
	Title       string
	Description string
	Href        string
	Kind        string
	Section     string
	Keywords    []string
}

type PublicRoute struct {
	Path        string
	Title       string
	Description string
}
