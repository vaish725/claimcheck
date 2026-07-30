// Package rewrite edits marked spans inside text files without disturbing
// surrounding bytes, and writes the result back atomically.
package rewrite

import (
	"fmt"
	"regexp"
	"sort"
)

// Marker is one <!-- claimcheck:ID --> ... <!-- /claimcheck:ID --> span.
// ContentStart/ContentEnd are byte offsets of the text between the tags,
// excluding the tags themselves.
type Marker struct {
	ID           string
	ContentStart int
	ContentEnd   int
}

// tokenPattern matches both open and close tags in one pass. Group 1 is
// "/" for a close tag, empty for an open tag; group 2 is the marker id.
var tokenPattern = regexp.MustCompile(`<!--\s*(/?)claimcheck:([A-Za-z0-9_-]+)\s*-->`)

// openMarker tracks a tag awaiting its close, so errors can point back at
// where the still-open marker started.
type openMarker struct {
	id           string
	contentStart int
	tagStart     int
}

// Parse scans content for well-formed marker pairs, in order. It errors on
// the first malformed marker - unterminated, nested, or a
// mismatched/orphaned close - rather than guessing: mangling a file is
// worse than refusing to touch it.
func Parse(content []byte) ([]Marker, error) {
	matches := tokenPattern.FindAllSubmatchIndex(content, -1)

	var markers []Marker
	var open *openMarker

	for _, m := range matches {
		tagStart := m[0]
		isClose := m[3] > m[2] // group 1 ("/") matched a non-empty span
		id := string(content[m[4]:m[5]])

		if isClose {
			if open == nil {
				return nil, fmt.Errorf("marker parse: closing tag for %q at byte %d has no matching open tag", id, tagStart)
			}
			if open.id != id {
				return nil, fmt.Errorf("marker parse: closing tag for %q at byte %d does not match open marker %q opened at byte %d", id, tagStart, open.id, open.tagStart)
			}
			markers = append(markers, Marker{ID: id, ContentStart: open.contentStart, ContentEnd: tagStart})
			open = nil
			continue
		}

		if open != nil {
			return nil, fmt.Errorf("marker parse: marker %q at byte %d is nested inside still-open marker %q opened at byte %d", id, tagStart, open.id, open.tagStart)
		}
		open = &openMarker{id: id, contentStart: m[1], tagStart: tagStart}
	}

	if open != nil {
		return nil, fmt.Errorf("marker parse: marker %q opened at byte %d is never closed", open.id, open.tagStart)
	}

	return markers, nil
}

// Replace returns new content with each marker's span replaced by
// values[marker.ID]. A marker whose id has no entry in values is left
// exactly as it was. Returns an error under the same conditions as Parse.
func Replace(content []byte, values map[string]string) ([]byte, error) {
	markers, err := Parse(content)
	if err != nil {
		return nil, err
	}

	spans := make([]Span, len(markers))
	for i, mk := range markers {
		spans[i] = Span{ID: mk.ID, Start: mk.ContentStart, End: mk.ContentEnd}
	}

	return ReplaceSpans(content, spans, values)
}

// Span is a byte range [Start, End) in some content, identified by ID.
// Markers are one way to find spans; other callers (e.g. update, locating
// claims.yaml scalars via a YAML node walk) find them differently but
// share this same splice primitive.
type Span struct {
	ID         string
	Start, End int
}

// ReplaceSpans returns new content with each span's bytes replaced by
// values[span.ID]. A span whose id has no entry in values is left exactly
// as it was. Spans do not need to be given in order, but must not overlap;
// ReplaceSpans sorts them by Start and returns an error if any overlap.
func ReplaceSpans(content []byte, spans []Span, values map[string]string) ([]byte, error) {
	sorted := make([]Span, len(spans))
	copy(sorted, spans)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start < sorted[j].Start })

	for i := 1; i < len(sorted); i++ {
		if sorted[i].Start < sorted[i-1].End {
			return nil, fmt.Errorf("replace spans: span %q [%d,%d) overlaps span %q [%d,%d)",
				sorted[i-1].ID, sorted[i-1].Start, sorted[i-1].End, sorted[i].ID, sorted[i].Start, sorted[i].End)
		}
	}

	var out []byte
	cursor := 0
	for _, sp := range sorted {
		replacement, ok := values[sp.ID]
		if !ok {
			continue
		}
		out = append(out, content[cursor:sp.Start]...)
		out = append(out, replacement...)
		cursor = sp.End
	}
	out = append(out, content[cursor:]...)

	return out, nil
}
