# Consumer verification

Use Go 1.27.3, Python 3, a native C compiler, and writable Go caches. `GO` can select the Go executable; an explicit `GOROOT` must match it. Set `GOWORK=off` and `CGO_ENABLED=1`. Expectations in `consumer/*.go` are independently specified rather than calculated from the implementation.

## Native delivery

From this repository, run:

```sh
python3 tests/native.py --expected-platform linux/amd64
```

Supported native targets are `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, and `windows/amd64`. The runner checks host and target OS/architecture and rejects cross-compilation. `CC` selects a compiler executable; `--cc-version` requires a substring in its version output. `--output` selects the evidence directory (default: `artifacts/native`).

The runner creates a content-addressed local module proxy, builds an independent consumer without replace directives, and runs all eight existing suites: `source`, `edit`, `yaml`, `audit`, `budget`, `retention`, `resume`, and `resources`. Every suite must exist, contain assertions, and pass. Missing tools, compilation failures, timeouts, and failed assertions produce a report and nonzero exit status.

The [native CI workflow](../.github/workflows/native.yml) runs the same entry point on five native runners. Its pinned Go, Python, Actions, and accepted C compiler versions are defined in the workflow. GitHub runner images change over time; evidence records their image versions rather than claiming bit-identical images.

The optional `--source-commit <commit>` requires every packaged runtime file to match the raw Git object in that commit. CI passes its exact source commit. Reports record the commit and both object IDs; missing Git metadata, an unavailable commit, or any byte difference fails verification. Default runs consume the current working tree, including uncommitted edits, and source archives without Git remain supported. `.gitattributes` fixes text files to LF so the same commit produces identical snapshot and contract identities on every native platform.

The checkout E2E exercises this behavior with two real temporary Git checkouts, `core.autocrlf=false` and `true`. Both package the public module and run all eight native consumer suites. Both request explicit commit verification. Expected bytes come directly from the temporary commit's Git blobs, independently of the snapshot packager. Additional cases consume an uncommitted runtime edit through the default entry point and reject explicit verification with a missing commit or missing Git metadata:

```sh
python3 tests/checkout_e2e.py
python3 tests/snapshot_e2e.py
```

Run this entry point on Linux amd64. It saves the exact source archive, Git settings and commands, module/contract identities, byte comparisons, and native consumer reports under `artifacts/checkout/`. `--output` chooses another evidence directory. `--without-attributes` removes the checkout policy only in the temporary source fixture; this deliberate counterexample must fail the byte-identity checks. It never changes the caller's Git index or configuration.

`snapshot_e2e.py` compares complete module ZIP bytes from the real snapshot CLI under native defaults and isolated subprocesses reproducing the ZIP constructor default and path ordering observed on Windows, plus an alternate valid deflate strategy. It retains all outputs and exact inputs under `artifacts/snapshot/`. This environment experiment is not native Windows acceptance. The CI matrix must also produce the same source, contract, version, and ZIP digests on all five platforms. ZIP creator metadata is fixed to Unix to match the archive's existing Unix permission bits, and entries are ordered by relative POSIX path strings independently of host filesystem path comparison. Module ZIP entries are stored without compression so different deflate implementations cannot change their bytes. Packaging changes require a fresh proxy directory and explicit consumer ZIP-lock updates; existing same-version proxy bytes are never overwritten.

## Individual suites and ablations

```sh
python3 tests/e2e.py source
python3 tests/e2e.py edit
python3 tests/e2e.py yaml
python3 tests/e2e.py audit
python3 tests/e2e.py budget
python3 tests/e2e.py retention
python3 tests/e2e.py resume
python3 tests/e2e.py resources
python3 tests/ablate.py
python3 tests/ablate.py resources
python3 tests/ablate.py retention
```

These consumers use a temporary independent Go module and local replace directive; the distributed `go.mod` contains no local paths. Ablations run modified copies and require a successful build plus the designated behavior assertion failing. A compilation failure does not count as detecting a missing guard.

The suites cover text coordinates and atomic edits; YAML structure, recovery, duplicate keys, aliases, and merges; the 256-level and 2,000-document boundaries; deep indentation and scanner recovery; cancellation; and resource lifetimes. `resume` checks the order and raw values of 8,000 key/value pairs and a one-second deadline on 70,000 comments.

`retention` exercises 96 independent 1 MiB context values for each success, depth-error, and cancellation path, checking finalizers and an 8 MiB retention threshold. `resources` measures resident memory after warmup and 2,000 larger parses, with a 64 MiB growth threshold. These experimental thresholds are not additional API limits and do not prove the absence of all leaks.

Resident memory is measured through Linux `/proc/self/status` VmRSS, macOS `ps -o rss=` (KiB converted to bytes), or Windows `GetProcessMemoryInfo` WorkingSetSize (bytes). A failed measurement fails the assertion; Go heap allocation is not a substitute for native resident memory.

## Independent snapshots and delivery failure checks

```sh
python3 tools/snapshot.py --output ./artifacts/module-proxy
python3 tests/delivery.py ./artifacts/module-proxy/manifest.json ./artifacts/module-proxy
python3 tests/native_delivery_e2e.py
python3 tests/delivery_failures_e2e.py
```

`snapshot.py` packages the public runtime sources, API contract, C includes and headers, provenance, and licenses. It rejects different bytes for an existing version and writes `manifest.json`. It does not publish a module.

Before `delivery.py`, set `GOMODCACHE` to a writable absolute path and populate its download cache with `go mod download`. The script collects dependencies locked by `go.sum`, including their real `.info` metadata, and builds a consumer with a fresh module cache, only a file proxy, `GOSUMDB=off`, `GOWORK=off`, and no replace directive. It runs its five delivery suites without reading sibling repositories. An incomplete download cache must be populated through the normal Go download mechanism; metadata must not be fabricated.

On Linux, `native_delivery_e2e.py` runs an isolated copy without parent or sibling sources and checks that empty suites, failed intermediate RSS reads, a wrong platform, and a missing compiler fail. `delivery_failures_e2e.py` verifies that missing Go or C tools, corrupt child reports, and timeouts cannot reuse a previous green report. It also removes the RSS all-samples guard in an isolated variant and requires the failure scenario to detect that buildable variant.

## Evidence format

Reports include commands, tool versions, environment, source and input hashes, independently specified expectations and assertion results, binary hashes, real stdout/stderr, and exit codes. Input ZIPs preserve the exact consumer and modified sources; binaries are retained under their SHA-256. Failure reports and earlier runs are preserved. Zero assertions, missing reports, and empty success entry points do not pass.

Native evidence also includes the module ZIP and manifest, consumer `go.mod`/`go.sum`, dependency manifests, runner image information, and the applicable MIT, Unicode, and Go BSD licenses. Compiler and SDK binaries are not redistributed. These artifacts are verification evidence, not a released SDK or product installer.

All evidence is generated locally under ignored `artifacts/` paths. Individual suites write `<stage>/report.json`; ablations write `<stage>-ablation/report.json`. Other default directories are `delivery`, `vendor-probe`, `native`, `native-delivery`, and `delivery-failures`. Source checkouts contain the durable inputs and expectations in `tests/consumer/`, not historical reports or binaries. Native consumer verification does not establish real Zed acceptance; each platform requires its own actual run.
