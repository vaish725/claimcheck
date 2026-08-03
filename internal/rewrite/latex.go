package rewrite

import (
	"bytes"
	"fmt"
)

// latexMacroOpen is the literal text that starts a claimcheck LaTeX
// marker. Users define `\newcommand{\claimcheck}[2]{#2}` once in their
// preamble so the macro renders only the value; the id argument is
// invisible metadata this package reads to find the value's byte span.
const latexMacroOpen = `\claimcheck{`

// ParseLaTeX scans content for \claimcheck{id}{value} calls and returns
// each value argument as a Marker (ContentStart/ContentEnd span the value
// only, not the id or the braces). LaTeX has no inline comment closer the
// way HTML does (a "%" comment runs to end of line), so a wrapper macro
// with braced arguments stands in for the marker pair Parse looks for in
// Markdown/README files.
//
// Malformed calls - an id with a stray character, an unterminated id or
// value argument, or a missing second argument - are an error rather than
// a guess, same as Parse. Scanning always resumes after a found marker's
// closing brace, so a \claimcheck{} occurring inside another one's value
// is simply never matched as its own marker - consumed as ordinary value
// text - rather than needing separate nested-marker rejection.
func ParseLaTeX(content []byte) ([]Marker, error) {
	var markers []Marker
	pos := 0

	for {
		rel := bytes.Index(content[pos:], []byte(latexMacroOpen))
		if rel == -1 {
			break
		}
		macroStart := pos + rel
		idStart := macroStart + len(latexMacroOpen)

		idEnd, err := scanLaTeXID(content, idStart)
		if err != nil {
			return nil, fmt.Errorf("latex marker at byte %d: %w", macroStart, err)
		}
		id := string(content[idStart:idEnd])

		valueOpen := idEnd + 1 // idEnd is the id's closing '}'
		if valueOpen >= len(content) || content[valueOpen] != '{' {
			return nil, fmt.Errorf("latex marker %q at byte %d: missing value argument", id, macroStart)
		}
		valueStart := valueOpen + 1

		valueEnd, err := scanLaTeXBalanced(content, valueStart)
		if err != nil {
			return nil, fmt.Errorf("latex marker %q at byte %d: %w", id, macroStart, err)
		}

		markers = append(markers, Marker{ID: id, ContentStart: valueStart, ContentEnd: valueEnd})
		pos = valueEnd + 1
	}

	return markers, nil
}

// scanLaTeXID validates and finds the end of an id argument starting at
// start, returning the byte offset of its closing '}'.
func scanLaTeXID(content []byte, start int) (end int, err error) {
	i := start
	for i < len(content) {
		b := content[i]
		if b == '}' {
			if i == start {
				return 0, fmt.Errorf("empty id argument")
			}
			return i, nil
		}
		if !isLaTeXIDByte(b) {
			return 0, fmt.Errorf("invalid character %q in id argument", b)
		}
		i++
	}
	return 0, fmt.Errorf("unterminated id argument")
}

func isLaTeXIDByte(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_' || b == '-'
}

// scanLaTeXBalanced finds the end of a value argument starting at start
// (already inside one open brace), returning the byte offset of the
// matching closing '}'. Escaped braces (\{, \}) don't affect depth, so a
// literal brace inside the value doesn't prematurely close the argument.
func scanLaTeXBalanced(content []byte, start int) (end int, err error) {
	depth := 1
	i := start
	for i < len(content) {
		b := content[i]
		if b == '\\' && i+1 < len(content) && (content[i+1] == '{' || content[i+1] == '}') {
			i += 2
			continue
		}
		switch b {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
		i++
	}
	return 0, fmt.Errorf("unterminated value argument")
}

// ReplaceLaTeX is the LaTeX analog of Replace: it returns new content with
// each \claimcheck{id}{value} call's value replaced by values[id]. An id
// with no entry in values is left exactly as it was. Returns an error
// under the same conditions as ParseLaTeX.
func ReplaceLaTeX(content []byte, values map[string]string) ([]byte, error) {
	markers, err := ParseLaTeX(content)
	if err != nil {
		return nil, err
	}

	spans := make([]Span, len(markers))
	for i, mk := range markers {
		spans[i] = Span{ID: mk.ID, Start: mk.ContentStart, End: mk.ContentEnd}
	}

	return ReplaceSpans(content, spans, values)
}
