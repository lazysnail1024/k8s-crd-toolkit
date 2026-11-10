# Consumer verification

Use Go 1.27.3, Python 3, a native C compiler, and writable Go caches. `GO` can select the Go executable; an explicit `GOROOT` must match it. Set `GOWORK=off` and `CGO_ENABLED=1`. Expectations in `consumer/*.go` are independently specified rather than calculated from the implementation.

## Native delivery

From this repository, run:

```sh
python3 tests/native.py --expected-platform linux/amd64
```

Supported native targets are `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, and `windows/amd64`. The runner checks host and target OS/architecture and rejects cross-compilation. `CC` selects a compiler executable; `--cc-version` requires a substring in its version output. `--output` selects the evidence directory (default: `artifacts/native`).

The runner creates a content-addressed local module proxy, builds an independent consumer without replace directives, and runs all ten suites: `source`, `edit`, `yaml`, `audit`, `budget`, `retention`, `resume`, `resources`, `index`, and `workspace`. The exact set of suite names must be present; every suite must contain assertions and pass. Unknown consumer stages fail instead of falling back to a different suite. Missing tools, compilation failures, timeouts, and failed assertions produce a report and nonzero exit status.

The [native CI workflow](../.github/workflows/native.yml) runs the same entry point on five native runners. Its pinned Go, Python, Actions, and accepted C compiler versions are defined in the workflow. GitHub runner images change over time; evidence records their image versions rather than claiming bit-identical images.

The optional `--source-commit <commit>` requires every packaged runtime file to match the raw Git object in that commit. CI passes its exact source commit. Reports record the commit and both object IDs; missing Git metadata, an unavailable commit, or any byte difference fails verification. Default runs consume the current working tree, including uncommitted edits, and source archives without Git remain supported. `.gitattributes` fixes text files to LF so the same commit produces identical snapshot and contract identities on every native platform.

The checkout E2E exercises this behavior with two real temporary Git checkouts, `core.autocrlf=false` and `true`. Both package the public module and run all ten native consumer suites. Both request explicit commit verification. The temporary fixture includes tracked and nonignored untracked candidate files, without changing the caller's Git index. Expected bytes come directly from the temporary commit's Git blobs, independently of the snapshot packager. Additional cases consume an uncommitted runtime edit through the default entry point and reject explicit verification with a missing commit or missing Git metadata:

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
python3 tests/e2e.py index
python3 tests/e2e.py workspace
python3 tests/ablate.py
python3 tests/ablate.py resources
python3 tests/ablate.py retention
GOFLAGS=-race python3 tests/e2e.py index-race
```

These consumers use a temporary independent Go module and local replace directive; the distributed `go.mod` contains no local paths. Ablations run modified copies and require a successful build plus the designated behavior assertion failing. A compilation failure does not count as detecting a missing guard.

The suites cover text coordinates and atomic edits; YAML structure, recovery, duplicate keys, aliases, and merges; the 256-level and 2,000-document boundaries; deep indentation and scanner recovery; cancellation; and resource lifetimes. `resume` checks the order and raw values of 8,000 key/value pairs and a one-second deadline on 70,000 comments.

`index` checks real consumer sequences through the public API: stale, replayed, deleted/recreated and cross-instance tickets; missing-target dependencies; definition additions, duplicates, moves and removals; position-only changes versus changed meaning; pending-location withdrawal; opaque scope/key separation; bounded symbol search; immutable input/output ownership; invalid facts; and recoverable capacity failures. Fixed Application/AppProject and Workflow/WorkflowTemplate YAML fixtures are parsed through `source` and `yamlview` into caller-owned facts. They demonstrate the same mechanism in both consumers without adding Argo rules to the toolkit. A separate concurrent scenario checks that `GetFile` never mixes a revision with another commit's facts. The race-enabled command above runs this independent consumer with the actual Go race detector; `GOFLAGS`, toolchain and compiler selection are recorded in its report.

The summary retention case keeps 16 tiny symbol names sliced from independent 1 MiB strings. The index must retain usable summaries without keeping those entire backing strings alive. It checks Go heap growth after collection against an 8 MiB experimental threshold. This is a specific retained-input check, not an RSS limit or proof of absence of every leak.

