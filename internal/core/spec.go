package core

type SpecFile struct {
	SourceID string
	Path     string
	Format   string
	Bytes    []byte
}

type Revision struct {
	ID          string
	SourceID    string
	Ref         string
	CommitSHA   string
	Version     string
	AuthorName  string
	AuthorEmail string
	Message     string
}

type RevisionCandidate struct {
	SourceID    string
	Ref         string
	Kind        string
	CommitSHA   string
	AuthorName  string
	AuthorEmail string
	Message     string
}

type SpecIndex struct {
	ProjectID       string
	RevisionID      string
	Title           string
	Version         string
	Branding        DocsBranding
	Overview        SpecOverview
	SpecDownload    SpecDownload
	Operations      []Operation
	Schemas         []Schema
	Search          []SearchDocument
	PublicRoutes    []PublicRoute
	ExampleSpecJSON string
}

type DocsBranding struct {
	DisplayName string
	Logo        DocsBrandingLogo
	Favicon     string
}

type DocsBrandingLogo struct {
	Src     string
	Alt     string
	HomeURL string
}

type SpecDownload struct {
	JSON     []byte
	Filename string
}

type SpecOverview struct {
	Description    string
	TermsOfService string
	Contact        SpecContact
	License        SpecLicense
	Servers        []SpecServer
}

type SpecContact struct {
	Name  string
	URL   string
	Email string
}

type SpecLicense struct {
	Name       string
	URL        string
	Identifier string
}

type SpecServer struct {
	URL         string
	Description string
	Variables   []SpecServerVariable
}

type SpecServerVariable struct {
	Name        string
	Default     string
	Description string
	Enum        []string
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
	ContentType     string
	Schema          SchemaSummary
	Example         string
	ExampleProvided bool
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
	Default     string
	Example     string
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
	Summary     SchemaSummary
	Example     SchemaExample
}

type SchemaExample struct {
	JSON     string
	Example  string
	Provided bool
}

type SearchDocument struct {
	ID          string
	Title       string
	Description string
	Href        string
	Kind        string
	Method      string
	Path        string
	Section     string
	Keywords    []string
}

type PublicRoute struct {
	Path        string
	Title       string
	Description string
}
