package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/vaish725/claimcheck/internal/schema"
)

// benchmarkExtractor runs a claim's shell command and reads one numeric
// field out of its JSON stdout.
type benchmarkExtractor struct{}

func (benchmarkExtractor) Extract(ctx context.Context, repoPath string, claim schema.Claim) (float64, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", claim.Command)
	cmd.Dir = repoPath
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Unlike go test/pytest, a non-zero exit here is a hard failure: a
	// benchmark script has no equivalent of "some benchmarks failed but
	// the JSON is still trustworthy".
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("running benchmark command: %w: %s", err, stderr.String())
	}

	value, err := extractJSONField(stdout.Bytes(), claim.Field)
	if err != nil {
		return 0, fmt.Errorf("claim %q: %w", claim.ID, err)
	}
	return value, nil
}

// extractJSONField reads one flat top-level numeric field from a JSON
// object, e.g. {"p50_ms": 0.09}.
func extractJSONField(output []byte, field string) (float64, error) {
	var data map[string]json.RawMessage
	if err := json.Unmarshal(output, &data); err != nil {
		return 0, fmt.Errorf("parsing benchmark output as JSON: %w", err)
	}

	raw, ok := data[field]
	if !ok {
		return 0, fmt.Errorf("field %q not found in benchmark output", field)
	}

	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("field %q is not a number: %w", field, err)
	}
	return value, nil
}
