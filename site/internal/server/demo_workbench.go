package server

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/araihu/manja/application"
	"github.com/araihu/manja/application/port"
	core "github.com/araihu/manja/domain"
	cacheadapter "github.com/araihu/manja/internal/adapters/cache"
	markdownadapter "github.com/araihu/manja/internal/adapters/markdown"
	openapiadapter "github.com/araihu/manja/internal/adapters/openapi"
	sourceadapter "github.com/araihu/manja/internal/adapters/source"
	storeadapter "github.com/araihu/manja/internal/adapters/store"
	"github.com/araihu/manja/internal/web"
)

const demoSpecPath = "openapi.yaml"

type demoSpecSeed struct {
	ID               string
	ProjectID        string
	ProjectName      string
	ProjectSlug      string
	RepoName         string
	PublishedRef     string
	CandidateRef     string
	PublishedPath    string
	PublishedSpec    string
	CandidateSpec    string
	PublishedAuthor  demoGitAuthor
	CandidateAuthor  demoGitAuthor
	PublishedMessage string
	CandidateMessage string
}

type demoGitAuthor struct {
	Name  string
	Email string
}

type demoSpecConfig struct {
	Seed   demoSpecSeed
	Repo   string
	Source core.Source
}

type demoWorkbenchHandler struct {
	management   http.Handler
	store        *storeadapter.FileStore
	publicOpts   web.PublicOptions
	defaultBase  string
	configBySpec map[string]demoSpecConfig
	configByPath map[string]demoSpecConfig
}

func newDemoWorkbench(ctx context.Context, opts Options) (http.Handler, error) {
	root := filepath.Join(opts.dataDir(), "demo-workbench")
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve demo data root: %w", err)
	}
	root = absRoot
	if err := os.RemoveAll(root); err != nil {
		return nil, fmt.Errorf("reset demo data: %w", err)
	}
	reposRoot := filepath.Join(root, "repos")
	store := storeadapter.NewFileStore(filepath.Join(root, "data"))
	publicOpts := web.PublicOptions{
		EndpointSidebarLabel: web.EndpointSidebarLabelAuto,
		MarkdownRenderer:     markdownadapter.NewRenderer(),
		StaticDir:            opts.staticDir(),
		Branding:             opts.Branding,
	}

	specs := make([]web.ManagedSpec, 0, len(demoSpecSeeds))
	configBySpec := make(map[string]demoSpecConfig, len(demoSpecSeeds))
	configByPath := make(map[string]demoSpecConfig, len(demoSpecSeeds))
	for _, seed := range demoSpecSeeds {
		repo, err := createDemoGitRepo(ctx, reposRoot, seed)
		if err != nil {
			return nil, err
		}
		cfg := demoSpecConfig{
			Seed: seed,
			Repo: "file://" + filepath.ToSlash(repo),
			Source: core.Source{
				ID:        "local-git/" + seed.RepoName,
				ProjectID: seed.ProjectID,
				Kind:      "git",
				SpecPath:  demoSpecPath,
			},
		}
		published, err := syncDemoSpec(ctx, store, cfg, seed.PublishedRef, "demo-published")
		if err != nil {
			return nil, err
		}
		candidate, err := syncDemoSpec(ctx, store, cfg, seed.CandidateRef, "demo-candidate")
		if err != nil {
			return nil, err
		}
		candidates, err := discoverDemoCandidates(ctx, cfg)
		if err != nil {
			return nil, err
		}
		project := core.Project{
			ID:   seed.ProjectID,
			Name: seed.ProjectName,
			Slug: seed.ProjectSlug,
			SEO: core.ProjectSEO{
				Robots: "index,follow",
			},
			Theme: core.ThemeSettings{
				Theme:    "manja",
				DarkMode: "auto",
			},
			SourceIDs: []string{cfg.Source.ID},
		}
		publication := core.Publication{
			ProjectID:  seed.ProjectID,
			RevisionID: published.Revision.ID,
			Public:     true,
			Path:       seed.PublishedPath,
		}
		if err := store.SavePublication(ctx, publication); err != nil {
			return nil, err
		}
		spec := web.ManagedSpec{
			ID:             seed.ID,
			Index:          candidate.Index,
			PublishedIndex: published.Index,
			Project:        project,
			Source:         cfg.Source,
			Revision:       candidate.Revision,
			Candidates:     candidates,
			Publication:    publication,
			SyncRecord:     candidate.Record,
		}
		specs = append(specs, spec)
		configBySpec[seed.ID] = cfg
		configByPath[seed.PublishedPath] = cfg
	}

	handler := &demoWorkbenchHandler{
		store:        store,
		publicOpts:   publicOpts,
		defaultBase:  demoSpecSeeds[0].PublishedPath,
		configBySpec: configBySpec,
		configByPath: configByPath,
	}
	handler.management = web.NewManagementServer(core.SpecIndex{}, web.ManagementOptions{
		Store:                store,
		SyncAction:           handler.syncAction,
		PublishedIndexLoader: handler.publishedIndexLoader,
		Specs:                specs,
	})
	return handler, nil
}

