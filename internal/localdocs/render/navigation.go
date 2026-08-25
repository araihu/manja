package render

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

var errInvalidOperationNavigationFragment = errors.New("local docs operation-navigation fragment is invalid")

// OperationNavigationFragment holds copied, admitted previous/next operation
// links derived from one immutable catalog directory and selected detail.
type OperationNavigationFragment struct {
	data  operationNavigationData
	valid bool
}

type operationNavigationData struct {
	Group    string
	Previous *operationNavigationItemData
	Next     *operationNavigationItemData
}

type operationNavigationItemData struct {
	Title  string
	Method string
	Href   string
}

// PrepareOperationNavigation derives the bounded catalog navigation fragment
// without parsing source input or retaining aliases to the admitted directory.
func PrepareOperationNavigation(
	detail catalog.DetailRecordV1,
	operation domain.Operation,
	document catalog.DocumentDirectoryV1,
	documentHref string,
	openGroups map[string]struct{},
) (OperationNavigationFragment, error) {
	if !validDocumentHref(documentHref) || domain.ValidateCatalogDocumentKey(document.Key) != nil ||
		len(document.Operations) == 0 || uint64(len(document.Operations)) > catalog.DefaultBounds().Operations {
		return OperationNavigationFragment{}, invalidOperationNavigationField("document")
	}
	if detail.Kind != "operation" || detail.Operation == nil || detail.Schema != nil || !validDetailID(detail.ID) {
		return OperationNavigationFragment{}, invalidOperationNavigationField("operation detail")
	}
	projected := detail.Operation
	id := string(detail.ID)
	if projected.ID != id || projected.Anchor != id || projected.HeadingID != id || projected.HeadingLevel == 0 ||
		!validOperationMethod(projected.Method) || !validOperationPath(projected.Path) ||
		operation.Anchor != projected.Anchor || operation.Title != projected.Heading || operation.Method != projected.Method ||
		operation.Path != projected.Path || operation.Summary != projected.Summary || operation.Description != projected.Description ||
		operation.Deprecated != projected.Deprecated {
		return OperationNavigationFragment{}, invalidOperationNavigationField("operation identity")
	}
	projectedTags, ok := operationNavigationProjectionTags(projected.Tags)
	if !ok || !equalNavigationStrings(projectedTags, operation.Tags) {
		return OperationNavigationFragment{}, invalidOperationNavigationField("operation tags")
	}
	documentRoute := "documents/" + document.Key + "/"
	wantSelectedHref := documentRoute + "?selected=" + id + "#" + id
	if projected.Href != wantSelectedHref {
		return OperationNavigationFragment{}, invalidOperationNavigationField("operation href")
	}

	selectedIndex := -1
	seen := make(map[domain.DetailID]struct{}, len(document.Operations))
	validGroups := make(map[string]struct{}, len(document.Operations)+1)
	for index, candidate := range document.Operations {
		if !validDetailID(candidate.DetailID) {
			return OperationNavigationFragment{}, invalidOperationNavigationField("directory detail id")
		}
		if !validOperationMethod(candidate.Method) || !validOperationPath(candidate.Path) {
			return OperationNavigationFragment{}, invalidOperationNavigationField("directory route")
		}
		if !validOperationNavigationText(candidate.Title, false) || !validOperationNavigationText(candidate.OperationID, true) ||
			!validOperationNavigationText(candidate.Description, true) || !validOperationNavigationStrings(candidate.Tags) {
			return OperationNavigationFragment{}, invalidOperationNavigationField("directory text")
		}
		if !validOperationNavigationDirectoryHref(candidate.Href, document.Key, candidate.DetailID) {
			return OperationNavigationFragment{}, invalidOperationNavigationField("directory href")
		}
		if !validOperationNavigationDetailChild(candidate.DetailChild) {
			return OperationNavigationFragment{}, invalidOperationNavigationField("directory detail child")
		}
		if _, duplicate := seen[candidate.DetailID]; duplicate {
			return OperationNavigationFragment{}, invalidOperationNavigationField("duplicate operation identity")
		}
		seen[candidate.DetailID] = struct{}{}
		validGroups[operationNavigationGroupID("operations-"+OperationGroupLabel(candidate))] = struct{}{}
		if candidate.DetailID == detail.ID {
			selectedIndex = index
		}
	}
	if len(document.Schemas) > 0 {
		validGroups[operationNavigationGroupID("schemas")] = struct{}{}
	}
	for groupID := range openGroups {
		if _, valid := validGroups[groupID]; !valid {
			return OperationNavigationFragment{}, invalidOperationNavigationField("open group")
		}
	}
	if selectedIndex < 0 {
		return OperationNavigationFragment{}, invalidOperationNavigationField("selected operation")
	}
	selected := document.Operations[selectedIndex]
	if selected.OperationID != operation.ID || selected.Method != operation.Method || selected.Path != operation.Path ||
		strings.TrimSpace(selected.Title) != operationNavigationTitle(operation.Title, operation.Summary, operation.ID, operation.Method, operation.Path) ||
		!equalNavigationStrings(selected.Tags, operation.Tags) || (selected.Href != projected.Href && selected.Href != strings.TrimPrefix(projected.Href, "documents/")) {
		return OperationNavigationFragment{}, invalidOperationNavigationField("selected directory operation")
	}

	group := OperationGroupLabel(selected)
	fragment := OperationNavigationFragment{data: operationNavigationData{Group: group}, valid: true}
	for index := selectedIndex - 1; index >= 0; index-- {
		if OperationGroupLabel(document.Operations[index]) == group {
			item := prepareOperationNavigationItem(documentHref, document.Operations[index], openGroups)
			fragment.data.Previous = &item
			break
		}
	}
	for index := selectedIndex + 1; index < len(document.Operations); index++ {
		if OperationGroupLabel(document.Operations[index]) == group {
			item := prepareOperationNavigationItem(documentHref, document.Operations[index], openGroups)
			fragment.data.Next = &item
			break
		}
	}

	var output boundedBuffer
	if err := operationNavigation(fragment.data).Render(context.Background(), &output); err != nil {
		return OperationNavigationFragment{}, invalidOperationNavigationField("rendered bytes")
	}
	return fragment, nil
}

