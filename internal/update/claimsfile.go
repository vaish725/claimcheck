package update

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/vaish725/claimcheck/internal/rewrite"
)

// locateDeclaredSpans decodes raw claims.yaml into a yaml.Node tree to find
// exactly where each claim's "declared" scalar sits in the original text,
// via the node's Line/Column - no re-encoding needed, so every other byte
// (comments, key order, indentation) stays untouched.
//
// Declared values must be bare (unquoted) YAML numbers, so a span's byte
// length equals len(node.Value) with no unescaping required.
func locateDeclaredSpans(raw []byte) ([]rewrite.Span, error) {
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
		declaredNode := findMapValue(claimNode, "declared")
		if declaredNode == nil {
			return nil, fmt.Errorf("claim %q: has no declared field", idNode.Value)
		}

		start, err := byteOffset(raw, lineStarts, declaredNode.Line, declaredNode.Column)
		if err != nil {
			return nil, fmt.Errorf("claim %q: locating declared value: %w", idNode.Value, err)
		}
		end := start + len(declaredNode.Value)

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
