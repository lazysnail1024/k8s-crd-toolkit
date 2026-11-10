# k8s-crd-toolkit

A Go module for immutable text, YAML structure, validated edits, incremental symbol indexes, and bounded workspace discovery. It contains no Kubernetes or Argo domain rules, cluster access, or user-file writes.

## Usage

Import the packages under `github.com/lazysnail1024/k8s-crd-toolkit`:

- `source`: immutable snapshots, UTF-8 byte spans, UTF-16 positions, and sequential changes.
- `yamlview`: fault-tolerant YAML parsing that preserves original spans, duplicate keys, and document boundaries.
- `edit`: atomic replacements with revision, expected-text, and overlap validation.
- `index`: per-file symbol/reference summaries, reverse dependencies (including missing targets), stale-result protection, and bounded symbol search.
- `workspace`: authoritative open buffers, disk-read tickets, a bounded priority queue, and resumable directory discovery.

See [the public API contract](CONTRACT.md) for signatures, limits, and error semantics, and [the integration guide](docs/workspace-index.md) for lifecycle and scheduling examples. Contract `toolkit-design-2` adds the two new packages without changing the three original package contracts. No formal version or tag has been released. Consumers must explicitly update their contract snapshot and implementation lock; existing plugins do not acquire indexing automatically. Local integration can use an explicit content snapshot:

```sh
python3 tools/snapshot.py --output ./artifacts/module-proxy
```

The generated manifest identifies the module version, contract, source files, and ZIP by digest; it marks the snapshot as unpublished.

## Requirements and build

Use Go 1.27.3, a native C compiler, and `CGO_ENABLED=1`. Python 3 is required for the verification and snapshot tools.

```sh
GOWORK=off CGO_ENABLED=1 go build ./...
python3 tests/native.py --expected-platform linux/amd64
```

Choose the native platform for your machine: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, or `windows/amd64`. The consumer runner rejects cross-compilation. See [verification instructions](tests/README.md) for evidence output and isolated delivery checks. A build alone does not validate consumer behavior or editor integration.

For vendored consumers, `go mod vendor` needs the grammar's `tree_sitter` headers and go-tree-sitter's `src/include` directory restored alongside the vendored sources; [the vendor probe](tests/vendor_e2e.py) demonstrates this requirement.

## Workspace behavior

Keep current-file work independent of background discovery. Accept open buffers first, process related directories in small batches, and queue the remaining workspace with bounded concurrency. Index facts are detached summaries, so closed-file text and parser trees can be released.

`index` preserves multiple definition candidates and never decides namespace, resource identity, or whether a rename is safe. `workspace` does not install watchers or start workers. A discovery pass being complete and an index having no pending files are separate facts; read failures, running work, and changed configurations still need to be accounted for by the consumer.

The public consumers exercise actual Application/AppProject and Workflow/WorkflowTemplate YAML fixtures, input races, incremental invalidation, directory continuation, capacity limits, and buildable fault-injection variants. These verify the shared mechanisms, not full Argo language-server behavior or Zed startup latency.

## License and attribution

Original code and documentation are licensed under [MIT](LICENSE). Retain the copyright and license notices when redistributing copies or substantial portions.

| Dependency | Pinned version | License |
|---|---|---|
| [go-tree-sitter](https://github.com/tree-sitter/go-tree-sitter) | v0.25.0 | MIT |
| [tree-sitter-yaml](https://github.com/tree-sitter-grammars/tree-sitter-yaml) | a1c4812a73ec; vendored with a bounded scanner serialization patch | MIT |
| [go-pointer](https://github.com/mattn/go-pointer) | v0.0.1; indirect dependency | MIT |

See [grammar provenance and patch details](yamlview/internal/parser/grammar/README.md). Tree-sitter's Unicode code retains its [separate copyright and license](https://github.com/tree-sitter/go-tree-sitter/blob/v0.25.0/src/unicode/LICENSE). Third-party licenses are not replaced by this project's MIT license. Source and binary distributions must include the licenses and applicable notices of their actual components. Dependency content is locked by `go.mod` and `go.sum`.