func (h *demoWorkbenchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/manage" || strings.HasPrefix(r.URL.Path, "/manage/"):
		h.management.ServeHTTP(w, r)
		return
	case r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/"):
		web.NewAPIServer().ServeHTTP(w, r)
		return
	}

	base, cfg, ok := h.publicBase(r.URL.Path)
	if !ok {
		base = h.defaultBase
		cfg = h.configByPath[base]
	}
	idx, err := h.indexForPublishedPath(r.Context(), base, cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	public := web.NewPublicServerWithOptions(idx, h.publicOpts)
	h.servePublicAtBase(w, r, base, public)
}

func (h *demoWorkbenchHandler) publicBase(path string) (string, demoSpecConfig, bool) {
	for base, cfg := range h.configByPath {
		if path == base || strings.HasPrefix(path, base+"/") {
			return base, cfg, true
		}
	}
	return "", demoSpecConfig{}, false
}

func (h *demoWorkbenchHandler) servePublicAtBase(w http.ResponseWriter, r *http.Request, base string, public http.Handler) {
	rewritten := r.Clone(r.Context())
	switch {
	case r.URL.Path == base:
		rewritten.URL.Path = "/"
	case strings.HasPrefix(r.URL.Path, base+"/"):
		rewritten.URL.Path = strings.TrimPrefix(r.URL.Path, base)
	default:
		rewritten.URL.Path = r.URL.Path
	}
	recorder := &prefixingResponseWriter{ResponseWriter: w, prefix: base}
	public.ServeHTTP(recorder, rewritten)
	recorder.flush()
}

func (h *demoWorkbenchHandler) indexForPublishedPath(ctx context.Context, path string, cfg demoSpecConfig) (core.SpecIndex, error) {
	pub, err := h.store.PublicPublicationByPath(ctx, path)
	if err != nil {
		return core.SpecIndex{}, err
	}
	idx, ok, err := h.indexForRevision(ctx, cfg, pub.RevisionID)
	if err != nil {
		return core.SpecIndex{}, err
	}
	if !ok {
		return core.SpecIndex{}, fmt.Errorf("published revision %q is not available", pub.RevisionID)
	}
	return idx, nil
}

func (h *demoWorkbenchHandler) publishedIndexLoader(ctx context.Context, spec web.ManagedSpec) (core.SpecIndex, bool, error) {
	cfg, ok := h.configBySpec[spec.ID]
	if !ok {
		return core.SpecIndex{}, false, nil
	}
	if spec.Publication.RevisionID == spec.Revision.ID {
		return spec.Index, true, nil
	}
	return h.indexForRevision(ctx, cfg, spec.Publication.RevisionID)
}

