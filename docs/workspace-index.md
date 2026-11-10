# Integrating workspace discovery and semantic indexes

The shared library provides mechanisms that both existing consumers need. The
Argo CD consumer needs Project definitions and callers, while the Workflows
consumer needs shared templates, callers, and signature dependencies. Both need
open buffers to win over disk, current-file work to run before a large scan, and
old background results to be rejected. Product interpretation remains in each
consumer.

The normative API is [CONTRACT.md](../CONTRACT.md). This guide describes how to
compose those APIs; it does not add another contract or an application framework.

## Ownership

| Library mechanism | Consumer responsibility |
|---|---|
| `source` text and coordinates | URI normalization and editor-version validation |
| `yamlview` syntax and source locations | Recognize resources, decode scalars, project aliases/overrides, resolve domain scope |
| `workspace.Documents` buffers and read tickets | Read bounded disk content, accept events in order, handle read failures |
| `workspace.Scanner` resumable membership discovery | Roots, exclusions, extensions, watcher/reconciliation policy |
| `workspace.Queue` coalescing and fair service | Worker concurrency, foreground execution slots, backpressure |
| `index` summaries, candidates, reverse dependencies | Keys, signatures, bindings, diagnostics, LSP features, safe refactoring |
| `edit` one-snapshot text preconditions | Multi-file edit planning and client-side application/version checks |

The runtime never imports either Argo repository. Domain names appear only in
documentation and fixed consumer fixtures. There is no parser plugin registry,
generic configuration bag, automatic schema selection, or domain callback.

## Accept events before starting analysis

Use one consumer coordinator (an event loop or equivalent synchronization) to
order effective-input changes and their index announcements. Documents and Index
are each thread-safe; operations spanning the two are not a single transaction.

1. On an editor open/change, update Documents and obtain the authoritative
   Snapshot. Reject an out-of-order editor version in the protocol adapter before
   calling Change. Call Index.Begin with that snapshot's URI/revision and the
   consumer's scope. Retain the accepted snapshot and original ticket together,
   then enqueue its URI at Current priority.
2. A disk worker obtains a read ticket before I/O. ErrOverlay means disk is not
   authoritative; retrieve the open snapshot instead. Bound the actual read, not
   only the size checked on publication. On I/O failure or cancellation, release
   only that ticket with DiscardRead. URI-wide Invalidate belongs to authoritative
   disk events and could otherwise revoke a newer read started by another worker.
3. Return the read result to the coordinator. PublishRead rejects a result made
   obsolete by open/change/close, a newer read, or invalidation. For an accepted
   snapshot, call Index.Begin and retain the snapshot and ticket before scheduling
   its URI for analysis. PublishRead does not retain closed-file content.
4. Analysis runs outside the coordinator and index lock, using only its immutable
   snapshot, version/configuration selection, and original index ticket. It
   produces detached Facts and calls Commit. Never obtain a replacement ticket
   after finishing analysis of old content. After a successful commit, release
   the accepted input only if it is still the same work item; completion of an
   older worker must not discard a newer accepted input.
5. Schedule Change.Affected for domain recomputation. References to missing
   targets must already be registered, so a target created later can wake them.

For an affected file whose accepted input has been released, obtain its open
snapshot or perform a fresh bounded disk read, accept the input, and call Begin
before enqueueing analysis. Missing temporary input does not prove that a file
was deleted. An authoritative discovery, watcher, or recomputation request can
create new accepted work; an old analysis notification cannot do so by itself.

File A changing does not revoke file B's ticket. A configuration change can call
Begin again with the same source revision; its new ticket supersedes earlier
analysis. Changes to global settings must be announced for the affected inputs
by the coordinator before old configuration results can be accepted.

If parsing, reading, or admission fails, retain an explicit failure/pending state.
Do not Commit empty facts to make a failed input appear valid. Empty facts are
appropriate only after successful recognition establishes that the file now has
no owned definitions or uses. If Begin itself cannot admit an updated input,
withdraw obsolete facts with Remove or otherwise prevent their presentation as
current while retaining the admission failure in the consumer's state.

## Disk deletion, closing, and moving files

When a disk file changes or disappears, Invalidate its pending disk read. If an
open buffer exists, it remains authoritative. Do not remove its symbols just
because disk is absent.

When the buffer closes, Close releases its text and revokes old read tickets. A
fresh disk read then supplies the effective input. If that read establishes that
the file no longer exists, remove its facts. There is deliberately no cached disk
snapshot to resurrect after a close.

For a removed workspace root, the coordinator can close affected inputs, remove
their queued jobs and facts, and dispose of the associated scanner. Moving a
file changes its URI; the consumer handles removal and addition explicitly.
Resource-name references need no textual rewrite merely because a source file
moved. Actual path references and transactional editor operations remain domain
and protocol responsibilities.

## Keep foreground work available

The Queue offers Current, Related, and Background service in an 8:4:1 weighted
cycle. It stores URI notifications and coalesces pending notifications at the
highest requested priority. Taking a job releases its pending slot immediately,
so an update that arrives while old work runs is not lost.

When dispatching a URI, obtain its latest accepted snapshot and original index
ticket from the coordinator. The coordinator already needs this input: Queue
never owns snapshots, and Documents retains only open buffers. A late notification
therefore selects current work or, if there is no accepted input, is ignored.
Deleting a file also discards its accepted work and calls Index.Remove; a later
notification alone cannot restore it. Queue does not assign another generation
or decide whether a worker's result is current. Documents and Index tickets
protect the separate read and analysis publication stages.

