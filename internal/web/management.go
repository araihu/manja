package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/araihu/manja/internal/core"
	"github.com/araihu/manja/internal/web/templates"
)

const managementSyncTimeout = 30 * time.Second

type ManagementStore interface {
	SavePublication(context.Context, core.Publication) error
}

type ManagementSyncAction func(context.Context, ManagedSpec, string) (ManagedSpec, error)

type ManagementOptions struct {
	Store       ManagementStore
	SyncAction  ManagementSyncAction
	Specs       []ManagedSpec
	Project     core.Project
	Source      core.Source
	Revision    core.Revision
	Candidates  []core.RevisionCandidate
	Publication core.Publication
	SyncRecord  core.SyncRecord
}

type ManagedSpec struct {
	ID          string
	Index       core.SpecIndex
	Project     core.Project
	Source      core.Source
	Revision    core.Revision
	Candidates  []core.RevisionCandidate
	Publication core.Publication
	SyncRecord  core.SyncRecord
}

func NewManagementServer(idx core.SpecIndex, opts ManagementOptions) http.Handler {
	srv := &managementServer{store: opts.Store, syncAction: opts.SyncAction, specs: normalizeManagedSpecs(idx, opts)}
	mux := http.NewServeMux()
	mux.HandleFunc("/manage", srv.overview)
	mux.HandleFunc("/manage/publication", srv.updatePublication)
	mux.HandleFunc("/manage/sync", srv.syncRef)
	return mux
}

type managementServer struct {
	store      ManagementStore
	syncAction ManagementSyncAction
	specs      []ManagedSpec
	mu         sync.RWMutex
}

