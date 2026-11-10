# k8s-crd-toolkit

A Go module for immutable text, YAML structure, validated edits, incremental symbol indexes, and bounded workspace discovery. It contains no Kubernetes or Argo domain rules, cluster access, or user-file writes.

## Usage

Add the first Go module prerelease explicitly:

```sh
go get github.com/lazysnail1024/k8s-crd-toolkit@v0.0.1-alpha
```

Keep the selected version in `go.mod` and the generated checksums in `go.sum`. Import the packages under `github.com/lazysnail1024/k8s-crd-toolkit`:

- `source`: immutable snapshots, UTF-8 byte spans, UTF-16 positions, and sequential changes.
- `yamlview`: fault-tolerant YAML parsing that preserves original spans, duplicate keys, and document boundaries.
- `edit`: atomic replacements with revision, expected-text, and overlap validation.
- `index`: per-file symbol/reference summaries, reverse dependencies (including missing targets), stale-result protection, and bounded symbol search.
- `workspace`: authoritative open buffers, disk-read tickets, a bounded priority queue, and resumable directory discovery.

See [the public API contract](CONTRACT.md) for signatures, limits, and error semantics, [the integration guide](docs/workspace-index.md) for lifecycle and scheduling examples, and [the release notes](CHANGELOG.md) for version details. Version `v0.0.1-alpha` implements contract `toolkit-design-2`, adding the two new packages without changing the three original package contracts. This is an alpha release: later prereleases may introduce documented incompatible changes. Consumers must explicitly update their contract snapshot and implementation lock; existing plugins do not acquire indexing automatically. For development before a future release, local integration can use an explicit content snapshot:

```sh
python3 tools/snapshot.py --output ./artifacts/module-proxy
```

The generated manifest identifies the local module version, contract, source files, and ZIP by digest; it marks the snapshot as unpublished. This local snapshot is separate from the published tag and its Go module checksums.

## Requirements and build

Use Go 1.27.3. Consuming `yamlview`, or building all packages, also requires a native C compiler and `CGO_ENABLED=1`. The `source`, `edit`, `index`, and `workspace` packages can be used without CGO. Python 3 is required only for this repository's verification and snapshot tools.

```sh
GOWORK=off CGO_ENABLED=1 go build ./...
python3 tests/native.py --expected-platform linux/amd64
```

Choose the native platform for your machine: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, or `windows/amd64`. The consumer runner rejects cross-compilation. See [verification instructions](tests/README.md) for evidence output and isolated delivery checks. A build alone does not validate consumer behavior or editor integration.

For vendored `yamlview` consumers, `go mod vendor` needs the grammar's `tree_sitter` headers and go-tree-sitter's `include` and `src` directories restored alongside the vendored sources; [the vendor probe](tests/vendor_e2e.py) demonstrates this requirement.

## Workspace behavior

Keep current-file work independent of background discovery. Accept open buffers first, process related directories in small batches, and queue the remaining workspace with bounded concurrency. The queue coalesces URI notifications; the consumer dispatches the latest accepted snapshot and its original index ticket. Index facts are detached summaries, so closed-file text and parser trees can be released.

`index` stores flat definitions and references, including files with references but no declarations. It preserves multiple definition candidates and never decides namespace, resource identity, or whether a rename is safe. `workspace` does not install watchers or start workers. A discovery pass being complete and an index having no pending files are separate facts; read failures, running work, and changed configurations still need to be accounted for by the consumer.

The public consumers exercise actual Application/AppProject and Workflow/WorkflowTemplate YAML fixtures, input races, incremental invalidation, directory continuation, capacity limits, and buildable fault-injection variants. These verify the shared mechanisms, not full Argo language-server behavior or Zed startup latency.

[Design ablations](docs/design-ablation.md) record which duplicate states and model requirements were removed, which algorithms were simplified, and which proposed deletions failed the resource or responsiveness requirements. The alpha APIs omit Queue.Generation and the index's Parent/From tree requirements; the three original package contracts remain unchanged.

## License and attribution

Original code and documentation are licensed under [MIT](LICENSE). Retain the copyright and license notices when redistributing copies or substantial portions.

| Dependency | Pinned version | License |
|---|---|---|
| [go-tree-sitter](https://github.com/tree-sitter/go-tree-sitter) | v0.25.0 | MIT |
| [tree-sitter-yaml](https://github.com/tree-sitter-grammars/tree-sitter-yaml) | a1c4812a73ec; vendored with a bounded scanner serialization patch | MIT |
| [go-pointer](https://github.com/mattn/go-pointer) | v0.0.1; indirect dependency | MIT |

See [grammar provenance and patch details](yamlview/internal/parser/grammar/README.md). Tree-sitter's Unicode code retains its [separate copyright and license](https://github.com/tree-sitter/go-tree-sitter/blob/v0.25.0/src/unicode/LICENSE). Third-party licenses are not replaced by this project's MIT license. Source and binary distributions must include the licenses and applicable notices of their actual components. Dependency content is locked by `go.mod` and `go.sum`.
