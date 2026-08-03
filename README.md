# claimcheck

Keep the numeric claims in your README and resume true. Declare each claim
once in a `claims.yaml` next to the code it describes, then recompute it
from real repo state on demand. `claimcheck verify` fails when a claim has
drifted beyond its declared tolerance.

This checks whether claims are *accurate*, not whether the underlying
numbers are *good*. It is not a coverage gate, a CI dashboard, or a metrics
history tool.

## Install

```
go install github.com/vaish725/claimcheck/cmd/claimcheck@latest
```

Or build from source:

```
go build -o claimcheck ./cmd/claimcheck
```

## Usage

Add a `claims.yaml` to the root of the repo you want to check. See
[examples/go/claims.yaml](examples/go/claims.yaml) or
[examples/python/claims.yaml](examples/python/claims.yaml) for a starting
point. Every claim needs an id, a type, a declared value, and a tolerance:

```yaml
repo: my-project
claims:
  - id: test_count
    type: test_count
    runner: go
    declared: 88
    tolerance: exact
    asserted_in: [README.md]
```

Then run:

```
claimcheck verify
```

This recomputes every claim, prints a drift report, and exits non-zero if
anything breached tolerance:

```
CLAIM       DECLARED  ACTUAL  DELTA  TOLERANCE  VERDICT
test_count  88        91      +3     exact      BREACH
coverage    54        53.2    -0.8   +-3%       PASS
```

Use `-soft` to print the same report without failing the build:

```
claimcheck verify -soft
```

Use `-claims` to point at a claims.yaml that isn't in the repo root, and
pass a path argument to check a repo other than the current directory:

```
claimcheck verify -claims path/to/claims.yaml path/to/repo
```

## Regenerating numbers with `update`

`claimcheck update` recomputes every claim and rewrites both `claims.yaml`'s
declared values and every marked span in the files each claim's
`asserted_in` lists, so refreshing a README or resume is one command
instead of manual editing:

```
claimcheck update
```

Use `-dry-run` to see what would change without writing anything:

```
claimcheck update -dry-run
```

To make a value in a file rewritable, wrap it in a matching pair of
markers named after the claim's id:

```markdown
Tests: <!-- claimcheck:test_count -->88<!-- /claimcheck:test_count --> passing.
```

Only the text *between* the markers is replaced, so put any unit outside
the closing tag, not inside it:

```markdown
Coverage: <!-- claimcheck:coverage -->54<!-- /claimcheck:coverage -->%.
```

`update` never writes a partial file: each rewrite is a temp-file-plus-rename,
so an interrupted run can't corrupt a source file, and comments and
formatting elsewhere in `claims.yaml` are left exactly as they were - only
the `declared:` (and, for benchmarks, `machine:`) values themselves change.
Both must be written as bare (unquoted) scalars for `update` to find them.

If any claim's actual value can't be recomputed, `update` still applies
every claim it could and reports the failure, exiting non-zero so a
partial update doesn't look like a clean one.

## Claim types

| Type | Runner | What it measures |
|------|--------|-------------------|
| `test_count` | `go` or `pytest` | number of tests with a terminal result |
| `coverage` | `go` or `pytest` | aggregate line coverage percentage |
| `loc` | none | lines across tracked and non-ignored files (`git ls-files`) |
| `commit_count` | none | commits reachable from `HEAD` |
| `benchmark` | none | one numeric field from a command's JSON stdout |

A `benchmark` claim runs a shell `command` and reads one flat field out of
its JSON output:

```yaml
- id: query_p50
  type: benchmark
  command: "python -m codesearch.bench --json"
  field: p50_ms
  declared: 0.09
  machine: unset
  tolerance: "+-25%"
  asserted_in: [resume]
```

Benchmark numbers are machine-dependent, so they carry a `machine`
fingerprint (OS/arch/core-count) and are **never compared across
machines** - a claim declared on one machine reports `SKIP`, not a
pass or a breach, when checked on another. Start a new benchmark claim
with `machine: unset` (a real value can't be written by hand in advance);
running `claimcheck update` on the machine whose numbers should be
canonical establishes both `declared` and `machine` together. From then
on, `update` refuses to touch that claim from any other machine, so a
different laptop or CI runner can never silently overwrite your declared
number - you have to run `update` on the original machine, or clear the
`machine` field back to `unset` to deliberately adopt a new one.

## Tolerance

Every claim must declare a tolerance; there is no default. This is
deliberate: a claim with no stated tolerance has no stated definition of
"still true".

- `exact` - the actual value must equal the declared value.
- `+-5` - absolute band.
- `+-10%` - relative band, as a percentage of the declared value.

Use exact tolerance for claims like "all tests pass". Use a wide relative
band for anything machine-dependent, like a benchmark. Use a tight
absolute band for anything a reader would treat as a headline number.

## GitHub Action

Run verification on every push:

```yaml
name: claimcheck
on: [push]
jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: stable
      - run: go install github.com/vaish725/claimcheck/cmd/claimcheck@latest
      - run: claimcheck verify
```

## Status

Test count, coverage, LOC, commit count, and benchmark claims are all
implemented, with concurrent extraction and a per-claim timeout so a hung
subprocess can't hang CI. `claimcheck verify` and `claimcheck update`
(marker rewriting in README-style files, atomic writes, machine-fingerprint
gating for benchmarks) are both implemented. LaTeX span substitution and
resume mode across multiple repos are not built yet.
