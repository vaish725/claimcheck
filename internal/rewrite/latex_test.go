package rewrite

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceLaTeXGolden(t *testing.T) {
	cases := []struct {
		name   string
		values map[string]string
	}{
		{name: "simple", values: map[string]string{"test_count": "91"}},
		{name: "multiple", values: map[string]string{"test_count": "91"}}, // coverage absent: its marker must stay untouched
		{name: "duplicate", values: map[string]string{"test_count": "91"}},
		{name: "no_markers", values: map[string]string{"test_count": "91"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in, err := os.ReadFile(filepath.Join("testdata", "golden-latex", tc.name+".in.tex"))
			if err != nil {
				t.Fatalf("reading input fixture: %v", err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", "golden-latex", tc.name+".out.tex"))
			if err != nil {
				t.Fatalf("reading golden fixture: %v", err)
			}

			got, err := ReplaceLaTeX(in, tc.values)
			if err != nil {
				t.Fatalf("ReplaceLaTeX: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("ReplaceLaTeX output does not match golden fixture.\ngot:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestParseLaTeXMalformed(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{
			name:    "unterminated id",
			content: `\claimcheck{test_count 88`,
		},
		{
			name:    "invalid character in id",
			content: `\claimcheck{test count}{88}`,
		},
		{
			name:    "missing value argument",
			content: `\claimcheck{test_count} not a brace here`,
		},
		{
			name:    "unterminated value",
			content: `\claimcheck{test_count}{88`,
		},
		{
			name:    "empty id",
			content: `\claimcheck{}{88}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseLaTeX([]byte(tc.content)); err == nil {
				t.Errorf("ParseLaTeX(%q): expected an error, got none", tc.content)
			}
		})
	}
}

func TestParseLaTeXWellFormed(t *testing.T) {
	content := []byte(`a \claimcheck{x}{1} b \claimcheck{y}{2} c`)

	markers, err := ParseLaTeX(content)
	if err != nil {
		t.Fatalf("ParseLaTeX: %v", err)
	}
	if len(markers) != 2 {
		t.Fatalf("len(markers) = %d, want 2", len(markers))
	}
	if got := string(content[markers[0].ContentStart:markers[0].ContentEnd]); got != "1" {
		t.Errorf("marker[0] content = %q, want %q", got, "1")
	}
	if got := string(content[markers[1].ContentStart:markers[1].ContentEnd]); got != "2" {
		t.Errorf("marker[1] content = %q, want %q", got, "2")
	}
}

func TestParseLaTeXEscapedBracesInValue(t *testing.T) {
	// An escaped brace inside the value must not be mistaken for the
	// argument's closing brace.
	content := []byte(`\claimcheck{note}{a \{literal\} brace}`)

	markers, err := ParseLaTeX(content)
	if err != nil {
		t.Fatalf("ParseLaTeX: %v", err)
	}
	if len(markers) != 1 {
		t.Fatalf("len(markers) = %d, want 1", len(markers))
	}
	want := `a \{literal\} brace`
	if got := string(content[markers[0].ContentStart:markers[0].ContentEnd]); got != want {
		t.Errorf("marker content = %q, want %q", got, want)
	}
}

func TestParseLaTeXNestedMarkerIsConsumedNotMatched(t *testing.T) {
	// A \claimcheck{}{} occurring inside another one's value is never
	// separately matched - it's part of the outer value's raw text - since
	// scanning always resumes after the outer marker's closing brace.
	content := []byte(`\claimcheck{outer}{\claimcheck{inner}{1}}`)

	markers, err := ParseLaTeX(content)
	if err != nil {
		t.Fatalf("ParseLaTeX: %v", err)
	}
	if len(markers) != 1 {
		t.Fatalf("len(markers) = %d, want 1 (the inner call must not be separately matched)", len(markers))
	}
	if markers[0].ID != "outer" {
		t.Errorf("marker ID = %q, want %q", markers[0].ID, "outer")
	}
}

func TestReplaceLaTeXLeavesUnknownIDsUntouched(t *testing.T) {
	content := []byte(`\claimcheck{known}{old} and \claimcheck{unknown}{stay}`)

	got, err := ReplaceLaTeX(content, map[string]string{"known": "new"})
	if err != nil {
		t.Fatalf("ReplaceLaTeX: %v", err)
	}
	want := `\claimcheck{known}{new} and \claimcheck{unknown}{stay}`
	if string(got) != want {
		t.Errorf("ReplaceLaTeX = %q, want %q", got, want)
	}
}

// FuzzParseLaTeX feeds arbitrary bytes to ParseLaTeX, which must never
// panic, and checks that any returned markers are in-bounds, ordered, and
// non-overlapping - the same invariants FuzzParse checks for the Markdown
// marker parser.
func FuzzParseLaTeX(f *testing.F) {
	seeds := []string{
		"",
		"plain text, no markers at all",
		`\claimcheck{a}{1}`,
		`\claimcheck{test_count 88`,                 // unterminated id
		`\claimcheck{test count}{88}`,               // invalid character in id
		`\claimcheck{test_count} not a brace`,       // missing value argument
		`\claimcheck{test_count}{88`,                // unterminated value
		`\claimcheck{}{88}`,                         // empty id
		`\claimcheck{outer}{\claimcheck{inner}{1}}`, // nested call
		`\claimcheck{note}{a \{literal\} brace}`,    // escaped braces
		`stray fragments: \claimcheck and {curly} but no macro`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		content := []byte(s)
		markers, err := ParseLaTeX(content)
		if err != nil {
			return // an error is a valid, non-panicking outcome
		}

		prevEnd := 0
		for _, mk := range markers {
			if mk.ContentStart < 0 || mk.ContentEnd < mk.ContentStart || mk.ContentEnd > len(content) {
				t.Fatalf("marker %+v has out-of-bounds offsets for content of length %d", mk, len(content))
			}
			if mk.ContentStart < prevEnd {
				t.Fatalf("marker %+v overlaps or is out of order (previous end %d)", mk, prevEnd)
			}
			prevEnd = mk.ContentEnd
		}

		out, err := ReplaceLaTeX(content, nil) // must never panic; nil values is a no-op
		if err != nil {
			t.Fatalf("ReplaceLaTeX returned an error after ParseLaTeX succeeded: %v", err)
		}
		if !bytes.Equal(out, content) {
			t.Fatalf("ReplaceLaTeX with no values changed the content: got %q, want %q", out, content)
		}
	})
}