The queue does not interrupt work already running. Keep a foreground execution
opportunity available, bound background concurrency, and cancel superseded work
where possible. OS I/O may remain blocked until the syscall returns; ticket
checks are still needed even when cancellation was requested.

On ErrLimit, preserve the event and pause discovery or retry later. In particular,
do not consume a Scanner batch, silently drop paths that did not fit the Queue,
and later claim that discovery and analysis completed successfully. Keep the
unaccepted part of the batch until it is admitted.

## Discovery is a resumable cursor

Create a Scanner with the consumer's roots, `.yaml`/`.yml` extensions if desired,
explicit excluded directory basenames, and a directory capacity. Construction
normalizes paths without opening or enumerating those roots. Call Next with a small work budget, process the returned
Files, and give the scheduler another opportunity before the next batch.

The budget includes ordinary files and directory operations, not only matching
YAML files. A large flat directory of unrelated files therefore yields regularly.
The cursor keeps its position across calls and cancellation. A canceled call may
return valid paths: process its Batch before handling the error. Close a scanner
that will no longer be used so its handles are released.

Prioritize can visit a related directory before its parents have been fully
enumerated. An active background directory is paused without rewinding it; after
priority discovery finishes, it resumes from the retained cursor. At most two
handles and 512 directory entries are buffered. An already active priority
directory is not preempted by another one, so this is bounded cooperative work,
not an elapsed-time guarantee.

Each Scanner enumerates a directory at most once. For a later reconciliation
pass, create another scanner. Use watcher events and per-file metadata policy in
the consumer to decide which discovered files need content reads. This library
does not perform periodic full-content rereads or promise a filesystem snapshot.

## Represent semantic facts without retaining source text

Choose a scope based on the consumer's actual visibility rules. A scope may
include several Git repositories. Namespace and resource kind belong in the
consumer's key construction, and directory proximity only affects scheduling.

Each file submits a complete Facts set. Multiple YAML documents receive distinct
file-local SymbolIDs. Parent links support domain outlines and From links identify
the owner of a reference/call. Source spans always identify the actual input
bytes, including a source origin selected by a projection.

| Domain need | Consumer-produced summary |
|---|---|
| Application references Project | Project key definition and matching reference key |
| Template refers to another template | Template definition key, call owner, call-site span |
| A template's input/output contract changes | Updated SemanticDigest |
| A known local path affects validation | Opaque Dependencies key, even without a navigable reference |
| A comment shifts a definition | New source spans/revision, unchanged semantic digest and fact shape |

SemanticDigest includes meaning absent from Symbol and Reference fields. It must
be computed from relevant semantic input, rather than the whole text, so comments
do not unnecessarily invalidate every caller. Conversely, a digest of names alone
would miss policy and signature changes. The index compares relation/identity
shape as well, so an unchanged digest cannot hide a changed reference key.

Definitions preserve all candidates. The consumer decides whether namespace,
resource kind, scope, and dynamic content permit a unique binding. Reverse
dependencies exist before binding succeeds; adding a missing definition or a
second candidate therefore schedules the existing callers.

Committed strings are detached from their incoming backing strings. Do not put
entire YAML text into Name, Kind, or Key; the byte budget is a guard, not a reason
to treat the index as a document cache. Release parser views and closed-file
snapshots after extraction. Keep open snapshots and any necessary bounded cache
under the consumer's separate memory policy.

## Completeness and edits

`Scanner.Complete` means that one discovery pass finished within its traversal
policy without observed errors/limits. `Index.State.Pending` counts known facts
awaiting publication. Neither alone establishes a complete semantic scope.

Before asserting absence, unique binding, all references, or a safe multi-file
rename, the consumer accounts for discovery status, queued and running work,
read/parse failures, configured exclusions, and effective input/configuration
versions. Known positive candidates can be useful sooner. A failed or canceled
scan must not cause bulk deletion of unseen files; an older reconciliation pass
must not delete files first observed after it began.

Position-only changes avoid semantic recomputation of callers, but navigation
caches must still honor scope generations and source revisions to detect moved
locations. Index query results carry both. Several
queries are separate snapshots: compare generations when combining them and
revalidate input versions before producing or applying edits. The `edit` package
still operates on one snapshot, without disk writes or cross-file transactions.

## Verification and adoption

The independent public consumer tests use fixed Application/AppProject and
Workflow/WorkflowTemplate YAML inputs through source, yamlview, and index. They
exercise unresolved references, candidate ambiguity, deletion, input/output
signature changes, actual source positions, and concurrent fact replacement.
Workspace tests use real directories, ordinary-file-heavy discovery, priority
interleaving, cancellation/resume, open buffers, stale reads, and capacity errors.
Buildable guard mutations verify that removing key protections causes the
designated behavioral assertion to fail. Separate
[design ablations](design-ablation.md) compare simpler working implementations
against the same requirements, including bounded query allocation and related
directory responsiveness.

See [tests/README.md](../tests/README.md) for commands and retained evidence. These
tests establish the mechanisms and packaging on the tested platform. They do not
establish plugin startup latency, full Argo semantics, or real Zed UI behavior.
Those require each consumer to adopt this contract and implementation explicitly
and then run its native CLI/stdio/editor acceptance suite.
