# k8s-crd-toolkit v0.0.1-alpha.2

The public alpha tag named by `VERSION` is delivered as a **Go module source dependency**; it contains no standalone executable. This tag uses the `toolkit-design-6` contract. This file describes the scope of the tag; for the current implementation on `main`, see the [README](README.md), the [public contract](CONTRACT.md) and the [architecture](docs/architecture.md).

## Capabilities

- Immutable source snapshots, UTF-8/UTF-16 coordinate conversion, and validated sequential and same-snapshot edits.
- Fault-tolerant multi-document YAML structure with original source spans; strict effective values with bounded alias/merge projection and provenance.
- Bounded field, type and candidate queries over caller-compiled JSON Schema graphs.
- Incremental definition/reference summaries, cancellable work-bounded symbol queries, stale-result rejection and dependency invalidation.
- Authoritative open buffers, bounded work queues, batched directory discovery and reconciliation after a complete scan.

## Changes since v0.0.1-alpha.1

### Contract `toolkit-design-6`

`toolkit-design-5` (used by `v0.0.1-alpha.1`) is superseded by `toolkit-design-6`. No public Go declaration changed. The revision documents native parser execution:

- The native parser lazily starts at most two private workers pinned to OS threads and reuses them for the process lifetime, bounding native allocator thread ownership. They only execute native parsing; application analysis, discovery and Go view construction stay on the caller.
- Admission observes context cancellation. Once admitted, cancellation is handled by the native parser and all per-call native resources are released before `yamlview.Parse` returns. Idle workers retain no caller text or context.
- Native parsing yields between 10 ms timeout slices, so one slow call does not occupy a worker for its whole parse.

Consumers upgrading from `toolkit-design-5` must still update their contract snapshot and implementation lock explicitly.

### Performance and simplification

These changes preserve the public contract, input/work/depth budgets, cancellation and safety guards:

- `yamlview`: copying a parse tree avoids redundant native child scans, per-node kind-name allocations and error queries on clean subtrees; tree relocation and issue collection are skipped when unnecessary.
- `yamlvalue`: proven plain mapping keys skip redundant strict decoding; maps and oversized-scalar recursion are allocated or visited only when needed. A duplicate snapshot check that `Document.ContentSpan` already enforces was removed.
- `edit`: validated replacements are applied in a single linear pass instead of a quadratic sequence of intermediate strings.
- `source`, `schemaquery`, `index`, `workspace`: lower allocation line indexing, bitset type summaries, direct scope traversal, and fewer filesystem metadata calls during discovery, while root and prioritized paths keep their per-component symlink checks.
- Local paired measurements on Linux amd64 (Go 1.27.3, synthetic workloads) showed, for example, a 200-replacement edit about 68x faster with 99% fewer allocated bytes, and repeated YAML parsing about 1.4x faster with roughly half the allocated bytes. These figures describe those workloads, not a latency guarantee for arbitrary inputs.

### Usability

- Edit and scanner errors now report the mismatched revisions, overlapping spans, invalid extension or exclusion values, and the excluded component of a rejected priority path. Error text remains descriptive rather than a stable protocol.

### Verification

- Deterministic fuzz seeds on all five native platforms, Linux mutation fuzz campaigns, property relations, registered behavior ablations and pinned downstream LSP consumers run in the full CI plan. Evidence is bound to the commit, run attempt, lane and platform.

## Integration requirements and boundaries

- Go 1.27.3; the YAML parser requires CGO and a C compiler for the target platform.
- Native verification platforms: Linux amd64/arm64, macOS amd64/arm64 and Windows amd64.
- The library provides no standalone LSP, Zed extension, Argo-specific rules, schema registry or cluster access.
- Publishing a tag does not upgrade either plugin. Each records its actual implementation commit and contract in `contracts/toolkit.lock.json`; see the [consumer boundary](docs/architecture.md#%E4%B8%A4%E4%B8%AA%E6%8F%92%E4%BB%B6%E7%9A%84%E5%AE%9E%E9%99%85%E5%BC%95%E7%94%A8%E4%B8%8E%E9%9A%94%E7%A6%BB).
- Vendoring must retain the grammar's tree-sitter headers, the go-tree-sitter native include and source directories, and third-party licenses and provenance.

## Relationship between releases and ongoing development

Published tags and releases keep their original contents. Later changes on `main` do not enter `v0.0.1-alpha.2`, and they do not claim that newer tests were run against this tag. The release workflow creates a release only after full main-branch acceptance, only while the accepted commit is still the remote `main`, and only when the tag is unused; existing releases are never overwritten.

A new version must update `VERSION` and this file explicitly and run the current acceptance process in the [verification guide](tests/README.md). This project is still alpha; the public interface is not yet stable.
