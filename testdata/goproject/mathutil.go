// Package goproject is a tiny fixture used by the extract package's tests.
// It exists purely to give the Go test-count and coverage extractors
// something real to run against; it has no relation to claimcheck's own
// functionality.
package goproject

import "errors"

func Add(a, b int) int {
	return a + b
}

func Sub(a, b int) int {
	return a - b
}

func Mul(a, b int) int {
	return a * b
}

// Div is deliberately left untested by mathutil_test.go so the fixture
// has partial, non-trivial coverage rather than a suspicious 100%.
func Div(a, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("division by zero")
	}
	return a / b, nil
}
