package core

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type SyncRequest struct {
	ProjectID string
	SourceID  string
	Trigger   string
}

type SyncResult struct {
	Spec     SpecFile
	Revision Revision
	Index    SpecIndex
	Record   SyncRecord
}

type Syncer struct {
	Source SourceFetcher
	Parser Parser
	Store  Store
	Blobs  BlobStore
	Cache  Cache
	Now    func() time.Time
}

func (s Syncer) Sync(ctx context.Context, req SyncRequest) (SyncResult, error) {
	if s.Source == nil {
		return SyncResult{}, errors.New("sync source is required")
	}
	if s.Parser == nil {
		return SyncResult{}, errors.New("sync parser is required")
	}
	if s.Store == nil {
		return SyncResult{}, errors.New("sync store is required")
	}
	if s.Blobs == nil {
		return SyncResult{}, errors.New("sync blob store is required")
	}
	now := s.now()
	record := SyncRecord{
		ID:        syncRecordID(now, req),
		ProjectID: req.ProjectID,
		SourceID:  req.SourceID,
		Trigger:   firstNonBlank(req.Trigger, "manual"),
		StartedAt: now,
	}

	spec, rev, err := s.Source.Fetch(ctx)
	record = syncRecordWithSpec(record, spec, rev)
	if record.SourceID == "" {
		record.SourceID = req.SourceID
	}
	if err != nil {
		return s.fail(ctx, record, err)
	}
	if rev.SourceID == "" {
		rev.SourceID = firstNonBlank(spec.SourceID, req.SourceID)
	}
	if spec.SourceID == "" {
		spec.SourceID = firstNonBlank(rev.SourceID, req.SourceID)
	}

	idx, err := s.Parser.Parse(ctx, spec, rev)
	if err != nil {
		record = syncRecordWithSpec(record, spec, rev)
		return s.fail(ctx, record, err)
	}
	idx.ProjectID = req.ProjectID
	idx.RevisionID = rev.ID

	if err := s.Blobs.Put(ctx, SpecBlobKey(rev, spec), spec.Bytes); err != nil {
		record = syncRecordWithSpec(record, spec, rev)
		return s.fail(ctx, record, err)
	}
	if err := s.Store.SaveRevision(ctx, rev); err != nil {
		record = syncRecordWithSpec(record, spec, rev)
		return s.fail(ctx, record, err)
	}
	if s.Cache != nil {
		s.Cache.Delete("public:" + req.ProjectID)
		s.Cache.Delete("search:" + req.ProjectID + ":" + rev.ID)
	}

	record = syncRecordWithSpec(record, spec, rev)
	record.Result = SyncResultSuccess
	record.FinishedAt = s.now()
	if err := s.Store.SaveSyncRecord(ctx, record); err != nil {
		return SyncResult{}, err
	}
	return SyncResult{Spec: spec, Revision: rev, Index: idx, Record: record}, nil
}

func (s Syncer) fail(ctx context.Context, record SyncRecord, cause error) (SyncResult, error) {
	record.Result = SyncResultFailure
	record.ErrorSummary = errorSummary(cause)
	record.FinishedAt = s.now()
	if err := s.Store.SaveSyncRecord(ctx, record); err != nil {
		return SyncResult{}, errors.Join(cause, err)
	}
	return SyncResult{}, cause
}

func (s Syncer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func syncRecordWithSpec(r SyncRecord, spec SpecFile, rev Revision) SyncRecord {
	r.SpecPath = spec.Path
	r.RevisionID = rev.ID
	r.Ref = rev.Ref
	r.CommitSHA = rev.CommitSHA
	r.SourceID = firstNonBlank(r.SourceID, rev.SourceID, spec.SourceID)
	return r
}

func syncRecordID(now time.Time, req SyncRequest) string {
	parts := []string{
		"sync",
		fmt.Sprintf("%d", now.UnixNano()),
		safeIDPart(req.ProjectID),
		safeIDPart(req.SourceID),
		safeIDPart(req.Trigger),
	}
	return strings.Trim(strings.Join(parts, "-"), "-")
}

func SpecBlobKey(rev Revision, spec SpecFile) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(spec.Path)), ".")
	if ext == "" {
		ext = strings.ToLower(spec.Format)
	}
	if ext == "" {
		ext = "yaml"
	}
	return "specs/" + rev.ID + "." + ext
}

func errorSummary(err error) string {
	if err == nil {
		return ""
	}
	summary := strings.TrimSpace(err.Error())
	if len(summary) > 512 {
		return summary[:512]
	}
	return summary
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func safeIDPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
