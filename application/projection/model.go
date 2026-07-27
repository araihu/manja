package projection

type Builder struct{}

type Document struct {
	FormatVersion         uint32               `json:"formatVersion"`
	ProjectID             string               `json:"projectId"`
	RevisionID            string               `json:"revisionId"`
	Title                 string               `json:"title"`
	APIVersion            string               `json:"apiVersion"`
	Branding              Branding             `json:"branding"`
	Overview              Overview             `json:"overview"`
	MainLandmark          Landmark             `json:"mainLandmark"`
	OperationGroupHeading Heading              `json:"operationGroupHeading"`
	SchemaGroupHeading    Heading              `json:"schemaGroupHeading"`
	SidebarSections       []SidebarSection     `json:"sidebarSections"`
	Operations            []OperationDirectory `json:"operations"`
	OperationDetails      []OperationDetail    `json:"operationDetails"`
	Schemas               []SchemaDirectory    `json:"schemas"`
	SchemaDetails         []SchemaDetail       `json:"schemaDetails"`
	SchemaNodes           []SchemaNode         `json:"schemaNodes"`
	Search                []SearchRecord       `json:"search"`
	PublicRoutes          []PublicRoute        `json:"publicRoutes"`
}

type Branding struct {
	DisplayName  string `json:"displayName"`
	LogoSrc      string `json:"logoSrc"`
	LogoAlt      string `json:"logoAlt"`
	LogoHomeHref string `json:"logoHomeHref"`
	FaviconHref  string `json:"faviconHref"`
}

type Overview struct {
	Anchor               string   `json:"anchor"`
	Href                 string   `json:"href"`
	HeadingID            string   `json:"headingId"`
	Heading              string   `json:"heading"`
	HeadingLevel         uint32   `json:"headingLevel"`
	Description          string   `json:"description"`
	TermsOfService       string   `json:"termsOfService"`
	SpecDownloadFilename string   `json:"specDownloadFilename"`
	Contact              Contact  `json:"contact"`
	License              License  `json:"license"`
	Servers              []Server `json:"servers"`
}

type Contact struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Email string `json:"email"`
}

type License struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	Identifier string `json:"identifier"`
}

type Landmark struct {
	ID   string `json:"id"`
	Role string `json:"role"`
}

type Heading struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Level uint32 `json:"level"`
}

type Server struct {
	Ordinal     uint32           `json:"ordinal"`
	ID          string           `json:"id"`
	URL         string           `json:"url"`
	Description string           `json:"description"`
	Variables   []ServerVariable `json:"variables"`
}

type ServerVariable struct {
	Ordinal     uint32       `json:"ordinal"`
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Default     string       `json:"default"`
	Description string       `json:"description"`
	Enum        []TextRecord `json:"enum"`
}

type TextRecord struct {
	Ordinal uint32 `json:"ordinal"`
	ID      string `json:"id"`
	Value   string `json:"value"`
}

type SidebarSection struct {
	Ordinal uint32        `json:"ordinal"`
	ID      string        `json:"id"`
	Kind    string        `json:"kind"`
	Title   string        `json:"title"`
	Href    string        `json:"href"`
	Items   []SidebarItem `json:"items"`
}

type SidebarItem struct {
	Ordinal uint32 `json:"ordinal"`
	ID      string `json:"id"`
	Anchor  string `json:"anchor"`
	Href    string `json:"href"`
	Label   string `json:"label"`
	Method  string `json:"method"`
}

type OperationDirectory struct {
	Ordinal    uint32       `json:"ordinal"`
	ID         string       `json:"id"`
	Anchor     string       `json:"anchor"`
	Href       string       `json:"href"`
	Method     string       `json:"method"`
	Path       string       `json:"path"`
	Title      string       `json:"title"`
	Deprecated bool         `json:"deprecated"`
	Sections   []TextRecord `json:"sections"`
}

