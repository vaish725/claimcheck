// Package schema defines the claims.yaml file format and the rules for
// how much a measured value is allowed to drift from its declared value.
package schema

import (
	"fmt"
	"strconv"
	"strings"
)

// Kind identifies how a tolerance value should be interpreted.
type Kind int

const (
	Exact    Kind = iota // actual must equal declared precisely
	Absolute             // actual may differ from declared by at most Value
	Relative             // actual may differ from declared by at most Value percent
)

// Tolerance is the parsed form of a claim's "tolerance" field, e.g.
// "exact", "+-5", or "+-10%".
type Tolerance struct {
	Kind  Kind
	Value float64 // unused when Kind is Exact
}

// ParseTolerance parses the tolerance string from claims.yaml. There is no
// default: an empty or malformed string is always an error, since a claim
// with no stated tolerance has no definition of "still true".
func ParseTolerance(raw string) (Tolerance, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Tolerance{}, fmt.Errorf("tolerance is required and cannot be empty")
	}

	if strings.EqualFold(s, "exact") {
		return Tolerance{Kind: Exact}, nil
	}

	// Accept "+-" and "+/-" as ASCII stand-ins for "±".
	body := s
	for _, prefix := range []string{"±", "+/-", "+-"} {
		if strings.HasPrefix(body, prefix) {
			body = strings.TrimPrefix(body, prefix)
			break
		}
	}
	if body == s {
		return Tolerance{}, fmt.Errorf("tolerance %q must be \"exact\", \"+-<number>\", or \"+-<number>%%\"", raw)
	}

	kind := Absolute
	if strings.HasSuffix(body, "%") {
		kind = Relative
		body = strings.TrimSuffix(body, "%")
	}

	value, err := strconv.ParseFloat(strings.TrimSpace(body), 64)
	if err != nil {
		return Tolerance{}, fmt.Errorf("tolerance %q has an invalid numeric part: %w", raw, err)
	}
	if value < 0 {
		return Tolerance{}, fmt.Errorf("tolerance %q must not be negative", raw)
	}

	return Tolerance{Kind: kind, Value: value}, nil
}

// Within reports whether actual is close enough to declared to satisfy t.
func (t Tolerance) Within(declared, actual float64) bool {
	switch t.Kind {
	case Exact:
		return declared == actual
	case Absolute:
		return diff(declared, actual) <= t.Value
	case Relative:
		allowed := declared * (t.Value / 100)
		if allowed < 0 {
			allowed = -allowed
		}
		return diff(declared, actual) <= allowed
	default:
		return false
	}
}

// String renders the tolerance back into its claims.yaml form.
func (t Tolerance) String() string {
	switch t.Kind {
	case Exact:
		return "exact"
	case Absolute:
		return fmt.Sprintf("+-%s", trimFloat(t.Value))
	case Relative:
		return fmt.Sprintf("+-%s%%", trimFloat(t.Value))
	default:
		return "unknown"
	}
}

func diff(a, b float64) float64 {
	d := a - b
	if d < 0 {
		return -d
	}
	return d
}

// trimFloat drops a trailing ".0" so "+-5" round-trips as "+-5".
func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
