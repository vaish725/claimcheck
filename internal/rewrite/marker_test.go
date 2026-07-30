package rewrite

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceSpansOutOfOrderInput(t *testing.T) {
	content := []byte("aaXbbYcc") // X at [2,3), Y at [5,6)
	spans := []Span{
		{ID: "y", Start: 5, End: 6}, // given out of order on purpose
		{ID: "x", Start: 2, End: 3},
	}

	got, err := ReplaceSpans(content, spans, map[string]string{"x": "1", "y": "22"})
	if err != nil {
		t.Fatalf("ReplaceSpans: %v", err)
	}
	if want := "aa1bb22cc"; string(got) != want {
		t.Errorf("ReplaceSpans = %q, want %q", got, want)
	}
}

func TestReplaceSpansOverlapError(t *testing.T) {
	content := []byte("abcdef")
	spans := []Span{
		{ID: "a", Start: 0, End: 3},
		{ID: "b", Start: 2, End: 5}, // overlaps "a"
	}

	if _, err := ReplaceSpans(content, spans, map[string]string{"a": "x", "b": "y"}); err == nil {
		t.Fatal("expected an error for overlapping spans, got none")
	}
}

func TestReplaceSpansUnknownIDLeftUntouched(t *testing.T) {
	content := []byte("aaXbb")
	spans := []Span{{ID: "x", Start: 2, End: 3}}

	got, err := ReplaceSpans(content, spans, map[string]string{"other": "1"})
	if err != nil {
		t.Fatalf("ReplaceSpans: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("ReplaceSpans = %q, want unchanged %q", got, content)
	}
}

func TestReplaceGolden(t *testing.T) {
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
			in, err := os.ReadFile(filepath.Join("testdata", "golden", tc.name+".in.md"))
			if err != nil {
				t.Fatalf("reading input fixture: %v", err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", "golden", tc.name+".out.md"))
			if err != nil {
				t.Fatalf("reading golden fixture: %v", err)
			}

			got, err := Replace(in, tc.values)
			if err != nil {
				t.Fatalf("Replace: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("Replace output does not match golden fixture.\ngot:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestParseMalformed(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{
			name:    "unterminated",
			content: "before <!-- claimcheck:a -->42 no close tag after",
		},
		{
			name:    "nested",
			content: "<!-- claimcheck:a --><!-- claimcheck:b -->1<!-- /claimcheck:b --><!-- /claimcheck:a -->",
		},
		{
			name:    "mismatched close",
			content: "<!-- claimcheck:a -->1<!-- /claimcheck:b -->",
		},
		{
			name:    "orphaned close",
			content: "text <!-- /claimcheck:a --> more text",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse([]byte(tc.content)); err == nil {
				t.Errorf("Parse(%q): expected an error, got none", tc.content)
			}
		})
	}
}

func TestParseWellFormed(t *testing.T) {
	content := []byte("a <!-- claimcheck:x -->1<!-- /claimcheck:x --> b <!-- claimcheck:y -->2<!-- /claimcheck:y --> c")

	markers, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
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

func TestReplaceLeavesUnknownMarkersUntouched(t *testing.T) {
	content := []byte("<!-- claimcheck:known -->old<!-- /claimcheck:known --> and <!-- claimcheck:unknown -->stay<!-- /claimcheck:unknown -->")

	got, err := Replace(content, map[string]string{"known": "new"})
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	want := "<!-- claimcheck:known -->new<!-- /claimcheck:known --> and <!-- claimcheck:unknown -->stay<!-- /claimcheck:unknown -->"
	if string(got) != want {
		t.Errorf("Replace = %q, want %q", got, want)
	}
}

// FuzzParse feeds arbitrary bytes to Parse, which must never panic, and
// checks that any returned markers are in-bounds, ordered, and
// non-overlapping. `go test ./...` runs only the seeds below; `go test
// -fuzz=FuzzParse` does real generative fuzzing.
func FuzzParse(f *testing.F) {
	seeds := []string{
		"",
		"plain text, no markers at all",
		"<!-- claimcheck:a -->1<!-- /claimcheck:a -->",
		"before <!-- claimcheck:a -->42 no close tag after",                                       // unterminated
		"<!-- claimcheck:a --><!-- claimcheck:b -->1<!-- /claimcheck:b --><!-- /claimcheck:a -->", // nested
		"<!-- claimcheck:a -->1<!-- /claimcheck:b -->",                                            // mismatched close
		"text <!-- /claimcheck:a --> more text",                                                   // orphaned close
		"<!--claimcheck:a-->x<!--/claimcheck:a-->",                                                // no interior whitespace
		"stray fragments: <!-- and --> claimcheck: and -->",                                       // look-alikes that shouldn't match
		"unicode content: éèê <!-- claimcheck:é -->1<!-- /claimcheck:é -->",                       // non-ASCII id, won't match the id charset
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		content := []byte(s)
		markers, err := Parse(content)
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

		out, err := Replace(content, nil) // must never panic; nil values is a no-op
		if err != nil {
			t.Fatalf("Replace returned an error after Parse succeeded: %v", err)
		}
		if !bytes.Equal(out, content) {
			t.Fatalf("Replace with no values changed the content: got %q, want %q", out, content)
		}
	})
}
