package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	core "github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/web/templates"
)

const managementSyncTimeout = 30 * time.Second

type ManagementStore interface {
	SavePublication(context.Context, core.Publication) error
}

type ManagementSyncAction func(context.Context, ManagedSpec, string) (ManagedSpec, error)
type ManagementPublishedIndexLoader func(context.Context, ManagedSpec) (core.SpecIndex, bool, error)

type ManagementOptions struct {
	Store                ManagementStore
	SyncAction           ManagementSyncAction
	PublishedIndexLoader ManagementPublishedIndexLoader
	Specs                []ManagedSpec
	Project              core.Project
	Source               core.Source
	Revision             core.Revision
	Candidates           []core.RevisionCandidate
	Publication          core.Publication
	PublishedIndex       core.SpecIndex
	SyncRecord           core.SyncRecord
}

type ManagedSpec struct {
	ID             string
	Index          core.SpecIndex
	PublishedIndex core.SpecIndex
	Project        core.Project
	Source         core.Source
	Revision       core.Revision
	Candidates     []core.RevisionCandidate
	Publication    core.Publication
	SyncRecord     core.SyncRecord
}

func NewManagementServer(idx core.SpecIndex, opts ManagementOptions) http.Handler {
	srv := &managementServer{
		store:                opts.Store,
		syncAction:           opts.SyncAction,
		publishedIndexLoader: opts.PublishedIndexLoader,
		specs:                normalizeManagedSpecs(idx, opts),
		completedMutations:   make(map[string]managementCompletedMutation),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/manage", srv.overview)
	mux.HandleFunc("/manage/specs", srv.specsOverview)
	mux.HandleFunc("/manage/spec/", srv.specDetail)
	mux.HandleFunc("/manage/publication", srv.updatePublication)
	mux.HandleFunc("/manage/sync", srv.syncRef)
	mux.HandleFunc("/manage/", srv.unknownRoute)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})
}

type managementServer struct {
	store                ManagementStore
	syncAction           ManagementSyncAction
	publishedIndexLoader ManagementPublishedIndexLoader
	specs                []ManagedSpec
	completedMutations   map[string]managementCompletedMutation
	mu                   sync.RWMutex
}

type managementCompletedMutation struct {
	RequestID          string
	PayloadFingerprint string
}

