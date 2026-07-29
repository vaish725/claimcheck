package schema

import "testing"

func TestParseTolerance(t *testing.T) {
	cases := []struct {
		raw     string
		wantErr bool
		kind    Kind
		value   float64
	}{
		{raw: "exact", kind: Exact},
		{raw: "EXACT", kind: Exact},
		{raw: "+-5", kind: Absolute, value: 5},
		{raw: "±5", kind: Absolute, value: 5},
		{raw: "+/-5", kind: Absolute, value: 5},
		{raw: "+-10%", kind: Relative, value: 10},
		{raw: "±25%", kind: Relative, value: 25},
		{raw: "", wantErr: true},
		{raw: "5", wantErr: true},        // missing +- prefix
		{raw: "+-abc", wantErr: true},    // non-numeric
		{raw: "+--5", wantErr: true},     // negative tolerance
	}

	for _, tc := range cases {
		got, err := ParseTolerance(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseTolerance(%q): expected error, got none", tc.raw)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseTolerance(%q): unexpected error: %v", tc.raw, err)
			continue
		}
		if got.Kind != tc.kind || got.Value != tc.value {
			t.Errorf("ParseTolerance(%q) = %+v, want Kind=%v Value=%v", tc.raw, got, tc.kind, tc.value)
		}
	}
}

func TestToleranceWithin(t *testing.T) {
	cases := []struct {
		name     string
		tol      Tolerance
		declared float64
		actual   float64
		want     bool
	}{
		{"exact match", Tolerance{Kind: Exact}, 88, 88, true},
		{"exact mismatch", Tolerance{Kind: Exact}, 88, 89, false},
		{"absolute within", Tolerance{Kind: Absolute, Value: 5}, 100, 104, true},
		{"absolute at boundary", Tolerance{Kind: Absolute, Value: 5}, 100, 105, true},
		{"absolute breach", Tolerance{Kind: Absolute, Value: 5}, 100, 106, false},
		{"relative within", Tolerance{Kind: Relative, Value: 10}, 100, 108, true},
		{"relative at boundary", Tolerance{Kind: Relative, Value: 10}, 100, 110, true},
		{"relative breach", Tolerance{Kind: Relative, Value: 10}, 100, 111, false},
		{"relative breach below", Tolerance{Kind: Relative, Value: 10}, 100, 89, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.tol.Within(tc.declared, tc.actual); got != tc.want {
				t.Errorf("Within(%v, %v) = %v, want %v", tc.declared, tc.actual, got, tc.want)
			}
		})
	}
}

func TestToleranceStringRoundTrip(t *testing.T) {
	cases := []string{"exact", "+-5", "+-10%"}
	for _, raw := range cases {
		tol, err := ParseTolerance(raw)
		if err != nil {
			t.Fatalf("ParseTolerance(%q): %v", raw, err)
		}
		if got := tol.String(); got != raw {
			t.Errorf("String() round trip: ParseTolerance(%q).String() = %q", raw, got)
		}
	}
}
