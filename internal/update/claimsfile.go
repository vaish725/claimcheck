package update

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/vaish725/claimcheck/internal/rewrite"
)

// locateFieldSpans decodes raw claims.yaml into a yaml.Node tree to find
// exactly where each claim's field scalar sits in the original text, via
// the node's Line/Column - no re-encoding needed, so every other byte
// (comments, key order, indentation) stays untouched. If required, a claim
// missing field is an error; otherwise that claim is just skipped (e.g.
// "machine" is opt-in - a claim without it simply never gets a span).
//
// Field values must be bare (unquoted) YAML scalars: a quoted scalar's
// Column points at the opening quote while Value excludes the quotes, so
// naively splicing [Column, Column+len(Value)) would land inside the
// string, not around it. Bare scalars have no such mismatch (confirmed
// empirically against yaml.v3), so a quoted or block scalar is rejected
// with a clear error instead of risking silent corruption.
func locateFieldSpans(raw []byte, field string, required bool) ([]rewrite.Span, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing claims.yaml structure: %w", err)
	}
	if len(doc.Content) == 0 {
		return nil, fmt.Errorf("claims.yaml: empty document")
	}

	claimsSeq := findMapValue(doc.Content[0], "claims")
	if claimsSeq == nil || claimsSeq.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf(`claims.yaml: no "claims" sequence found`)
	}

	lineStarts := computeLineStartOffsets(raw)

	spans := make([]rewrite.Span, 0, len(claimsSeq.Content))
	for _, claimNode := range claimsSeq.Content {
		idNode := findMapValue(claimNode, "id")
		if idNode == nil {
			return nil, fmt.Errorf("claims.yaml: a claim near line %d has no id", claimNode.Line)
		}
		fieldNode := findMapValue(claimNode, field)
		if fieldNode == nil {
			if required {
				return nil, fmt.Errorf("claim %q: has no %s field", idNode.Value, field)
			}
			continue
		}
		if fieldNode.Style != 0 {
			return nil, fmt.Errorf("claim %q: %s must be a bare (unquoted) value, not a quoted or block scalar", idNode.Value, field)
		}

		start, err := byteOffset(raw, lineStarts, fieldNode.Line, fieldNode.Column)
		if err != nil {
			return nil, fmt.Errorf("claim %q: locating %s value: %w", idNode.Value, field, err)
		}
		end := start + len(fieldNode.Value)

		spans = append(spans, rewrite.Span{ID: idNode.Value, Start: start, End: end})
	}

	return spans, nil
}

// findMapValue returns key's value node from a mapping's flat
// [key1, value1, ...] Content slice, or nil if absent.
func findMapValue(mapNode *yaml.Node, key string) *yaml.Node {
	if mapNode == nil || mapNode.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapNode.Content); i += 2 {
		if mapNode.Content[i].Value == key {
			return mapNode.Content[i+1]
		}
	}
	return nil
}

// computeLineStartOffsets maps each 1-indexed line number to its byte
// offset in raw; index 0 is unused.
func computeLineStartOffsets(raw []byte) []int {
	starts := []int{0, 0} // line 1 starts at offset 0
	for i, b := range raw {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// byteOffset converts a yaml.Node's 1-indexed (line, column) - column is a
// rune count from line start, per yaml.v3 - into a byte offset into raw.
func byteOffset(raw []byte, lineStarts []int, line, column int) (int, error) {
	if line <= 0 || line >= len(lineStarts) {
		return 0, fmt.Errorf("line %d out of range", line)
	}
	lineStart := lineStarts[line]
	lineEnd := len(raw)
	if line+1 < len(lineStarts) {
		lineEnd = lineStarts[line+1]
	}
	lineText := string(raw[lineStart:lineEnd])

	runeCount := 0
	for byteIdx := range lineText {
		if runeCount == column-1 {
			return lineStart + byteIdx, nil
		}
		runeCount++
	}
	if runeCount == column-1 {
		return lineStart + len(lineText), nil
	}
	return 0, fmt.Errorf("column %d out of range on line %d", column, line)
}