func (h *demoWorkbenchHandler) indexForRevision(ctx context.Context, cfg demoSpecConfig, revisionID string) (core.SpecIndex, bool, error) {
	if strings.TrimSpace(revisionID) == "" {
		return core.SpecIndex{}, false, nil
	}
	rev, err := h.store.Revision(ctx, revisionID)
	if err != nil {
		return core.SpecIndex{}, false, err
	}
	file := core.SpecFile{
		SourceID: rev.SourceID,
		Path:     demoSpecPath,
		Format:   "yaml",
	}
	revisions, err := application.NewRevisionService(h.store)
	if err != nil {
		return core.SpecIndex{}, false, err
	}
	data, err := revisions.LoadSpec(ctx, rev)
	if err != nil {
		return core.SpecIndex{}, false, err
	}
	file.Bytes = data
	idx, err := openapiadapter.Parser{}.Parse(ctx, file, rev)
	if err != nil {
		return core.SpecIndex{}, false, err
	}
	idx.ProjectID = cfg.Seed.ProjectID
	idx.RevisionID = rev.ID
	return idx, true, nil
}

func (h *demoWorkbenchHandler) syncAction(ctx context.Context, spec web.ManagedSpec, ref string) (web.ManagedSpec, error) {
	cfg, ok := h.configBySpec[spec.ID]
	if !ok {
		return web.ManagedSpec{}, fmt.Errorf("demo spec %q is not configured", spec.ID)
	}
	result, err := syncDemoSpec(ctx, h.store, cfg, ref, "manual")
	if err != nil {
		return web.ManagedSpec{}, err
	}
	candidates, err := discoverDemoCandidates(ctx, cfg)
	if err != nil {
		return web.ManagedSpec{}, err
	}
	spec.Index = result.Index
	spec.Project.ID = cfg.Seed.ProjectID
	spec.Project.Name = result.Index.Title
	spec.Project.Slug = cfg.Seed.ProjectSlug
	spec.Source = cfg.Source
	spec.Revision = result.Revision
	spec.Candidates = candidates
	spec.SyncRecord = result.Record
	return spec, nil
}

func syncDemoSpec(ctx context.Context, store *storeadapter.FileStore, cfg demoSpecConfig, ref string, trigger string) (application.SyncResult, error) {
	service, err := application.NewSyncService(application.SyncDependencies{
		Source: sourceadapter.Git{
			Repo: cfg.Repo,
			Ref:  ref,
			Path: demoSpecPath,
		},
		Parser:     openapiadapter.Parser{},
		UnitOfWork: store,
		Blobs:      store,
		Cache:      cacheadapter.NewMemory(),
		Clock:      demoClock{},
	})
	if err != nil {
		return application.SyncResult{}, err
	}
	return service.Sync(ctx, application.SyncCommand{
		ContractID: cfg.Seed.ProjectID,
		SourceID:   cfg.Source.ID,
		Trigger:    trigger,
	})
}

func discoverDemoCandidates(ctx context.Context, cfg demoSpecConfig) ([]core.RevisionCandidate, error) {
	return sourceadapter.Git{
		Repo: cfg.Repo,
		Path: demoSpecPath,
	}.Discover(ctx)
}

type demoClock struct{}

func (demoClock) Now(context.Context) time.Time {
	return time.Now().UTC()
}

var _ port.Clock = demoClock{}

