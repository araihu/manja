package selfhosted

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/araihu/manja/application"
	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
	cacheadapter "github.com/araihu/manja/internal/adapters/cache"
	markdownadapter "github.com/araihu/manja/internal/adapters/markdown"
	openapiadapter "github.com/araihu/manja/internal/adapters/openapi"
	sourceadapter "github.com/araihu/manja/internal/adapters/source"
	storeadapter "github.com/araihu/manja/internal/adapters/store"
	"github.com/araihu/manja/internal/web"
)

type EndpointSidebarLabelMode = web.EndpointSidebarLabelMode

const (
	EndpointSidebarLabelAuto = web.EndpointSidebarLabelAuto
	EndpointSidebarLabelPath = web.EndpointSidebarLabelPath
)

type Options struct {
	ProjectID            string
	SourceID             string
	SourceKind           string
	SpecPath             string
	GitRepo              string
	GitRef               string
	GitUsername          string
	GitToken             string
	GitSSHPrivateKey     string
	DataDir              string
	StaticDir            string
	Branding             domain.DocsBranding
	EndpointSidebarLabel EndpointSidebarLabelMode
}

const (
	SourceKindFile = "file"
	SourceKindGit  = "git"
)

func New(ctx context.Context, specPath string) (http.Handler, error) {
	return NewServer(ctx, Options{SpecPath: specPath})
}

func NewWithOptions(ctx context.Context, options Options) (http.Handler, error) {
	return NewServer(ctx, options)
}

func NewServer(ctx context.Context, options Options) (http.Handler, error) {
	options = options.withDefaults()
	sourceFetcher, source, err := options.source()
	if err != nil {
		return nil, err
	}
	store := storeadapter.NewFileStore(options.DataDir)
	result, err := syncSource(ctx, sourceFetcher, store, options.ProjectID, options.SourceID, "startup")
	if err != nil {
		return nil, err
	}
	candidates, discoveryErr := discoverSourceRefs(ctx, sourceFetcher)
	managementRecord := result.Record
	if discoveryErr != nil {
		managementRecord.Result = domain.SyncResultFailure
		managementRecord.ErrorSummary = discoveryErr.Error()
	}
	publication, err := store.Publication(ctx, options.ProjectID, result.Revision.ID)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	syncAction := managementSyncAction(options, store, candidates)
	return web.NewServerWithOptions(result.Index, web.Options{
		Public: web.PublicOptions{
			EndpointSidebarLabel: web.EndpointSidebarLabelMode(options.EndpointSidebarLabel),
			MarkdownRenderer:     markdownadapter.NewRenderer(),
			StaticDir:            options.StaticDir,
			Branding:             options.Branding,
		},
		Management: web.ManagementOptions{
			Store:                store,
			SyncAction:           syncAction,
			PublishedIndexLoader: managementPublishedIndexLoader(options, store),
			Project: domain.Project{
				ID:   options.ProjectID,
				Name: result.Index.Title,
				SEO: domain.ProjectSEO{
					Robots: "index,follow",
				},
				Theme: domain.ThemeSettings{
					Theme:    "manja",
					DarkMode: "auto",
				},
				SourceIDs: []string{options.SourceID},
			},
			Source:      source,
			Revision:    result.Revision,
			Candidates:  candidates,
			Publication: publication,
			SyncRecord:  managementRecord,
		},
	}), nil
}

func syncSource(
	ctx context.Context,
	source port.SourceFetcher,
	store *storeadapter.FileStore,
	projectID string,
	sourceID string,
	trigger string,
) (application.SyncResult, error) {
	service, err := application.NewSyncService(application.SyncDependencies{
		Source: source, Parser: openapiadapter.Parser{}, UnitOfWork: store, SyncRecords: store,
		Blobs: store, Clock: wallClock{}, Cache: cacheadapter.NewMemory(),
	})
	if err != nil {
		return application.SyncResult{}, err
	}
	return service.Sync(ctx, application.SyncCommand{
		ContractID: projectID,
		SourceID:   sourceID,
		Trigger:    trigger,
	})
}

func discoverSourceRefs(ctx context.Context, source port.SourceFetcher) ([]domain.RevisionCandidate, error) {
	discoverer, ok := source.(port.SourceDiscoverer)
	if !ok {
		return nil, nil
	}
	return discoverer.Discover(ctx)
}

func managementSyncAction(options Options, store *storeadapter.FileStore, candidates []domain.RevisionCandidate) web.ManagementSyncAction {
	if options.SourceKind != SourceKindGit {
		return nil
	}
	candidateState := newCandidateMemory(candidates)
	return func(ctx context.Context, spec web.ManagedSpec, ref string) (web.ManagedSpec, error) {
		syncOptions := options
		syncOptions.GitRef = ref
		sourceFetcher, source, err := syncOptions.source()
		if err != nil {
			return web.ManagedSpec{}, err
		}
		result, err := syncSource(ctx, sourceFetcher, store, options.ProjectID, options.SourceID, "manual")
		if err != nil {
			return web.ManagedSpec{}, err
		}
		refreshedCandidates, discoveryErr := discoverSourceRefs(ctx, sourceFetcher)
		refreshedCandidates = candidateState.resolve(refreshedCandidates, discoveryErr)
		spec.Index = result.Index
		spec.Project.ID = firstNonBlank(spec.Project.ID, options.ProjectID)
		spec.Project.Name = result.Index.Title
		spec.Source = source
		spec.Revision = result.Revision
		spec.Candidates = refreshedCandidates
		spec.SyncRecord = result.Record
		return spec, nil
	}
}