type OperationDetail struct {
	Ordinal        uint32                `json:"ordinal"`
	ID             string                `json:"id"`
	Anchor         string                `json:"anchor"`
	Href           string                `json:"href"`
	HeadingID      string                `json:"headingId"`
	Heading        string                `json:"heading"`
	HeadingLevel   uint32                `json:"headingLevel"`
	Method         string                `json:"method"`
	Path           string                `json:"path"`
	Summary        string                `json:"summary"`
	Description    string                `json:"description"`
	Deprecated     bool                  `json:"deprecated"`
	Tags           []TextRecord          `json:"tags"`
	Parameters     []Parameter           `json:"parameters"`
	HasRequestBody bool                  `json:"hasRequestBody"`
	RequestBody    RequestBody           `json:"requestBody"`
	Responses      []Response            `json:"responses"`
	Security       []SecurityRequirement `json:"security"`
	CodeSamples    []CodeSample          `json:"codeSamples"`
}

type Parameter struct {
	Ordinal     uint32    `json:"ordinal"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	In          string    `json:"in"`
	Required    bool      `json:"required"`
	Description string    `json:"description"`
	SchemaRef   SchemaRef `json:"schemaRef"`
	Examples    []Example `json:"examples"`
}

type RequestBody struct {
	Description string      `json:"description"`
	Required    bool        `json:"required"`
	MediaTypes  []MediaType `json:"mediaTypes"`
}

type Response struct {
	Ordinal     uint32      `json:"ordinal"`
	ID          string      `json:"id"`
	Status      string      `json:"status"`
	Description string      `json:"description"`
	MediaTypes  []MediaType `json:"mediaTypes"`
}

type MediaType struct {
	Ordinal     uint32    `json:"ordinal"`
	ID          string    `json:"id"`
	ContentType string    `json:"contentType"`
	SchemaRef   SchemaRef `json:"schemaRef"`
	Examples    []Example `json:"examples"`
}

type SecurityRequirement struct {
	Ordinal uint32       `json:"ordinal"`
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Scopes  []TextRecord `json:"scopes"`
}

type CodeSample struct {
	Ordinal  uint32 `json:"ordinal"`
	ID       string `json:"id"`
	Label    string `json:"label"`
	Language string `json:"language"`
	Code     string `json:"code"`
}

type SchemaRef uint32

type SchemaNode struct {
	Ordinal      uint32               `json:"ordinal"`
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	Type         string               `json:"type"`
	Format       string               `json:"format"`
	Description  string               `json:"description"`
	DefaultValue string               `json:"defaultValue"`
	ExampleText  string               `json:"exampleText"`
	JSON         string               `json:"json"`
	Properties   []SchemaNodeProperty `json:"properties"`
	Items        []SchemaNodeItem     `json:"items"`
}

type SchemaNodeProperty struct {
	Ordinal     uint32    `json:"ordinal"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Required    bool      `json:"required"`
	Description string    `json:"description"`
	SchemaRef   SchemaRef `json:"schemaRef"`
}

type SchemaNodeItem struct {
	Ordinal   uint32    `json:"ordinal"`
	ID        string    `json:"id"`
	SchemaRef SchemaRef `json:"schemaRef"`
}

type Example struct {
	Ordinal  uint32 `json:"ordinal"`
	ID       string `json:"id"`
	Text     string `json:"text"`
	Provided bool   `json:"provided"`
}

type SchemaDirectory struct {
	Ordinal     uint32 `json:"ordinal"`
	ID          string `json:"id"`
	Anchor      string `json:"anchor"`
	Href        string `json:"href"`
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type SchemaDetail struct {
	Ordinal           uint32    `json:"ordinal"`
	ID                string    `json:"id"`
	Anchor            string    `json:"anchor"`
	Href              string    `json:"href"`
	HeadingID         string    `json:"headingId"`
	Heading           string    `json:"heading"`
	HeadingLevel      uint32    `json:"headingLevel"`
	Description       string    `json:"description"`
	SchemaRef         SchemaRef `json:"schemaRef"`
	ExampleSchemaJSON string    `json:"exampleSchemaJSON"`
	Examples          []Example `json:"examples"`
}

type SearchRecord struct {
	Ordinal     uint32       `json:"ordinal"`
	ID          string       `json:"id"`
	ResultID    string       `json:"resultId"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Href        string       `json:"href"`
	Kind        string       `json:"kind"`
	Method      string       `json:"method"`
	Path        string       `json:"path"`
	Section     string       `json:"section"`
	Keywords    []TextRecord `json:"keywords"`
}

type PublicRoute struct {
	Ordinal     uint32 `json:"ordinal"`
	Path        string `json:"path"`
	Title       string `json:"title"`
	Description string `json:"description"`
}