func createDemoGitRepo(ctx context.Context, root string, seed demoSpecSeed) (string, error) {
	worktree := filepath.Join(root, seed.ID+"-worktree")
	bare := filepath.Join(root, seed.ID+".git")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	if _, err := demoGit(ctx, "", nil, "init", "-b", "main", worktree); err != nil {
		return "", fmt.Errorf("init demo repo %s: %w", seed.ID, err)
	}
	if _, err := demoGit(ctx, worktree, nil, "config", "user.name", "Manja Demo"); err != nil {
		return "", err
	}
	if _, err := demoGit(ctx, worktree, nil, "config", "user.email", "demo@manja.test"); err != nil {
		return "", err
	}
	if err := commitDemoSpec(ctx, worktree, seed.PublishedSpec, seed.PublishedMessage, seed.PublishedAuthor); err != nil {
		return "", fmt.Errorf("commit published demo spec %s: %w", seed.ID, err)
	}
	if _, err := demoGit(ctx, worktree, nil, "tag", seed.PublishedRef); err != nil {
		return "", fmt.Errorf("tag demo spec %s: %w", seed.ID, err)
	}
	if _, err := demoGit(ctx, worktree, nil, "checkout", "-b", seed.CandidateRef); err != nil {
		return "", fmt.Errorf("create candidate branch %s: %w", seed.ID, err)
	}
	if err := commitDemoSpec(ctx, worktree, seed.CandidateSpec, seed.CandidateMessage, seed.CandidateAuthor); err != nil {
		return "", fmt.Errorf("commit candidate demo spec %s: %w", seed.ID, err)
	}
	if _, err := demoGit(ctx, "", nil, "clone", "--bare", worktree, bare); err != nil {
		return "", fmt.Errorf("clone bare demo repo %s: %w", seed.ID, err)
	}
	return bare, nil
}

func commitDemoSpec(ctx context.Context, repo string, spec string, message string, author demoGitAuthor) error {
	path := filepath.Join(repo, demoSpecPath)
	if err := os.WriteFile(path, []byte(spec), 0o600); err != nil {
		return err
	}
	if _, err := demoGit(ctx, repo, nil, "add", demoSpecPath); err != nil {
		return err
	}
	env := []string{
		"GIT_AUTHOR_NAME=" + author.Name,
		"GIT_AUTHOR_EMAIL=" + author.Email,
		"GIT_COMMITTER_NAME=" + author.Name,
		"GIT_COMMITTER_EMAIL=" + author.Email,
	}
	_, err := demoGit(ctx, repo, env, "commit", "-m", message)
	return err
}

func demoGit(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), env...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, out.String())
	}
	return strings.TrimSpace(out.String()), nil
}

