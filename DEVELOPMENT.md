# Development and acceptance

This guide is the contributor entry point. [The verification guide](tests/README.md)
contains the maintained suite inventory, CI selection rules, evidence contracts
and failure controls. [The architecture](docs/architecture.md) describes runtime
ownership; [CONTRACT.md](CONTRACT.md) defines the public API.

## Build and run a public consumer

Use the exact Go version in [go.mod](go.mod), a native C compiler and Python 3.
Set `GOWORK=off` and `CGO_ENABLED=1`; native verification additionally requires
`GOTOOLCHAIN=local`. `GO` may select a Go executable for the native/e2e runners;
if `GOROOT` is set, it must belong to that executable. Other scripts invoke `go`
from `PATH`, so keep the selected toolchain consistent.

```sh
GOWORK=off CGO_ENABLED=1 go build ./...
python3 tests/native.py --expected-platform linux/amd64
```

Replace the platform with the actual native host: `linux/amd64`, `linux/arm64`,
`darwin/amd64`, `darwin/arm64` or `windows/amd64`. The native runner builds an
independent consumer from a content-addressed module, with no local `replace`.
Add `--source-commit <full-commit-sha>` to require the packaged runtime inputs
(including its contract and grammar notices) to match that commit exactly.
Working-tree verification without this option does not establish a committed
source identity.

For a focused change, run the affected public consumer and its existing fault
control. For example:

```sh
python3 tests/e2e.py index
GOFLAGS=-race python3 tests/e2e.py index-race
go test -race -count=1 ./index
python3 tests/ablate.py index-query-budget index-query-files index-query-scope
```

The complete native workflow also runs source delivery, deterministic property
relations, mutation fuzzing and pinned downstream LSP acceptance. Their commands,
output paths and limits are maintained in [tests/README.md](tests/README.md).
Use fresh output directories where the runner requires them. A build failure,
missing tool or stale report cannot stand in for a detected behavioral defect.

## Maintain a public API or native dependency

Update the implementation, its independent consumer expectations and the public
contract together when behavior changes. Consumers retain byte-identical
contract snapshots and implementation locks; a documentation edit to
`CONTRACT.md` therefore requires deliberate compatibility handling. The
contract ID and a module version identify different things.

Preserve grammar provenance, license, patch and all required C includes/headers.
Run the existing checkout, snapshot, vendor and delivery checks when those
inputs or packaging rules change. A matching Go build alone does not prove that
the published module or vendored consumer contains the same native inputs.

The two Argo plugins upgrade independently. Their actual pins are documented in
[the consumer boundary table](docs/architecture.md#两个插件的实际引用与隔离).
`tests/downstream-pins.json` separately selects immutable test consumers; those
pins are test inputs, not the latest plugin versions. Candidate injection into
an isolated downstream checkout does not update or publish either plugin.

## CI and publication

`Toolkit required` validates the execution plan and reports from the same run
attempt. A partial rerun that leaves producer evidence on an older attempt
requires rerunning the complete workflow. The full plan and the exact prose-only
PR allowlist are described in [the CI guide](tests/README.md#ci-执行与证据门禁).

The publication job only runs after successful acceptance for a main push and
checks that the same commit remains main. [VERSION](VERSION) selects the alpha
tag and [RELEASE_NOTES.md](RELEASE_NOTES.md) supplies its notes. Existing releases
remain unchanged; an existing tag without a release is rejected. To publish a
new alpha, update the version and its release-specific notes intentionally, then
allow the full main workflow to verify that commit.

Keep current operating documentation and executable fixtures. Do not commit
stage plans, audit snapshots, experiment transcripts or generated evidence under
documentation; reports belong in the ignored `artifacts/` directory. Active
machine contracts, including `docs/testing-ci-baseline.json`, remain required
inputs and are explained in the [documentation index](docs/README.md).
