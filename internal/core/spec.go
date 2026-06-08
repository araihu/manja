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
	ProjectID       string
	RevisionID      string
	Title           string
	Version         string
	Operations      []Operation
	Schemas         []Schema
	Search          []SearchDocument
	PublicRoutes    []PublicRoute
	ExampleSpecJSON string
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
	Parameters  []OperationParameter
	RequestBody *OperationRequestBody
	Responses   []OperationResponse
	Security    []OperationSecurity
	Snippets    []RequestSnippet
}

type OperationParameter struct {
	Name        string
	In          string
	Required    bool
	Description string
	Schema      SchemaSummary
	Example     string
}

type OperationRequestBody struct {
	Description string
	Required    bool
	MediaTypes  []OperationMediaType
}

type OperationResponse struct {
	Status      string
	Description string
	MediaTypes  []OperationMediaType
}

type OperationMediaType struct {
	ContentType string
	Schema      SchemaSummary
	Example     string
}

type OperationSecurity struct {
	Name   string
	Scopes []string
}

type RequestSnippet struct {
	Label    string
	Language string
	Code     string
}

type SchemaSummary struct {
	Name        string
	Type        string
	Format      string
	Description string
	Properties  []SchemaProperty
	Items       *SchemaSummary
	JSON        string
}

type SchemaProperty struct {
	Name        string
	Required    bool
	Schema      SchemaSummary
	Description string
}

type Schema struct {
	Name        string
	Description string
	Example     SchemaExample
}

type SchemaExample struct {
	JSON    string
	Example string
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
