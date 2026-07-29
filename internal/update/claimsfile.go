package update

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/vaish725/claimcheck/internal/rewrite"
)

// locateDeclaredSpans decodes raw claims.yaml bytes into a yaml.Node tree
// purely to find exactly where each claim's "declared" scalar sits in the
// original text - the tree's Line/Column fields point precisely at where
// a scalar's raw text begins, so no re-encoding or line-scoped searching
// is needed. Every byte outside the returned spans - comments, key order,
// indentation, quoting - is left completely alone by whatever rewrites
// these spans, the same guarantee the marker rewriter gives READMEs.
//
// Declared values must be written as bare (unquoted) YAML numbers; that is
// what every claims.yaml example and R2's extractors produce, and it's
// what makes a span's byte length equal to len(node.Value) with no
// unescaping required.
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

// findMapValue returns the value node for key in a mapping node's flat
// [key1, value1, key2, value2, ...] Content slice, or nil if the mapping
// or the key is absent.
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

// computeLineStartOffsets returns, for each 1-indexed line number, the
// byte offset in raw where that line begins. Index 0 is unused so that
// lineStarts[n] directly answers "where does line n start".
func computeLineStartOffsets(raw []byte) []int {
	starts := []int{0, 0} // line 1 starts at offset 0
	for i, b := range raw {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// byteOffset converts a yaml.Node's 1-indexed (line, column) - column
// being a rune count from the start of the line, per yaml.v3 - into a byte
// offset into raw.
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