func (s *managementServer) overview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	specs := cloneManagedSpecs(s.specs)
	s.mu.RUnlock()

	if err := templates.ManagementOverview(managementOverviewModel(specs)).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *managementServer) updatePublication(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	specID := strings.TrimSpace(r.FormValue("spec_id"))
	specIndex := 0
	if specID != "" {
		specIndex = -1
		for i, spec := range s.specs {
			if spec.ID == specID {
				specIndex = i
				break
			}
		}
		if specIndex == -1 {
			http.Error(w, "managed spec not found", http.StatusBadRequest)
			return
		}
	}
	if len(s.specs) == 0 {
		http.Error(w, "managed spec is required", http.StatusBadRequest)
		return
	}

	spec := s.specs[specIndex]
	pub := spec.Publication
	pub.ProjectID = firstNonBlank(pub.ProjectID, spec.Project.ID, spec.Index.ProjectID)
	pub.RevisionID = firstNonBlank(pub.RevisionID, spec.Revision.ID, spec.Index.RevisionID)
	pub.Path = strings.TrimSpace(r.FormValue("path"))
	pub.Public = strings.TrimSpace(r.FormValue("visibility")) == "public"
	if pub.ProjectID == "" || pub.RevisionID == "" {
		http.Error(w, "publication project and revision are required", http.StatusBadRequest)
		return
	}
	if s.store != nil {
		if err := s.store.SavePublication(r.Context(), pub); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	s.specs[specIndex].Publication = pub
	http.Redirect(w, r, "/manage", http.StatusSeeOther)
}

func (s *managementServer) syncRef(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if s.syncAction == nil {
		http.Error(w, "sync action is not configured", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	specIndex, err := s.managedSpecIndex(strings.TrimSpace(r.FormValue("spec_id")))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ref := strings.TrimSpace(r.FormValue("ref"))
	if ref == "" {
		http.Error(w, "ref is required", http.StatusBadRequest)
		return
	}
	spec := s.specs[specIndex]
	if !managementRefAllowed(spec.Candidates, ref) {
		http.Error(w, "ref is not available for this source", http.StatusBadRequest)
		return
	}

	syncCtx, cancel := context.WithTimeout(r.Context(), managementSyncTimeout)
	defer cancel()
	updated, err := s.syncAction(syncCtx, spec, ref)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	updated = normalizeManagedSpecs(core.SpecIndex{}, ManagementOptions{Specs: []ManagedSpec{updated}})[0]
	if len(updated.Candidates) == 0 {
		updated.Candidates = spec.Candidates
	}
	s.specs[specIndex] = updated
	if strings.TrimSpace(r.FormValue("publish")) == "public" {
		pub := updated.Publication
		pub.ProjectID = firstNonBlank(pub.ProjectID, updated.Project.ID, updated.Index.ProjectID)
		pub.RevisionID = firstNonBlank(updated.Revision.ID, updated.Index.RevisionID, pub.RevisionID)
		pub.Path = strings.TrimSpace(r.FormValue("path"))
		pub.Public = true
		if pub.ProjectID == "" || pub.RevisionID == "" {
			http.Error(w, "publication project and revision are required", http.StatusBadRequest)
			return
		}
		if s.store != nil {
			if err := s.store.SavePublication(r.Context(), pub); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		updated.Publication = pub
		s.specs[specIndex] = updated
	}
	http.Redirect(w, r, "/manage", http.StatusSeeOther)
}

func (s *managementServer) managedSpecIndex(specID string) (int, error) {
	if len(s.specs) == 0 {
		return 0, errors.New("managed spec is required")
	}
	if specID == "" {
		return 0, nil
	}
	for i, spec := range s.specs {
		if spec.ID == specID {
			return i, nil
		}
	}
	return 0, errors.New("managed spec not found")
}

func normalizeManagedSpecs(idx core.SpecIndex, opts ManagementOptions) []ManagedSpec {
	specs := opts.Specs
	if len(specs) == 0 {
		specs = []ManagedSpec{{
			Index:       idx,
			Project:     opts.Project,
			Source:      opts.Source,
			Revision:    opts.Revision,
			Candidates:  opts.Candidates,
			Publication: opts.Publication,
			SyncRecord:  opts.SyncRecord,
		}}
	}

	normalized := make([]ManagedSpec, 0, len(specs))
	for _, spec := range specs {
		if spec.Index.ProjectID == "" {
			spec.Index.ProjectID = firstNonBlank(spec.Project.ID, idx.ProjectID)
		}
		if spec.Index.RevisionID == "" {
			spec.Index.RevisionID = firstNonBlank(spec.Revision.ID, idx.RevisionID)
		}
		if spec.Project.ID == "" {
			spec.Project.ID = spec.Index.ProjectID
		}
		if spec.Source.ProjectID == "" {
			spec.Source.ProjectID = spec.Project.ID
		}
		if spec.Revision.ID == "" {
			spec.Revision.ID = spec.Index.RevisionID
		}
		if spec.Revision.SourceID == "" {
			spec.Revision.SourceID = spec.Source.ID
		}
		if spec.Publication.ProjectID == "" {
			spec.Publication.ProjectID = spec.Project.ID
		}
		if spec.Publication.RevisionID == "" {
			spec.Publication.RevisionID = spec.Revision.ID
		}
		if spec.SyncRecord.ProjectID == "" {
			spec.SyncRecord.ProjectID = spec.Project.ID
		}
		if spec.SyncRecord.SourceID == "" {
			spec.SyncRecord.SourceID = spec.Source.ID
		}
		if spec.SyncRecord.RevisionID == "" {
			spec.SyncRecord.RevisionID = spec.Revision.ID
		}
		if spec.ID == "" {
			spec.ID = managedSpecID(spec)
		}
		normalized = append(normalized, spec)
	}
	return normalized
}

func cloneManagedSpecs(specs []ManagedSpec) []ManagedSpec {
	cloned := make([]ManagedSpec, len(specs))
	copy(cloned, specs)
	return cloned
}

func managementOverviewModel(specs []ManagedSpec) templates.ManagementOverviewModel {
	model := templates.ManagementOverviewModel{
		Specs: make([]templates.ManagedSpecModel, 0, len(specs)),
	}
	for _, spec := range specs {
		model.Specs = append(model.Specs, templates.ManagedSpecModel{
			ID:          spec.ID,
			Title:       managementSpecTitle(spec),
			Version:     managementSpecVersion(spec),
			Project:     spec.Project,
			Source:      spec.Source,
			Revision:    spec.Revision,
			Candidates:  spec.Candidates,
			Publication: spec.Publication,
			SyncRecord:  spec.SyncRecord,
		})
	}
	if len(model.Specs) > 0 {
		selected := model.Specs[0]
		model.Project = selected.Project
		model.Source = selected.Source
		model.Revision = selected.Revision
		model.Publication = selected.Publication
		model.SyncRecord = selected.SyncRecord
	}
	return model
}

func managementRefAllowed(candidates []core.RevisionCandidate, ref string) bool {
	for _, candidate := range candidates {
		if candidate.Ref == ref {
			return true
		}
	}
	return false
}

func managedSpecID(spec ManagedSpec) string {
	parts := compactManagedSpecIDParts(
		spec.Project.ID,
		spec.Source.ID,
		spec.Revision.ID,
		spec.Index.ProjectID,
		spec.Index.RevisionID,
	)
	if len(parts) == 0 {
		return "current"
	}
	return strings.Join(parts, "-")
}

func managementSpecTitle(spec ManagedSpec) string {
	if strings.TrimSpace(spec.Index.Title) != "" {
		return spec.Index.Title
	}
	if strings.TrimSpace(spec.Project.Name) != "" {
		return spec.Project.Name
	}
	return "Untitled API"
}

func managementSpecVersion(spec ManagedSpec) string {
	if strings.TrimSpace(spec.Index.Version) != "" {
		return spec.Index.Version
	}
	if strings.TrimSpace(spec.Revision.Version) != "" {
		return spec.Revision.Version
	}
	return "unversioned"
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func compactManagedSpecIDParts(values ...string) []string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			continue
		}
		var b strings.Builder
		lastDash := false
		for _, r := range value {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				b.WriteRune(r)
				lastDash = false
				continue
			}
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
		part := strings.Trim(b.String(), "-")
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}
