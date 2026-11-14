# k8s-crd-toolkit

A Go library for Kubernetes CRD source analysis. It provides immutable text snapshots, YAML structure and effective values, compiled JSON Schema queries, checked text edits, incremental symbol indexes and bounded workspace discovery. Argo domain rules, LSP servers, editor adapters and cluster operations remain in consumers.

Published module: **[v0.0.1-alpha.1](https://github.com/lazysnail1024/k8s-crd-toolkit/releases/tag/v0.0.1-alpha.1)**. This is a Go source module, with no standalone binary to install. The API may change during alpha development. Published tags retain their original implementation; later changes on main require an explicit dependency upgrade.

## Use

Requires **Go 1.27.3**. YAML parsing also requires a native C compiler and `CGO_ENABLED=1`.

```sh
go get github.com/lazysnail1024/k8s-crd-toolkit@v0.0.1-alpha.1
```

Parse YAML while preserving original source locations:

```go
package main

import (
    "context"
    "fmt"

    "github.com/lazysnail1024/k8s-crd-toolkit/source"
    "github.com/lazysnail1024/k8s-crd-toolkit/yamlview"
)

func main() {
    snapshot, err := source.New("file:///example.yaml", 1, "apiVersion: v1\nkind: ConfigMap\n")
    if err != nil {
        panic(err)
    }
    view, err := yamlview.Parse(context.Background(), snapshot)
    if err != nil {
        panic(err)
    }
    fmt.Printf("documents=%d issues=%d\n", len(view.Documents()), len(view.Issues()))
}
```

[CONTRACT.md](CONTRACT.md) defines public signatures, cancellation and budgets. A parsed YAML document does not establish Kubernetes validity. Consumers decide resource ownership, version compatibility, reference binding and edit authority.

## Public packages

| Package | Responsibility |
| --- | --- |
| `source` | Immutable text, UTF-8 byte spans, UTF-16 coordinates and sequential edits |
| `yamlview` | Fault-tolerant YAML structure, document boundaries, source locations and syntax issues |
| `yamlvalue` | Strict effective values, bounded alias/merge projection and provenance |
| `schemaquery` | Field, type and finite-value queries over caller-compiled schema graphs |
| `edit` | Revision and original-text checks, non-overlapping replacements and atomic application |
| `index` | Definition/reference facts, invalidation tickets and cancellable bounded symbol queries |
| `workspace` | Open buffers, read tickets, work bookkeeping, reconciliation and batched discovery |

Current source uses contract `toolkit-design-6`. Its native parser lazily reuses at most two private OS-thread workers to limit allocator thread ownership. Go tree copying runs on the caller. Cancellation before admission does not start native work; admitted calls finish per-call cleanup before returning. Idle workers do not retain caller text or contexts. Applications still own their input budgets and analysis scheduling. No service, watcher, shared editor cache or Kubernetes client is started.

## Argo consumers

[zed-argocd](https://github.com/lazysnail1024/zed-argocd) and [zed-argo-workflows](https://github.com/lazysnail1024/zed-argo-workflows) independently own their LSP, domain rules, version catalogues, diagnostics, edits and Zed adapters. Each pins an exact module implementation, contract snapshot, checksums and native dependency inputs. Publishing a toolkit tag does not upgrade either consumer or an already published plugin.

Use each consumer's `contracts/toolkit.lock.json` for its actual dependency identity. Reproducible consumers should pin a reviewed version or full commit and retain `go.sum` and the required native sources. See [architecture](docs/architecture.md) for module ownership and integration boundaries.

## Validation and publication

```sh
GOWORK=off CGO_ENABLED=1 go build ./...
python3 tests/native.py --expected-platform linux/amd64
```

Choose the actual host: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64` or `windows/amd64`. Cross-compilation does not replace native execution. Python 3 is used for repository verification and source snapshots, not at consumer runtime.

PR and main CI execute independent Go module consumers on all five platforms, plus Linux race and ablation checks. Publication requires successful checks for the same main commit. Existing tags and releases are never overwritten. See the [verification guide](tests/README.md) and [release notes](RELEASE_NOTES.md).

## Documentation and licenses

- [Architecture and consumer boundaries](docs/architecture.md)
- [Public API contract](CONTRACT.md)
- [Verification and distribution](tests/README.md)
- [Lessons and regression evidence](docs/lessons-learned.md)
- [Documentation index](docs/README.md)

First-party code and documentation use [MIT](LICENSE). Third-party code retains its own licenses. [go.mod](go.mod) and [go.sum](go.sum) pin tree-sitter Go v0.25.0, jsonschema/v6 v6.0.3, yaml.v3 v3.0.1 and sigs YAML v1.6.0. The YAML grammar uses tree-sitter-yaml `a1c4812a73ec` with the local bounded-scanner patch; see its [source and license notices](yamlview/internal/parser/grammar/README.md).

Contributor build, CI, fuzz and downstream acceptance commands are in [DEVELOPMENT.md](DEVELOPMENT.md).
