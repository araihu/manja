package core

import "context"

type Store interface {
	SaveProject(context.Context, Project) error
	Project(context.Context, string) (Project, error)
	SaveRevision(context.Context, Revision) error
	Revision(context.Context, string) (Revision, error)
	SavePublication(context.Context, Publication) error
	Publication(context.Context, string, string) (Publication, error)
	PublicPublicationByPath(context.Context, string) (Publication, error)
	SaveSyncRecord(context.Context, SyncRecord) error
}

type BlobStore interface {
	Put(context.Context, string, []byte) error
	Get(context.Context, string) ([]byte, error)
}

type SecretStore interface {
	PutSecret(context.Context, string, []byte) error
	GetSecret(context.Context, string) ([]byte, error)
}

type Cache interface {
	Get(string) ([]byte, bool)
	Set(string, []byte)
	Delete(string)
}

type SourceFetcher interface {
	Fetch(context.Context) (SpecFile, Revision, error)
}

type SourceDiscoverer interface {
	Discover(context.Context) ([]RevisionCandidate, error)
}

type Parser interface {
	Parse(context.Context, SpecFile, Revision) (SpecIndex, error)
}

type MarkdownRenderer interface {
	Render(context.Context, string) (MarkdownResult, error)
}

type MarkdownResult struct {
	HTML  string
	Plain string
}
