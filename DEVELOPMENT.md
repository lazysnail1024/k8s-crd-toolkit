# Development and acceptance

Use the exact Go version in `go.mod`, a C compiler, Python 3, `GOWORK=off`,
and `CGO_ENABLED=1`. The native workflow retains all five native platforms,
public consumer stages, race checks, source delivery controls and mutations.
`docs/testing-ci-baseline.json` pins the original inventory, source hashes,
resource constraints and mutation oracles to the immutable baseline commit.

Run `python tests/native.py --expected-platform linux/amd64 --source-commit
$(git rev-parse HEAD)` for native public API acceptance. Run
`python tests/e2e.py properties` for bounded input/overlay/index equivalence,
and `python tests/fuzz.py --seconds 60` for three actual Go mutation campaigns.
`python tests/fuzz_seeds.py --platform linux/amd64` runs deterministic seed
replay; the native workflow executes this separately on all five platforms.
Each command writes replayable evidence below `artifacts/`; fuzz stdout and
failure corpora are retained even when a campaign fails. A successful seed
replay or build alone is not a completed fuzz campaign. Periodic CI uses
600 seconds per target; PRs use 60 seconds, two workers and 30 seconds of
minimization. Replay a retained failure with Go's printed `-run` command.

`python tests/downstream.py` clones the two immutable consumers in
`tests/downstream-pins.json`, injects the candidate runtime into their vendor
trees, verifies actual compiler input paths and bytes, then executes real LSP
projection, editing and lifecycle consumers. The derived sources are explicitly
`downstream-candidate`; they never replace downstream checkouts or publish a
release. `--checkout-cache DIRECTORY` may use local clones only as Git object
sources. Both reports must bind the same three-SHA tuple. Use a fresh `--output`
directory for every attempt. Candidate runtime bytes must match its commit.

`Toolkit required` verifies all selected jobs and same-attempt reports. Partial
reruns with older producer evidence require rerunning the complete workflow.
Missing, duplicate, stale or incomplete reports fail. The selector understands
an explicit prose allowlist, but the workflow forces full execution until hosted
selection controls are accepted. Main and scheduled runs remain full.

Verifier controls: `python tests/ci_plan_e2e.py` and
`python tests/ci_receipt_e2e.py`, `python tests/ci_negative_e2e.py`, and
`python tests/downstream_receipt_e2e.py artifacts/downstream/report.json`. After the implementation stage, run existing
`python tests/ablate.py`, then `python tests/fuzz_effectiveness.py`.
The fuzz effectiveness runner requires a compiled fuzz execution, exit code 1,
and the designated behavior failure; build errors and infrastructure failures
are not detected defects. `python tests/fuzz_effectiveness_e2e.py` replays
process outcomes through the runner as reporting controls; it does not run or
claim a product mutation campaign. Downstream
controls use `--control old-vendor`, `wrong-sha`, `missing-file`, or
`missing-consumer`; each must exit unsuccessfully and preserve its report.

`tools/ci_trends.py` consumes explicit warm/cold raw timing samples with run,
attempt, SHA and evidence references. Each sample also declares `platform`,
`toolchain`, `selection`, `fixture`, `resources`, `variant` (`baseline` or
`candidate`), `passed`, nonempty `phases`, and an optional `pairID`. Failed
samples declare `failureType`. Grouping preserves those execution dimensions
and success/failure classification. Five warm pairs or three cold pairs must
have both variants and alternate execution order before supporting median and
outlier inspection; unpaired samples remain insufficient. Reports preserve raw
samples and first failures and omit p95 estimates. These counts alone never
establish optimization acceptance or replace the full matrix. Run
`python tests/ci_trends_e2e.py` for the external reporting controls. The tool
never substitutes CI duration for product measurements or changes limits.
Hosted matrix, branch protection, seven-day observations and real Zed host
acceptance are separate evidence, not claims established by local scripts.

Property coverage maps to the seven P08 relations as follows:

| Relation | Toolkit evidence and boundary |
| --- | --- |
| Incremental versus final load | `properties`: `property-<seed>-change-<step>` and `property-<seed>-final-load` compare sequential immutable text against independently constructed final text. |
| Cold versus reused facts | `properties`: `property-<seed>-warm-cold`; native `index`: `reuse-restores-new-revision`, `reuse-advances-generation-without-caller-reparse`, and `reuse-does-not-copy-large-summary` exercise explicit `Index.Reuse`, its semantic-change result and allocation boundary. This does not claim an implicit parser cache hit. |
| Edit and undo | The source/edit stages check independently specified edited bytes and atomic rejection. Full domain diagnostic/navigation restoration belongs to the two downstream LSP consumers; toolkit does not produce those domain results. |
| Target version isolation | Not a toolkit runtime capability: Argo target/schema selection belongs to the pinned downstream consumers. Toolkit scopes and instance identities cannot certify Argo A/B/A version switching. |
| Scope and root isolation | `properties`: `property-<seed>-scope-isolation`; native `index`: `scope-isolates-same-key` and `reuse-rejects-different-scope`. Filesystem roots are a consumer policy, not an index namespace inferred by toolkit. |
| Cancellation recovery | Native `index` exercises `query-mid-traversal-cancel-discards-items`; native `workspace` coordinates newer accepted input after obsolete work (`coordinator-late-worker-cannot-overwrite`). Full LSP cancellation/final cold equivalence remains downstream evidence. |
| Repeated events | Native `index`: `repeated-begin-not-double-pending`; native `workspace`: `coordinator-pending-changes-coalesce`, `coordinator-old-notice-after-commit-safe`, and `coordinator-completed-input-not-recreated`. |

The table identifies ownership and existing checks; it does not upgrade narrower
library assertions into unexecuted protocol or target-version acceptance.
