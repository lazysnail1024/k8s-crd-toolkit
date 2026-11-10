# Changelog

## Unreleased — shared file lifecycle (`toolkit-design-3`)

- Share bounded ready/running/deferred work and scan observation bookkeeping between the two language servers.
- Let consumers reuse unchanged retained index facts without maintaining another copy and its memory budget.
- Expose the authoritative open-text byte count to remove duplicate consumer counters.
- Keep resource recognition, configuration, diagnostics, worker concurrency, and LSP publication in the plugins.

Consumers pin source commits and Go pseudo-versions. This change does not publish a tag or release.

## Initial source integration (`toolkit-design-2`)

The initial prerelease was prepared but not published; the two plugins consumed the source commit directly.

### Included

- Immutable source snapshots with UTF-8 byte spans and UTF-16 position conversion.
- Fault-tolerant YAML views preserving source locations, duplicate keys, and multiple documents.
- Validated atomic text edits against one source revision.
- Incremental per-file definitions, references, reverse dependencies, stale-result rejection, and bounded symbol search.
- Authoritative open buffers, disk-read tickets, bounded priority scheduling, and resumable directory discovery.

The two initial design-ablation rounds removed redundant states and unnecessary symbol-tree requirements. Resource limits, input freshness, unresolved reverse dependencies, and foreground scheduling remain part of the public contract.

### Integration requirements

- Go 1.27.3. `yamlview` also needs `CGO_ENABLED=1` and a native C compiler; the other public packages do not require CGO.
- Pin the module version and retain `go.sum`. Contract-based consumers must also pin the byte-identical `CONTRACT.md` and its SHA-256.
- This is a source library. Plugins still implement resource recognition, Argo reference binding, LSP adaptation, worker execution, and editor integration.
- Alpha APIs may change incompatibly in later versions. Such changes require documented contract revisions and explicit consumer upgrades. Existing version tags will not be moved.

### Validation scope

The public consumer covers ten suites and 470 assertions, including real Argo YAML fixtures, input races, incremental invalidation, discovery continuation, and capacity limits. CI runs native consumers on Linux amd64/arm64, macOS amd64/arm64, and Windows amd64, plus vet, race detection, and thirteen buildable fault-injection variants. Release evidence must identify the exact tested commit and successful CI run; this matrix is not an assertion that downstream plugins have adopted the library or met editor startup latency targets.