// OperationGroupLabel reproduces the catalog's stable operation grouping from
// immutable directory fields so sidebar and prepared navigation share labels.
func OperationGroupLabel(operation catalog.OperationDirectoryV1) string {
	if len(operation.Tags) > 0 {
		if label := strings.TrimSpace(operation.Tags[0]); label != "" {
			return label
		}
	}
	fallback := ""
	for _, segment := range strings.Split(strings.Trim(operation.Path, "/"), "/") {
		segment = strings.TrimSpace(segment)
		if segment == "" || strings.HasPrefix(segment, "{") {
			continue
		}
		if fallback == "" {
			fallback = segment
		}
		if segment == "api" || operationNavigationAPIVersionSegment(segment) {
			continue
		}
		return segment
	}
	if fallback != "" {
		return fallback
	}
	return "Untagged"
}

func prepareOperationNavigationItem(documentHref string, operation catalog.OperationDirectoryV1, openGroups map[string]struct{}) operationNavigationItemData {
	return operationNavigationItemData{
		Title:  operationNavigationTitle(operation.Title, operation.OperationID, operation.Method, operation.Path),
		Method: operation.Method,
		Href:   operationNavigationHref(documentHref, operation.DetailID, openGroups),
	}
}

func operationNavigationTitle(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func operationNavigationHref(documentHref string, selectedID domain.DetailID, openGroups map[string]struct{}) string {
	query := url.Values{}
	groups := make([]string, 0, len(openGroups))
	for groupID := range openGroups {
		groups = append(groups, groupID)
	}
	sort.Strings(groups)
	if len(groups) == 0 {
		query.Add("group", "")
	} else {
		for _, groupID := range groups {
			query.Add("group", groupID)
		}
	}
	query.Set("selected", string(selectedID))
	return documentHref + "?" + query.Encode() + "#" + url.PathEscape(string(selectedID))
}

func operationNavigationProjectionTags(records []projection.TextRecord) ([]string, bool) {
	values := make([]string, len(records))
	for index, record := range records {
		if record.Ordinal != uint32(index) || !validOperationNavigationText(record.ID, false) || !validOperationNavigationText(record.Value, false) {
			return nil, false
		}
		values[index] = record.Value
	}
	return values, true
}

func validOperationNavigationStrings(values []string) bool {
	for _, value := range values {
		if !validOperationNavigationText(value, false) {
			return false
		}
	}
	return true
}

func validOperationNavigationText(value string, allowEmpty bool) bool {
	return utf8.ValidString(value) && (allowEmpty || strings.TrimSpace(value) != "")
}

func validOperationNavigationDetailChild(value string) bool {
	return utf8.ValidString(value) && strings.HasPrefix(value, "details/") && !strings.Contains(value, `\`) && path.Clean(value) == value
}

func validOperationNavigationDirectoryHref(value, documentKey string, detailID domain.DetailID) bool {
	suffix := documentKey + "/?selected=" + string(detailID) + "#" + string(detailID)
	return value == suffix || value == "documents/"+suffix
}

func equalNavigationStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func operationNavigationGroupID(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "group-" + hex.EncodeToString(digest[:6])
}

func operationNavigationAPIVersionSegment(segment string) bool {
	if len(segment) < 2 || segment[0] != 'v' {
		return false
	}
	for _, character := range segment[1:] {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func OperationNavigation(fragment OperationNavigationFragment) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid {
			return errInvalidOperationNavigationFragment
		}
		var output boundedBuffer
		if err := operationNavigation(fragment.data).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func (fragment OperationNavigationFragment) Bytes(ctx context.Context) ([]byte, error) {
	var output bytes.Buffer
	if err := OperationNavigation(fragment).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func invalidOperationNavigationField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidOperationNavigationFragment, strings.TrimSpace(name))
}