`workspace` exercises authoritative unsaved buffers, read-ticket revocation, close/reopen, byte and entry budgets, queue coalescing and priority promotion, and foreground/background fairness. A real directory fixture contains 800 YAML and 800 ordinary files. Discovery starts scanning that directory before a newly requested current-file directory is prioritized; the current file must arrive within a two-unit batch, and the interrupted scan must resume without duplicate or lost candidates. Cancellation, symlink roots and children, exclusions, malformed options, missing directories and directory-capacity exhaustion have explicit assertions. Evidence records work counts, candidate counts, disk-read counts and queue service counts. A Windows host unable to create symlinks records that limitation separately; it does not claim that branch was exercised.

The disk-read consumer opens and closes the file explicitly and reads at most the configured byte limit plus one byte before publishing. Failed or cancelled workers call `DiscardRead(ticket)`: a superseded worker cannot revoke the newer read for the same URI. Separate scenarios check that cancelling the current read frees its entry budget, repeated and cross-store cancellation leave valid reads intact, and an unsaved overlay survives cleanup. Missing-file, cancelled-context and oversized-file paths exercise this cleanup through the actual bounded-read helper. `Invalidate(uri)` is reserved for authoritative filesystem events or coordinator decisions.

These are library consumers. They do not prove large-workspace LSP completion, complete Argo semantics, editor protocol behavior or real Zed interaction. Those remain acceptance tests of the language servers that use the toolkit.

The new buildable counterexamples can be run independently of the existing parser/resource ablations:

```sh
python3 tests/ablate.py index-ticket index-missing index-position index-fresh index-retention
python3 tests/ablate.py workspace-overlay workspace-ticket workspace-generation workspace-fairness
python3 tests/ablate.py workspace-discard
python3 tests/ablate.py workspace-priority workspace-complete workspace-filter
```

Each counterexample removes one relevant guard in an isolated runtime copy, compiles the same independent consumer, and must fail its designated behavior assertion. The expected failures cover stale commits, newly satisfied references, positional invalidation, stale navigation, retained source storage, authoritative overlays, disk read generations, ticket-scoped cancellation, queue generations/fairness, current-directory priority, honest completeness and candidate filtering. `workspace-discard` removes only the read-generation comparison from `DiscardRead`; its compiled consumer must fail `cancelled-old-read-keeps-new-ticket`. A crash, compilation failure, timeout or unrelated assertion does not count as detecting the counterexample.

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

Before `delivery.py`, set `GOMODCACHE` to a writable absolute path and populate its download cache with `go mod download`. The script collects dependencies locked by `go.sum`, including their real `.info` metadata, and builds a consumer with a fresh module cache, only a file proxy, `GOSUMDB=off`, `GOWORK=off`, and no replace directive. It runs seven delivery suites (`source`, `edit`, `yaml`, `audit`, `resources`, `index`, and `workspace`) without reading sibling repositories. An incomplete download cache must be populated through the normal Go download mechanism; metadata must not be fabricated.

On Linux, `native_delivery_e2e.py` runs an isolated copy without parent or sibling sources and checks that empty suites, failed intermediate RSS reads, a wrong platform, and a missing compiler fail. `delivery_failures_e2e.py` verifies that missing Go or C tools, corrupt child reports, and timeouts cannot reuse a previous green report. It also removes the RSS all-samples guard in an isolated variant and requires the failure scenario to detect that buildable variant.

## Evidence format

Reports include commands, tool versions, environment, source and input hashes, independently specified expectations and assertion results, binary hashes, real stdout/stderr, and exit codes. Input ZIPs preserve the exact consumer and modified sources; binaries are retained under their SHA-256. Failure reports and earlier runs are preserved. Zero assertions, missing reports, and empty success entry points do not pass.

Native evidence also includes the module ZIP and manifest, consumer `go.mod`/`go.sum`, dependency manifests, runner image information, and the applicable MIT, Unicode, and Go BSD licenses. Compiler and SDK binaries are not redistributed. These artifacts are verification evidence, not a released SDK or product installer.

All evidence is generated locally under ignored `artifacts/` paths. Individual suites write `<stage>/report.json`; ablations write `<stage>-ablation/report.json`. Other default directories are `delivery`, `vendor-probe`, `native`, `native-delivery`, and `delivery-failures`. Source checkouts contain the durable inputs and expectations in `tests/consumer/`, not historical reports or binaries. Native consumer verification does not establish real Zed acceptance; each platform requires its own actual run.
