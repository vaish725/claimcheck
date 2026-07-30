package extract

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"

	"github.com/vaish725/claimcheck/internal/schema"
)

// pytestCountExtractor counts a Python repo's tests via pytest's built-in
// JUnit XML report, so no extra plugin is required in the target repo.
type pytestCountExtractor struct{}

func (pytestCountExtractor) Extract(ctx context.Context, repoPath string, _ schema.Claim) (float64, error) {
	reportPath, cleanup, err := tempFilePath("claimcheck-junit-*.xml")
	if err != nil {
		return 0, err
	}
	defer cleanup()

	cmd := exec.CommandContext(ctx, "python3", "-m", "pytest", "--junitxml="+reportPath, "-q")
	cmd.Dir = repoPath
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if _, isExitErr := err.(*exec.ExitError); !isExitErr {
			return 0, fmt.Errorf("running pytest: %w: %s", err, stderr.String())
		}
		// failing tests still leave a valid report
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		return 0, fmt.Errorf("reading pytest junit report: %w", err)
	}
	count, err := parseJUnitTestCount(data)
	if err != nil {
		return 0, err
	}
	return float64(count), nil
}

// parseJUnitTestCount sums the "tests" attribute of every <testsuite>
// element, handling both a bare <testsuite> root and a <testsuites> root
// wrapping several.
func parseJUnitTestCount(data []byte) (int, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	total := 0
	found := false
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, fmt.Errorf("parsing junit xml: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "testsuite" {
			continue
		}
		for _, attr := range se.Attr {
			if attr.Name.Local != "tests" {
				continue
			}
			n, err := strconv.Atoi(attr.Value)
			if err != nil {
				return 0, fmt.Errorf("parsing testsuite tests attribute %q: %w", attr.Value, err)
			}
			total += n
			found = true
		}
	}
	if !found {
		return 0, fmt.Errorf("no <testsuite tests=...> element found in junit xml")
	}
	return total, nil
}

// pytestCoverageExtractor recomputes coverage via coverage.py's aggregate
// line-rate from its own XML report.
type pytestCoverageExtractor struct{}

func (pytestCoverageExtractor) Extract(ctx context.Context, repoPath string, claim schema.Claim) (float64, error) {
	dataPath, cleanupData, err := tempFilePath("claimcheck-coverage-*.data")
	if err != nil {
		return 0, err
	}
	defer cleanupData()

	xmlPath, cleanupXML, err := tempFilePath("claimcheck-coverage-*.xml")
	if err != nil {
		return 0, err
	}
	defer cleanupXML()

	// COVERAGE_FILE keeps coverage.py's data file out of the target repo's
	// working tree, so claimcheck leaves no stray build artifacts behind.
	env := append(os.Environ(), "COVERAGE_FILE="+dataPath)

	runCmd := exec.CommandContext(ctx, "python3", "-m", "coverage", "run", "-m", "pytest", "-q")
	runCmd.Dir = repoPath
	runCmd.Env = env
	var runStderr bytes.Buffer
	runCmd.Stdout = io.Discard
	runCmd.Stderr = &runStderr
	if err := runCmd.Run(); err != nil {
		if _, isExitErr := err.(*exec.ExitError); !isExitErr {
			return 0, fmt.Errorf("running coverage run: %w: %s", err, runStderr.String())
		}
	}

	xmlCmd := exec.CommandContext(ctx, "python3", "-m", "coverage", "xml", "-o", xmlPath)
	xmlCmd.Dir = repoPath
	xmlCmd.Env = env
	var xmlStderr bytes.Buffer
	xmlCmd.Stdout = io.Discard
	xmlCmd.Stderr = &xmlStderr
	if err := xmlCmd.Run(); err != nil {
		return 0, fmt.Errorf("running coverage xml: %w: %s", err, xmlStderr.String())
	}

	data, err := os.ReadFile(xmlPath)
	if err != nil {
		return 0, fmt.Errorf("reading coverage xml report: %w", err)
	}
	pct, err := parseCoverageXML(data)
	if err != nil {
		return 0, fmt.Errorf("claim %q: %w", claim.ID, err)
	}
	return pct, nil
}

// coverageXMLRoot models the root <coverage> element of coverage.py's
// Cobertura-style XML report.
type coverageXMLRoot struct {
	XMLName  xml.Name `xml:"coverage"`
	LineRate float64  `xml:"line-rate,attr"`
}

// parseCoverageXML reads coverage.py's aggregate line-rate (a 0-1 fraction)
// and converts it to a percentage.
func parseCoverageXML(data []byte) (float64, error) {
	var root coverageXMLRoot
	if err := xml.Unmarshal(data, &root); err != nil {
		return 0, fmt.Errorf("parsing coverage xml: %w", err)
	}
	return root.LineRate * 100, nil
}

// tempFilePath reserves a unique path matching pattern, for tools that
// insist on creating the file themselves. Caller must defer cleanup.
func tempFilePath(pattern string) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", nil, fmt.Errorf("reserving temp file: %w", err)
	}
	p := f.Name()
	f.Close()
	os.Remove(p)
	return p, func() { os.Remove(p) }, nil
}