var demoSpecSeeds = []demoSpecSeed{
	{
		ID:            "payments-api",
		ProjectID:     "payments",
		ProjectName:   "Acme Payments API",
		ProjectSlug:   "payments",
		RepoName:      "payments-api.git",
		PublishedRef:  "v1.0.0",
		CandidateRef:  "release/breaking-auth",
		PublishedPath: "/payments/v1",
		PublishedAuthor: demoGitAuthor{
			Name:  "Nina Park",
			Email: "nina@acme.test",
		},
		CandidateAuthor: demoGitAuthor{
			Name:  "Ada Lovelace",
			Email: "ada@acme.test",
		},
		PublishedMessage: "Publish payments v1 contract",
		CandidateMessage: "Require payment versioning and retire customers endpoint",
		PublishedSpec: `openapi: 3.1.0
info:
  title: Acme Payments API
  version: 2024-10-01
paths:
  /payments:
    get:
      operationId: listPayments
      summary: List payments
      parameters:
        - name: expand
          in: query
          required: false
          schema:
            type: string
      responses:
        "200":
          description: Payments returned
        "404":
          description: Customer account was not found
    post:
      operationId: createPayment
      summary: Create a payment
      requestBody:
        required: false
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/PaymentCreate"
      responses:
        "201":
          description: Payment created
  /customers:
    get:
      operationId: listCustomers
      summary: List customers
      responses:
        "200":
          description: Customers returned
components:
  schemas:
    Payment:
      type: object
      properties:
        id:
          type: string
        amount:
          type: integer
    PaymentCreate:
      type: object
      properties:
        amount:
          type: integer
    Customer:
      type: object
      properties:
        id:
          type: string
`,
		CandidateSpec: `openapi: 3.1.0
info:
  title: Acme Payments API
  version: 2025-02-15
paths:
  /payments:
    get:
      operationId: listPayments
      summary: List payments
      parameters:
        - name: expand
          in: query
          required: true
          schema:
            type: string
        - name: api-version
          in: header
          required: true
          schema:
            type: string
      responses:
        "200":
          description: Payments returned
        "202":
          description: Payments export queued
    post:
      operationId: createPayment
      summary: Create a payment
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/PaymentCreate"
      responses:
        "201":
          description: Payment created
  /payment-intents:
    post:
      operationId: createPaymentIntent
      summary: Create a payment intent
      responses:
        "202":
          description: Payment intent queued
components:
  schemas:
    Payment:
      type: object
      properties:
        id:
          type: string
        amount:
          type: integer
    PaymentCreate:
      type: object
      required:
        - amount
      properties:
        amount:
          type: integer
    Refund:
      type: object
      properties:
        id:
          type: string
`,
	},
	{
		ID:            "identity-api",
		ProjectID:     "identity",
		ProjectName:   "Acme Identity API",
		ProjectSlug:   "identity",
		RepoName:      "identity-api.git",
		PublishedRef:  "v2.3.0",
		CandidateRef:  "feature/group-directory",
		PublishedPath: "/identity/v2",
		PublishedAuthor: demoGitAuthor{
			Name:  "Miguel Santos",
			Email: "miguel@acme.test",
		},
		CandidateAuthor: demoGitAuthor{
			Name:  "Bruno Dias",
			Email: "bruno@acme.test",
		},
		PublishedMessage: "Publish identity v2.3 contract",
		CandidateMessage: "Add group directory endpoints",
		PublishedSpec: `openapi: 3.1.0
info:
  title: Acme Identity API
  version: 2.3.0
paths:
  /users:
    get:
      operationId: listUsers
      summary: List users
      responses:
        "200":
          description: Users returned
components:
  schemas:
    User:
      type: object
      properties:
        id:
          type: string
        email:
          type: string
`,
		CandidateSpec: `openapi: 3.1.0
info:
  title: Acme Identity API
  version: 2.4.0
paths:
  /users:
    get:
      operationId: listUsers
      summary: List users
      responses:
        "200":
          description: Users returned
        "206":
          description: Partial user page returned
  /groups:
    get:
      operationId: listGroups
      summary: List groups
      responses:
        "200":
          description: Groups returned
components:
  schemas:
    User:
      type: object
      properties:
        id:
          type: string
        email:
          type: string
    Group:
      type: object
      properties:
        id:
          type: string
        name:
          type: string
`,
	},
	{
		ID:            "notifications-api",
		ProjectID:     "notifications",
		ProjectName:   "Acme Notifications API",
		ProjectSlug:   "notifications",
		RepoName:      "notifications-api.git",
		PublishedRef:  "v1.8.0",
		CandidateRef:  "release/required-template",
		PublishedPath: "/notifications/v1",
		PublishedAuthor: demoGitAuthor{
			Name:  "Priya Shah",
			Email: "priya@acme.test",
		},
		CandidateAuthor: demoGitAuthor{
			Name:  "Carla Mendes",
			Email: "carla@acme.test",
		},
		PublishedMessage: "Publish notifications v1.8 contract",
		CandidateMessage: "Require delivery templates for message sends",
		PublishedSpec: `openapi: 3.1.0
info:
  title: Acme Notifications API
  version: 1.8.0
paths:
  /messages:
    post:
      operationId: sendMessage
      summary: Send a message
      requestBody:
        required: false
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/MessageSend"
      responses:
        "202":
          description: Message accepted
        "400":
          description: Message was invalid
components:
  schemas:
    MessageSend:
      type: object
      properties:
        to:
          type: string
        body:
          type: string
`,
		CandidateSpec: `openapi: 3.1.0
info:
  title: Acme Notifications API
  version: 1.9.0
paths:
  /messages:
    post:
      operationId: sendMessage
      summary: Send a message
      parameters:
        - name: X-Delivery-Policy
          in: header
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/MessageSend"
      responses:
        "202":
          description: Message accepted
  /templates:
    get:
      operationId: listTemplates
      summary: List templates
      responses:
        "200":
          description: Templates returned
components:
  schemas:
    MessageSend:
      type: object
      required:
        - template_id
      properties:
        to:
          type: string
        template_id:
          type: string
    Template:
      type: object
      properties:
        id:
          type: string
        name:
          type: string
`,
	},
}