func (s *managementServer) unknownRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	specs := cloneManagedSpecs(s.specs)
	s.mu.RUnlock()
	model := s.managementOverviewModel(r.Context(), specs, "")
	component := templates.ManagementUnknownPage(model, r.URL.Path)
	isFragment := managementWantsFragment(r)
	if isFragment {
		component = templates.ManagementUnknownContent(r.URL.Path)
	}
	var body bytes.Buffer
	if err := component.Render(r.Context(), &body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if isFragment {
		w.Header().Set("X-Manja-Application-Status", "not-found")
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
	_, _ = w.Write(body.Bytes())
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

	model := s.managementOverviewModel(r.Context(), specs, "")
	component := templates.ManagementOverview(model)
	if managementWantsFragment(r) {
		component = templates.ManagementOverviewContent(model)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *managementServer) specsOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	specs := cloneManagedSpecs(s.specs)
	s.mu.RUnlock()

	model := s.managementOverviewModel(r.Context(), specs, "")
	model.SpecFilter = strings.TrimSpace(r.URL.Query().Get("q"))
	model.SpecStatus = normalizedManagementSpecStatus(r.URL.Query().Get("status"))
	model.FilteredSpecs = filterManagementSpecModels(model.Specs, model.SpecFilter, model.SpecStatus)
	component := templates.ManagementSpecsPage(model)
	if managementWantsFragment(r) {
		component = templates.ManagementSpecsContent(model)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *managementServer) specDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	specID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/manage/spec/"), "/")
	if specID == "" {
		s.unknownRoute(w, r)
		return
	}

	s.mu.RLock()
	specs := cloneManagedSpecs(s.specs)
	s.mu.RUnlock()
	model := s.managementOverviewModel(r.Context(), specs, specID)
	component := templates.ManagementSpecPage(model)
	isFragment := managementWantsFragment(r)
	if isFragment {
		component = templates.ManagementSpecContent(model)
	}
	var body bytes.Buffer
	if err := component.Render(r.Context(), &body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, ok := managedSpecByID(specs, specID); !ok {
		if isFragment {
			w.Header().Set("X-Manja-Application-Status", "not-found")
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}
	_, _ = w.Write(body.Bytes())
}

func normalizedManagementSpecStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "public":
		return "public"
	case "private":
		return "private"
	default:
		return ""
	}
}

func filterManagementSpecModels(specs []templates.ManagedSpecModel, query string, status string) []templates.ManagedSpecModel {
	query = strings.ToLower(strings.TrimSpace(query))
	filtered := make([]templates.ManagedSpecModel, 0, len(specs))
	for _, spec := range specs {
		if status == "public" && !spec.Publication.Public {
			continue
		}
		if status == "private" && spec.Publication.Public {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(strings.Join([]string{
				spec.ID,
				spec.Title,
				spec.Version,
				spec.Project.ID,
				spec.Project.Name,
				spec.Source.ID,
				spec.Source.Kind,
				spec.Source.SpecPath,
				spec.Revision.ID,
				spec.Revision.Ref,
				spec.Revision.CommitSHA,
			}, " "))
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		filtered = append(filtered, spec)
	}
	return filtered
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
			s.respondManagementApplicationError(w, r, cloneManagedSpecs(s.specs), specID, "validation-error", "managed spec not found", core.Publication{}, "Choose an available contract")
			return
		}
	}
	if len(s.specs) == 0 {
		s.respondManagementApplicationError(w, r, nil, specID, "validation-error", "managed spec is required", core.Publication{}, "Choose an available contract")
		return
	}

	spec := s.specs[specIndex]
	requestID := managementMutationRequestID(r)
	mutationSlot := "publication:" + spec.ID
	pub := spec.Publication
	pub.ProjectID = firstNonBlank(pub.ProjectID, spec.Project.ID, spec.Index.ProjectID)
	if revisionID := strings.TrimSpace(r.FormValue("revision_id")); revisionID != "" {
		pub.RevisionID = revisionID
	} else {
		pub.RevisionID = firstNonBlank(pub.RevisionID, spec.Revision.ID, spec.Index.RevisionID)
	}
	pub.Path = strings.TrimSpace(r.FormValue("path"))
	pub.Public = strings.TrimSpace(r.FormValue("visibility")) == "public"
	payloadFingerprint := managementMutationPayloadFingerprint("publication", r)
	replay, conflict := s.completedMutationStatus(mutationSlot, requestID, payloadFingerprint)
	if conflict {
		s.respondManagementApplicationError(w, r, cloneManagedSpecs(s.specs), spec.ID, "validation-error", "request token does not match the submitted publication values", pub, "Reload this contract before retrying publication")
		return
	}
	if replay {
		s.respondManagementMutation(w, r, cloneManagedSpecs(s.specs), spec.ID)
		return
	}
	if pub.ProjectID == "" || pub.RevisionID == "" {
		s.respondManagementApplicationError(w, r, cloneManagedSpecs(s.specs), spec.ID, "validation-error", "publication project and revision are required", pub, "Retry publication")
		return
	}
	if s.store != nil {
		if err := s.store.SavePublication(r.Context(), pub); err != nil {
			s.respondManagementApplicationError(w, r, cloneManagedSpecs(s.specs), spec.ID, "persistence-error", err.Error(), pub, "Retry publication")
			return
		}
	}
	s.specs[specIndex].Publication = pub
	if requestID != "" {
		s.completedMutations[mutationSlot] = managementCompletedMutation{RequestID: requestID, PayloadFingerprint: payloadFingerprint}
	}
	s.respondManagementMutation(w, r, cloneManagedSpecs(s.specs), s.specs[specIndex].ID)
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
		http.Error(w, "sync action is not configured", http.StatusInternalServerError)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	specIndex, err := s.managedSpecIndex(strings.TrimSpace(r.FormValue("spec_id")))
	if err != nil {
		s.respondManagementApplicationError(w, r, cloneManagedSpecs(s.specs), strings.TrimSpace(r.FormValue("spec_id")), "validation-error", err.Error(), core.Publication{}, "Retry sync")
		return
	}
	ref := strings.TrimSpace(r.FormValue("ref"))
	if ref == "" {
		s.respondManagementApplicationError(w, r, cloneManagedSpecs(s.specs), s.specs[specIndex].ID, "validation-error", "ref is required", core.Publication{}, "Retry sync")
		return
	}
	spec := s.specs[specIndex]
	if !managementRefAllowed(spec.Candidates, ref) {
		s.respondManagementApplicationError(w, r, cloneManagedSpecs(s.specs), spec.ID, "validation-error", "ref is not available for this source", core.Publication{}, "Retry sync")
		return
	}

	requestID := managementMutationRequestID(r)
	payloadFingerprint := managementMutationPayloadFingerprint("sync", r)
	syncSlot := "sync:" + spec.ID
	replay, conflict := s.completedMutationStatus(syncSlot, requestID, payloadFingerprint)
	if conflict {
		s.respondManagementApplicationError(w, r, cloneManagedSpecs(s.specs), spec.ID, "validation-error", "request token does not match the submitted sync values", core.Publication{}, "Reload this contract before retrying sync")
		return
	}
	updated := spec
	if !replay {
		syncCtx, cancel := context.WithTimeout(r.Context(), managementSyncTimeout)
		defer cancel()
		updated, err = s.syncAction(syncCtx, spec, ref)
		if err != nil {
			s.respondManagementApplicationError(w, r, cloneManagedSpecs(s.specs), spec.ID, "sync-error", err.Error(), core.Publication{}, "Retry sync")
			return
		}
		updated = normalizeManagedSpecs(core.SpecIndex{}, ManagementOptions{Specs: []ManagedSpec{updated}})[0]
		if len(updated.Candidates) == 0 {
			updated.Candidates = spec.Candidates
		}
		s.specs[specIndex] = updated
		if requestID != "" {
			s.completedMutations[syncSlot] = managementCompletedMutation{RequestID: requestID, PayloadFingerprint: payloadFingerprint}
		}
	}
	if strings.TrimSpace(r.FormValue("publish")) == "public" {
		publicationSlot := "sync-publication:" + spec.ID
		publicationReplay, publicationConflict := s.completedMutationStatus(publicationSlot, requestID, payloadFingerprint)
		if publicationConflict {
			s.respondManagementApplicationError(w, r, cloneManagedSpecs(s.specs), spec.ID, "validation-error", "request token does not match the submitted sync values", core.Publication{}, "Reload this contract before retrying sync")
			return
		}
		if publicationReplay {
			s.respondManagementMutation(w, r, cloneManagedSpecs(s.specs), s.specs[specIndex].ID)
			return
		}
		pub := updated.Publication
		pub.ProjectID = firstNonBlank(pub.ProjectID, updated.Project.ID, updated.Index.ProjectID)
		pub.RevisionID = firstNonBlank(updated.Revision.ID, updated.Index.RevisionID, pub.RevisionID)
		pub.Path = strings.TrimSpace(r.FormValue("path"))
		pub.Public = true
		if pub.ProjectID == "" || pub.RevisionID == "" {
			s.respondManagementApplicationError(w, r, cloneManagedSpecs(s.specs), spec.ID, "validation-error", "publication project and revision are required", pub, "Retry publication")
			return
		}
		if s.store != nil {
			if err := s.store.SavePublication(r.Context(), pub); err != nil {
				s.respondManagementApplicationError(w, r, cloneManagedSpecs(s.specs), spec.ID, "persistence-error", err.Error(), pub, "Retry publication")
				return
			}
		}
		updated.Publication = pub
		s.specs[specIndex] = updated
		if requestID != "" {
			s.completedMutations[publicationSlot] = managementCompletedMutation{RequestID: requestID, PayloadFingerprint: payloadFingerprint}
		}
	}
	s.respondManagementMutation(w, r, cloneManagedSpecs(s.specs), s.specs[specIndex].ID)
}

