# Design ablation of the workspace index

This document preserves two experiment rounds. The first reduces duplicate
state; the [second](#second-round-flat-facts-and-shorter-scanner-lifetimes)
removes the shared symbol-tree requirement and shortens scanner object lifetimes.
Counts and API comparisons belong to their named source revisions.

The 2026-11-10 experiment starts from commit
[`3121b84aa4ce5f63588ba5f2083ce3e80f71cdf7`](https://github.com/lazysnail1024/k8s-crd-toolkit/commit/3121b84aa4ce5f63588ba5f2083ce3e80f71cdf7),
the first implementation of the proposed, unreleased `toolkit-design-2`
contract. It asks whether simpler working implementations preserve the actual
consumer requirements. Making a deliberately broken guard fail a test is useful
regression evidence, but does not answer that design question.

Each candidate runs outside the working implementation against the same public
consumer source. Acceptance requires correct locations and invalidation, bounded
retained work, and current/related work that can run before background discovery
finishes. Fewer lines alone are not an acceptance criterion.

## Decisions applied

| Candidate | What was removed or replaced | Evidence and decision |
|---|---|---|
| Remove private `entry.fresh` | A second representation of whether the current ticket has been consumed | `serial != 0` means pending; a successful commit sets it to zero. The public `File.Fresh` result remains. Adopted. |
| Merge scope membership and pending sets | Two URI maps for the same scope | One `map[URI]bool` records membership and pending status; a count keeps state queries O(1). Repeated Begin and scope migration remain correct. Adopted. |
| Use `container/heap` with typed initial appends | Two hand-maintained sift loops | Append the first k values directly, initialize the heap on overflow, and use `heap.Fix` for replacements. Retains bounded top-k storage. Adopted. |
| Remove `Job.Generation` | Queue-local input ordering and stale-generation rejection | Queue stores URI notifications and priority only. The consumer selects its latest accepted snapshot and original ticket. Adopted; this changes the proposed Queue API. |
| Remove `directoryWork.done` | A completion flag duplicating cursor/list membership | Completed and failed directories remain in `seen`, without a pending list element. Prioritizing them cannot re-enqueue them. Adopted. |

The index variants, including the combined implementation, passed **163/163**
assertions with the same enhanced consumer. Both the baseline and combined index
also passed that consumer with Go's race detector. Removing `fresh` did not
produce a measurable retained-memory saving; its benefit is one fewer state to
keep synchronized. The three index changes together remove only one net line.

The Queue and Scanner changes passed the same **30/30 requirement assertions**
separately, together, and with the adopted index changes. These include edits
while pending or running, late notifications and workers, deletion and reopen,
releasing completed input, priority promotion, fairness, cancellation, and
finished/failed directory handling. The original experimental coordinator source
was byte-identical across Queue implementations (SHA-256
`6e4ef833f7624936750f112a4c2c7b49b9b9381351d11d7e0310220cc2634610`).
The old Queue adapter additionally needed a generation counter; the new one
does not.

### Why Queue simplification does not move version management elsewhere

The old Queue never owned snapshots or index tickets. The consumer already had
to retain the accepted input while pending or running, including closed-file
snapshots that `Documents.PublishRead` deliberately does not cache. That same
input ownership is sufficient for URI notifications. There is no replacement
queue clock or generation map.

The old generation comparison only applied while a URI was pending. Pop removed
that membership, so the queue could not decide whether a running result was
still current. The original Documents and Index tickets already made the final
read and analysis publication decisions. Those protections remain.

Completion releases an accepted input only if it is still the same work item.
An old notification then either selects current accepted work or finds none.
It cannot create input. An authoritative discovery, watcher, or recomputation
request can obtain and accept input again; missing temporary text does not mean
the file was deleted. The integration guide specifies that distinction.

On the measured amd64 platform, `Job` changed from three fields and **32 bytes**
to two fields and **24 bytes**. This is a struct-size measurement, not a claim
about total process memory or plugin startup time. The contract is still
unreleased; consumers of the proposed old Queue API must remove the Generation
field. The original `source`, `yamlview`, and `edit` contracts are unchanged.

## Alternatives rejected by the experiments

### Collect all symbols, sort, then truncate

This replacement passed the old **148/148** index assertions and returned the
correct ordered top-k results. It still failed the resource requirement:

| Workload: 50,000 symbols, eight queries | Retained top-k algorithm | Collect all, sort, truncate |
|---|---:|---:|
| k = 10, temporary total allocation | 37,440 B | 266,984,032 B |
| k = 100, temporary total allocation | 318,016 B | 266,984,032 B |

At k=10 this is approximately **7,131 times** the allocation. These are total
allocated bytes during the queries, not retained heap or RSS. Correct final
results alone did not detect the regression.

The durable consumer now tests ordering across URI, span start/end, and symbol
ID, multiple limits including k=1, exact limits, pending exclusions, and scope
exclusions. Its separate allocation check queries 20,000 symbols four times at
k=10 with a generous **1 MiB** ceiling. Against the same strengthened consumer,
the baseline allocated **18,720 B**; collect-all allocated **50,097,728 B** and
failed that specific assertion while compiling and passing the other 162.
The adopted combined implementation allocated **18,960 B** in the final run.

### One scanner cursor

A complete single-cursor alternative still enumerated all files exactly once.
It failed the requirement that a newly prioritized related directory become
available within the next **four work units** after starting a background
directory containing 1,200 unrelated files: **29/30** assertions passed, and
related-directory responsiveness failed. The baseline and simplified two-cursor
implementations passed **30/30**.

Keep one active cursor and at most one paused background cursor. This permits
related work without restarting background enumeration, with at most two open
directory handles and 512 buffered directory entries. Work units are not a
wall-clock latency guarantee; operating-system calls can block.

The durable scan test now processes at least 512 background work units before
prioritizing another directory. It requires the current file promptly, all 801
candidates exactly once, and at most 1,800 total visited work units. Both baseline
and final implementation observed **1,608**. That last bound detects restarting
the processed prefix even if output deduplication hides repeated traversal.

### Direct `heap.Push` for every initial item

This standard-library version passed the behavioral assertions but introduced
interface boxing for the first k values. It was replaced with typed append plus
`heap.Init`/`heap.Fix`:

| Eight queries over 50,000 symbols | k=10: allocated bytes / allocations | k=100: allocated bytes / allocations |
|---|---:|---:|
| Original custom heap | 37,440 / 64 | 318,016 / 88 |
| Direct `heap.Push` | 49,408 / 160 | 433,664 / 904 |
| Typed append, Init and Fix | 37,888 / 80 | 318,464 / 104 |

The adopted adapter adds **56 bytes and two allocations per query** in this
probe, independent of N and k in the tested workloads. It removes the custom
sift algorithm without changing the O(k) temporary-result bound. The small
fixed overhead is an explicit tradeoff, not a performance improvement claim.

## Scope memory experiment

The public-API probe builds one symbol per file, measures retained Go heap after
GC, then calls Begin on every file and measures again. Three fresh processes per
variant and size were run with Go 1.27.3, Linux amd64, and `GOMAXPROCS=4`.

| 50,000 files; median retained heap above starting heap | All committed | All pending |
|---|---:|---:|
| Baseline | 39,842,944 B | 42,390,000 B |
| Remove `fresh` only | 39,842,768 B | 42,390,192 B |
| Merge scope maps only | 39,842,800 B | 40,642,592 B |

The merged map saves about **1.67 MiB** in the all-pending workload. The tiny
`fresh` differences are measurement noise. The pending count remains constant
time; it is not recomputed by scanning the merged map on every state query.

The cost matrix contained 90 samples: five implementations, two sizes
(2,000/50,000), a scope probe and two query limits (10/100), and three processes
for each case. Query measurements used two warm-ups, a GC, then eight queries
whose ordered results were checked independently. The typed-heap variant was
measured after the initial batch, so timings are not a tightly paired benchmark.
No startup or query speedup is claimed.

## Mechanisms retained and their distinct jobs

| Mechanism | Requirement it serves |
|---|---|
| Documents read ticket and Index analysis ticket | Reject obsolete results at two separate asynchronous publication stages; a configuration change may invalidate analysis without changing text. |
| Reverse dependencies for unresolved keys | Wake existing users when a previously missing definition appears or candidate ambiguity changes. |
| Facts shape comparison plus semantic digest | A reused digest cannot hide a changed definition or relation; consumer meaning beyond those fields still needs its digest. |
| Queue URI map and FIFO lists | Constant-time pending lookup, coalescing and promotion, with weighted service opportunities. |
| Two scanner cursors | Related-directory service without losing background traversal position. |
| Per-call work budget and directory-capacity limit | Bound a single scheduling turn and total remembered traversal state, respectively. |
| `Done` and `Complete` | Distinguish exhausted traversal from successful coverage; an I/O or capacity failure cannot prove absence. |
| Detached facts and byte/count limits | Release source backing strings and bound different retained resources. |

These responsibilities do not justify a new worker framework, parser registry,
domain callback interface, or global configuration system. None was added.

## Reproduction and regression gates

Run the first-round public consumer against both historical runtimes using the
existing runner. That consumer passed **163/163 index** and **192/192 workspace**
assertions against each. The workspace total includes the 30 requirement cases
and nine setup assertions added to the prior 153-assertion suite. The Queue tests
use named URI/Priority fields so the same source compiles against either API;
they verify retained requirements, not the intentionally removed generation
rejection contract.

```sh
baseline_dir=$(mktemp -d)
round1_dir=$(mktemp -d)
git archive 3121b84aa4ce5f63588ba5f2083ce3e80f71cdf7 | tar -x -C "$baseline_dir"
git archive 34f0d2c6afad203bfc3768b94f5b621a70ed4a26 | tar -x -C "$round1_dir"
(cd "$round1_dir" && python3 tests/e2e.py index-design-baseline "$baseline_dir")
(cd "$round1_dir" && python3 tests/e2e.py workspace-design-baseline "$baseline_dir")
(cd "$round1_dir" && python3 tests/e2e.py index-design-final)
(cd "$round1_dir" && python3 tests/e2e.py workspace-design-final)
```

The two small cost probes are retained as ordinary public-API programs, without
a new benchmark framework. To compare the first-round runtime and baseline with the
same probes:

```sh
(cd "$round1_dir" && GOMAXPROCS=4 go run ./tests/probes/index-scope 50000)
(cd "$round1_dir" && GOMAXPROCS=4 go run ./tests/probes/index-query 50000 10 8)
cp -R "$round1_dir/tests/probes" "$baseline_dir/tests/probes"
(cd "$baseline_dir" && GOMAXPROCS=4 go run ./tests/probes/index-scope 50000)
(cd "$baseline_dir" && GOMAXPROCS=4 go run ./tests/probes/index-query 50000 10 8)
```

Repeat in fresh processes for medians. The single-change variants above are
historical design experiments; these commands compare the frozen baseline with
the first-round combined runtime. The regression consumer retains the requirements
that made the choices consequential. [tests/README.md](../tests/README.md)
documents native, isolated delivery, race, and buildable guard-mutation checks.
Those checks establish library behavior on tested platforms, not adoption by
either language server or real editor startup latency.

## Second round: flat facts and shorter scanner lifetimes

The second round fixes its baseline at
[`34f0d2c6afad203bfc3768b94f5b621a70ed4a26`](https://github.com/lazysnail1024/k8s-crd-toolkit/commit/34f0d2c6afad203bfc3768b94f5b621a70ed4a26).
It tests whether the model and retained objects can shrink, as well as whether
whole data structures have simpler viable replacements. Common file state,
incremental indexing and foreground scheduling remain requirements. A shared
call tree is not needed to locate a definition or find referring files.

| Candidate actually implemented | Decision | Reason |
|---|---|---|
| Flat facts without Symbol.Parent / Reference.From | Adopt | Removes a forced symbol tree and synthetic declarations for reference-only files; navigation and invalidation still work. |
| Scanner remembers completed identities without retaining work objects | Adopt | Removes the work-to-list backpointer and releases completed work without adding consumer state. |
| One slice instead of Queue's lists and URI map | Reject | Same behavior, but linear lookup and shifting degrade foreground operations as pending work grows. |
| One outer bucket map for both definition and dependency postings | Reject | Saves memory for dense dual-role keys, increases memory for sparse keys, and adds role dispatch. |
| A separately allocated capability object for each Begin | Reject | Still needs the generation counter and adds one allocation per Begin; no consumer responsibility disappears. |

### Remove the compulsory symbol tree

The old model required every Reference.From to identify a local Symbol. A file
containing a target reference without a named declaration could not be submitted
directly. A consumer had to manufacture an owner symbol, choose a reserved ID/key,
and prevent that artificial declaration from appearing in symbol results.

The candidate removes `Symbol.Parent`, `Reference.From`, and parent existence and
cycle validation. Symbol IDs still distinguish real definitions within a file.
References contain only a target key and a source span. Existing scope isolation,
definition candidates, reverse dependencies, source revisions, tickets and atomic
replacement are unchanged. Domain outlines and call ownership remain in the
consumer's existing semantic model; this index does not require them.

Two independently built consumers used identical main source and six identical
Application/AppProject and Workflow/WorkflowTemplate YAML fixtures. Both passed
the same **44 business assertions** through the public source/yamlview/index
APIs. The assertions cover actual definition and reference spans, unresolved
uses, later target arrival, duplicates, removals, retargeting, position-only
updates and semantic invalidation. The common main SHA-256 was
`aa2554841ee06f309e39b6de92378bdc63eaeeeef4c8430711de450076dae542`.

The adapters intentionally differ:

| Adapter obligation | Old model | Flat model |
|---|---|---|
| Submit reference-only facts directly | Rejected | Accepted |
| Invent an owner and reserved identity | Required by this adapter | Unnecessary |
| Raw empty-query symbols before a real target exists | Contains the invented owner | Empty |
| Raw limit-one query after the target is created | Can return the invented owner first | Returns the real declaration |
| Preserve correct visible symbol results | Identify/filter invented owners and request enough candidates | Direct bounded query |
| Adapter code in this comparison | 46 lines / 1,577 bytes | 11 lines / 432 bytes |

Neither adapter adds a persistent map. The old comparison adapter overfetches up
to its known symbol budget; that is its disclosed implementation choice, not the
only possible repair. Iterative widening or tracking artificial owners would be
other additional responsibilities. The flat model needs none of those repairs.
The line counts describe this adapter, not total language-server integration.

The fixtures are syntax-complete and exercise callers without named declarations.
They do not establish support for arbitrary damaged YAML, aliases, generated
declarations, or every Argo binding rule. This is an intentional change to the
unreleased API. The six rejection/recovery assertions for missing parents, parent
cycles and unknown reference owners no longer describe the contract and were
removed explicitly. Positive reference-only cases replace that obligation in the
durable public consumer. The other 157 pre-existing index assertions passed the
flat candidate before those new cases were added.

### Release completed scanner work

Previously, every remembered directory retained its `directoryWork` until the
scanner closed. The replacement maps each seen path to its list element while
queued, active or paused, and to nil after completion/failure. An absent key is
unseen. It removes `directoryWork.element`; completed paths retain only their
deduplication identity. At most two detached list elements remain while cursors
are active or paused.

The same 197-assertion experimental consumer passed on baseline, scanner
replacement and slice-queue replacement. It included the existing workspace and
coordination inputs, four active/paused priority checks, and an independent
50,000-operation queue model. Only the scanner checks were added to the formal
suite, since the queue replacement was rejected.

For 6,001 directories, three fresh process runs with Go 1.27.3 on Linux amd64 gave
the following medians after a completed pass and GC:

| Measure | Baseline | Scanner replacement |
|---|---:|---:|
| Net retained Go heap | 712,528 B | 528,264 B |
| Net retained objects | 12,032 | 6,029 |
| Visited work units | 18,002 | 18,002 |
| Complete | true | true |

The object difference is consistent with releasing roughly one work object per
completed directory. These are GC-sensitive net measurements, not exact object
accounting, RSS, or plugin startup timings. No memory or scheduling state is
transferred to the consumer.

### Why the other structural replacements were rejected

The slice Queue preserves coalescing, promotion, FIFO, capacity and weighted
service; all 197 assertions pass. It also uses less retained memory. Its linear
URI search and deletion shifts, however, scale poorly. Three-run local medians
for filling a queue were:

| Pending URIs | Existing map/list Queue | Slice Queue |
|---|---:|---:|
| 1,024 | 0.254 ms | 1.611 ms |
| 8,192 | 1.475 ms | 109.830 ms |
| 32,768 | 8.496 ms | 1,800.016 ms |

With the active URI at the back of 32,768 pending items, repeated Put averaged
about 15 ns versus 106 microseconds per operation in this probe. These observations
and the O(n) lookup / O(n^2) fill paths justify retaining the lookup map and FIFO
lists; they are not universal latency promises.

The single posting-bucket alternative also passes all 163 baseline index
assertions. At 20,000 pure-definition keys it adds 262,432 B of retained heap;
with mostly single-role keys and one high-fanout key it adds 261,840 B. A dense
dual-role ring instead saves 1,312,160 B. It adds role constants/dispatch and
15 net lines, so the favorable dense fixture does not justify adopting it as a
general simplification.

The capability-token alternative passes the same 163 assertions but does not
remove the index generation needed for query state. For 20,000 superseding Begin
calls with an empty scope, the existing serial implementation allocates 0 B / 0
objects in the measured loop; separate capabilities allocate 20,000 B / 20,000
objects. A 24-character scope changes 480,000 B / 20,000 allocations to 500,000 B /
40,000 allocations. The two-line reduction does not justify replacing an already
available sequence with another allocation lifecycle.

The Documents callback proposal was reviewed but not implemented: combining
BeginRead/PublishRead/DiscardRead into a callback would retain the reservation
and acceptance phases and introduce an I/O callback. It is a design assessment,
not a completed ablation or evidence of equivalent behavior.

### Second-round integration gates

The adopted flat model and scanner changes pass the full native content-snapshot
consumer: **10 suites / 470 assertions**, including **181 index** and **198
workspace** assertions. The independent offline delivery consumer passes with a
clean module cache and no replace directive. Both focused suites also pass with
`-race`; the workspace coordination cases retain their explicit serial event
ordering and do not claim editor-level concurrency coverage.

All **13** existing index/workspace fault variants build successfully, then exit
with their designated behavior assertion failing. `index-missing` also fails the
new reference-only target-arrival cases. A compiler error, timeout or crash is not
accepted as detecting a fault. The durable inputs and runner remain in the
existing consumer and ablation tools; no new testing framework was added.