type candidateMemory struct {
	mu        sync.Mutex
	lastKnown []domain.RevisionCandidate
}

func newCandidateMemory(candidates []domain.RevisionCandidate) *candidateMemory {
	return &candidateMemory{lastKnown: append([]domain.RevisionCandidate(nil), candidates...)}
}

func (m *candidateMemory) resolve(candidates []domain.RevisionCandidate, discoveryErr error) []domain.RevisionCandidate {
	m.mu.Lock()
	defer m.mu.Unlock()
	if discoveryErr == nil {
		m.lastKnown = append(m.lastKnown[:0], candidates...)
	}
	return append([]domain.RevisionCandidate(nil), m.lastKnown...)
}

func managementPublishedIndexLoader(options Options, store *storeadapter.FileStore) web.ManagementPublishedIndexLoader {
	parser := openapiadapter.Parser{}
	revisions, err := application.NewRevisionService(store)
	return func(ctx context.Context, spec web.ManagedSpec) (domain.SpecIndex, bool, error) {
		if err != nil {
			return domain.SpecIndex{}, false, err
		}
		if !spec.Publication.Public || strings.TrimSpace(spec.Publication.RevisionID) == "" {
			return domain.SpecIndex{}, false, nil
		}
		if spec.Publication.RevisionID == spec.Revision.ID &&
			spec.Publication.ProjectID == spec.Project.ID &&
			spec.Revision.ContractID == spec.Publication.ProjectID {
			return spec.Index, true, nil
		}
		revision, err := store.ContractRevision(
			ctx,
			spec.Publication.ProjectID,
			spec.Publication.RevisionID,
		)
		if err != nil {
			return domain.SpecIndex{}, false, err
		}
		specPath := firstNonBlank(spec.Source.SpecPath, options.SpecPath, "openapi.yaml")
		file := domain.SpecFile{
			SourceID: revision.SourceID,
			Path:     specPath,
			Format:   specFormat(specPath),
		}
		if strings.TrimSpace(revision.SpecBlobKey) == "" {
			file.Bytes, err = store.GetLegacy(ctx, legacySpecBlobKey(revision, file))
		} else {
			file.Bytes, err = revisions.LoadSpec(ctx, revision)
		}
		if err != nil {
			return domain.SpecIndex{}, false, err
		}
		index, err := parser.Parse(ctx, file, revision)
		if err != nil {
			return domain.SpecIndex{}, false, err
		}
		index.ProjectID = firstNonBlank(spec.Project.ID, spec.Index.ProjectID, options.ProjectID)
		index.RevisionID = revision.ID
		return index, true, nil
	}
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func specFormat(specPath string) string {
	switch strings.ToLower(filepath.Ext(specPath)) {
	case ".json":
		return "json"
	default:
		return "yaml"
	}
}

func legacySpecBlobKey(revision domain.ContractRevision, spec domain.SpecFile) string {
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(spec.Path)), ".")
	if extension == "" {
		extension = "yaml"
	}
	return filepath.ToSlash(filepath.Join("specs", revision.ID+"."+extension))
}

func (options Options) withDefaults() Options {
	if options.SourceKind == "" {
		if options.GitRepo != "" {
			options.SourceKind = SourceKindGit
		} else {
			options.SourceKind = SourceKindFile
		}
	}
	if options.ProjectID == "" {
		options.ProjectID = "default"
	}
	if options.SourceID == "" {
		if options.SourceKind == SourceKindGit && options.GitRepo != "" {
			options.SourceID = options.GitRepo
		} else {
			options.SourceID = "default"
		}
	}
	if options.DataDir == "" {
		options.DataDir = filepath.Join(".manja", "data")
	}
	switch options.EndpointSidebarLabel {
	case EndpointSidebarLabelPath:
	default:
		options.EndpointSidebarLabel = EndpointSidebarLabelAuto
	}
	return options
}

func (options Options) source() (port.SourceFetcher, domain.Source, error) {
	source := domain.Source{
		ID:        options.SourceID,
		ProjectID: options.ProjectID,
		Kind:      options.SourceKind,
		SpecPath:  options.SpecPath,
	}
	switch options.SourceKind {
	case SourceKindFile:
		return sourceadapter.File{Path: options.SpecPath}, source, nil
	case SourceKindGit:
		if options.GitRepo == "" {
			return nil, domain.Source{}, fmt.Errorf("git source repo is required")
		}
		if source.ID == "" {
			source.ID = options.GitRepo
		}
		return sourceadapter.Git{
			Repo:          options.GitRepo,
			Ref:           options.GitRef,
			Path:          options.SpecPath,
			Username:      options.GitUsername,
			Token:         options.GitToken,
			SSHPrivateKey: options.GitSSHPrivateKey,
		}, source, nil
	default:
		return nil, domain.Source{}, fmt.Errorf("unsupported source kind %q", options.SourceKind)
	}
}

type wallClock struct{}

func (wallClock) Now(context.Context) time.Time {
	return time.Now().UTC()
}