// Expected application validation and recovery renders HTML with HTTP 200 and
// X-Manja-Application-Status. Unexpected transport and server failures remain non-2xx.
func (s *managementServer) respondManagementApplicationError(w http.ResponseWriter, r *http.Request, specs []ManagedSpec, selectedSpecID string, status string, message string, enteredPublication core.Publication, retryAction string) {
	model := s.managementOverviewModel(r.Context(), specs, selectedSpecID)
	model.ApplicationStatus = status
	model.ApplicationError = message
	model.EnteredRef = strings.TrimSpace(r.FormValue("ref"))
	model.RetryAction = retryAction
	if enteredPublication != (core.Publication{}) {
		model.Publication = enteredPublication
		for i := range model.Specs {
			if model.Specs[i].ID == selectedSpecID {
				model.Specs[i].Publication = enteredPublication
			}
		}
	}
	component := templates.ManagementSpecPage(model)
	if managementWantsFragment(r) {
		component = templates.ManagementSpecContent(model)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Manja-Application-Status", status)
	if err := component.Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func managementMutationRequestID(r *http.Request) string {
	requestID := strings.TrimSpace(r.FormValue("request_id"))
	if len(requestID) > 256 {
		return ""
	}
	return requestID
}

func managementMutationPayloadFingerprint(action string, r *http.Request) string {
	values := []string{action, strings.TrimSpace(r.FormValue("spec_id"))}
	switch action {
	case "publication":
		values = append(values,
			strings.TrimSpace(r.FormValue("revision_id")),
			strings.TrimSpace(r.FormValue("visibility")),
			strings.TrimSpace(r.FormValue("path")),
		)
	case "sync":
		values = append(values,
			strings.TrimSpace(r.FormValue("ref")),
			strings.TrimSpace(r.FormValue("publish")),
			strings.TrimSpace(r.FormValue("path")),
		)
	}
	sum := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return fmt.Sprintf("%x", sum)
}

func (s *managementServer) completedMutationStatus(slot string, requestID string, payloadFingerprint string) (replay bool, conflict bool) {
	if requestID == "" {
		return false, false
	}
	completed, ok := s.completedMutations[slot]
	if !ok || completed.RequestID != requestID {
		return false, false
	}
	if completed.PayloadFingerprint != payloadFingerprint {
		return false, true
	}
	return true, false
}

func managementMutationReplacesCurrentURL(r *http.Request, redirectPath string) bool {
	currentURL := strings.TrimSpace(r.Header.Get("HX-Current-URL"))
	if currentURL == "" {
		return false
	}
	parsed, err := url.Parse(currentURL)
	if err != nil {
		return false
	}
	return parsed.Path == redirectPath
}

func (s *managementServer) respondManagementMutation(w http.ResponseWriter, r *http.Request, specs []ManagedSpec, selectedSpecID string) {
	redirectPath := managementSpecRedirectPath(ManagedSpec{ID: selectedSpecID})
	if managementWantsFragment(r) {
		model := s.managementOverviewModel(r.Context(), specs, selectedSpecID)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if managementMutationReplacesCurrentURL(r, redirectPath) {
			w.Header().Set("HX-Replace-Url", redirectPath)
		} else {
			w.Header().Set("HX-Push-Url", redirectPath)
		}
		if err := templates.ManagementSpecContent(model).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	http.Redirect(w, r, redirectPath, http.StatusSeeOther)
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
			Index:          idx,
			PublishedIndex: opts.PublishedIndex,
			Project:        opts.Project,
			Source:         opts.Source,
			Revision:       opts.Revision,
			Candidates:     opts.Candidates,
			Publication:    opts.Publication,
			SyncRecord:     opts.SyncRecord,
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

func (s *managementServer) managementOverviewModel(ctx context.Context, specs []ManagedSpec, selectedSpecID string) templates.ManagementOverviewModel {
	model := templates.ManagementOverviewModel{
		Specs:          make([]templates.ManagedSpecModel, 0, len(specs)),
		SelectedSpecID: selectedSpecID,
	}
	for _, spec := range specs {
		diff, diffAvailable, diffMessage := s.managementSpecDiff(ctx, spec)
		model.Specs = append(model.Specs, templates.ManagedSpecModel{
			ID:            spec.ID,
			Title:         managementSpecTitle(spec),
			Version:       managementSpecVersion(spec),
			Operations:    len(spec.Index.Operations),
			Schemas:       len(spec.Index.Schemas),
			Routes:        len(spec.Index.PublicRoutes),
			Diff:          diff,
			DiffAvailable: diffAvailable,
			DiffMessage:   diffMessage,
			Project:       spec.Project,
			Source:        spec.Source,
			Revision:      spec.Revision,
			Candidates:    spec.Candidates,
			Publication:   spec.Publication,
			SyncRecord:    spec.SyncRecord,
		})
	}
	if len(model.Specs) > 0 {
		selected := model.Specs[0]
		if selectedSpecID != "" {
			found := false
			for _, spec := range model.Specs {
				if spec.ID == selectedSpecID {
					selected = spec
					found = true
					break
				}
			}
			if !found {
				return model
			}
		}
		model.SelectedSpecID = selected.ID
		model.Project = selected.Project
		model.Source = selected.Source
		model.Revision = selected.Revision
		model.Publication = selected.Publication
		model.SyncRecord = selected.SyncRecord
	}
	return model
}

func (s *managementServer) managementSpecDiff(ctx context.Context, spec ManagedSpec) (core.SpecDiff, bool, string) {
	if !spec.Publication.Public || strings.TrimSpace(spec.Publication.RevisionID) == "" {
		return core.SpecDiff{}, false, "Publish once to create a production baseline for contract checks."
	}
	baseline, ok, err := s.managementBaselineIndex(ctx, spec)
	if err != nil {
		return core.SpecDiff{}, false, "Could not load the production baseline: " + err.Error()
	}
	if !ok {
		return core.SpecDiff{}, false, "Production baseline is not available for this revision yet."
	}
	diff, err := core.DiffSpecIndexes(baseline, spec.Index)
	if err != nil {
		return core.SpecDiff{}, false, "Could not compare the production baseline: " + err.Error()
	}
	return diff, true, ""
}

func (s *managementServer) managementBaselineIndex(ctx context.Context, spec ManagedSpec) (core.SpecIndex, bool, error) {
	if hasManagementSpecIndex(spec.PublishedIndex) &&
		managementIndexMatchesPublication(spec, spec.PublishedIndex) {
		return spec.PublishedIndex, true, nil
	}
	if managementIndexMatchesPublication(spec, spec.Index) &&
		spec.Revision.ID == spec.Publication.RevisionID &&
		spec.Revision.ContractID == spec.Publication.ProjectID {
		return spec.Index, true, nil
	}
	if s.publishedIndexLoader == nil {
		return core.SpecIndex{}, false, nil
	}
	return s.publishedIndexLoader(ctx, spec)
}

func managementIndexMatchesPublication(spec ManagedSpec, index core.SpecIndex) bool {
	return spec.Project.ID != "" &&
		spec.Publication.ProjectID == spec.Project.ID &&
		spec.Publication.RevisionID != "" &&
		index.ProjectID == spec.Publication.ProjectID &&
		index.RevisionID == spec.Publication.RevisionID
}

func hasManagementSpecIndex(idx core.SpecIndex) bool {
	return strings.TrimSpace(idx.RevisionID) != "" ||
		strings.TrimSpace(idx.Title) != "" ||
		len(idx.Operations) > 0 ||
		len(idx.Schemas) > 0 ||
		len(idx.PublicRoutes) > 0
}

func managedSpecByID(specs []ManagedSpec, id string) (ManagedSpec, bool) {
	for _, spec := range specs {
		if spec.ID == id {
			return spec, true
		}
	}
	return ManagedSpec{}, false
}

func managementSpecRedirectPath(spec ManagedSpec) string {
	if strings.TrimSpace(spec.ID) == "" {
		return "/manage"
	}
	return "/manage/spec/" + url.PathEscape(spec.ID)
}

func managementWantsFragment(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("HX-Request"), "true") &&
		!strings.EqualFold(r.Header.Get("HX-Boosted"), "true") &&
		!strings.EqualFold(strings.TrimSpace(r.Header.Get("HX-Target")), "body") &&
		!strings.EqualFold(r.Header.Get("HX-History-Restore-Request"), "true")
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
	seen := map[string]bool{}
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
		if part != "" && !seen[part] {
			parts = append(parts, part)
			seen[part] = true
		}
	}
	return parts
}
