# Changelog

## v0.0.1-alpha

First Go module prerelease of contract `toolkit-design-2` for independent plugin integration.

```sh
go get github.com/lazysnail1024/k8s-crd-toolkit@v0.0.1-alpha
```

### Included

- Immutable source snapshots with UTF-8 byte spans and UTF-16 position conversion.
- Fault-tolerant YAML views preserving source locations, duplicate keys, and multiple documents.
- Validated atomic text edits against one source revision.
- Incremental per-file definitions, references, reverse dependencies, stale-result rejection, and bounded symbol search.
- Authoritative open buffers, disk-read tickets, bounded priority scheduling, and resumable directory discovery.

The two design-ablation rounds removed redundant states and unnecessary symbol-tree requirements before this release. Resource limits, input freshness, unresolved reverse dependencies, and foreground scheduling remain part of the public contract.

### Integration requirements

- Go 1.27.3. `yamlview` also needs `CGO_ENABLED=1` and a native C compiler; the other public packages do not require CGO.
- Pin the module version and retain `go.sum`. Contract-based consumers must also pin the byte-identical `CONTRACT.md` and its SHA-256.
- This release is a source library. Plugins still implement resource recognition, Argo reference binding, LSP adaptation, worker scheduling, and editor integration.
- Alpha APIs may change incompatibly in later versions. Such changes require documented contract revisions and explicit consumer upgrades. Existing version tags will not be moved.

### Validation scope

The public consumer covers ten suites and 470 assertions, including real Argo YAML fixtures, input races, incremental invalidation, discovery continuation, and capacity limits. CI runs native consumers on Linux amd64/arm64, macOS amd64/arm64, and Windows amd64, plus vet, race detection, and thirteen buildable fault-injection variants. Release evidence must identify the exact tested commit and successful CI run; this matrix is not an assertion that downstream plugins have adopted the library or met editor startup latency targets.
