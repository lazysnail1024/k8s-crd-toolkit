# k8s-crd-toolkit

供 Go 语言服务器复用的 **Kubernetes CRD 源码分析基础库**。它处理文本快照、YAML 结构与有效值、编译后 JSON Schema 查询、安全文本替换、增量符号索引和有界工作区发现；不包含 Argo 业务规则、LSP 服务端、Zed 适配器或集群操作。

当前预发布版本：**[v0.0.1-alpha.1](https://github.com/lazysnail1024/k8s-crd-toolkit/releases/tag/v0.0.1-alpha.1)**。这是 Go Module 源码版本，没有需要安装的独立二进制。alpha 阶段 API 可能调整。

## 使用

需要 **Go 1.27.3**。使用 YAML 解析的消费者还需要本机 C 编译器和 `CGO_ENABLED=1`。

```sh
go get github.com/lazysnail1024/k8s-crd-toolkit@v0.0.1-alpha.1
```

例如，解析一份 YAML 并获取保留原始位置的文档视图：

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

库的完整公开签名、取消与配额语义以 [CONTRACT.md](CONTRACT.md) 为准。YAML 可被解析不等于 Kubernetes 资源有效；资源识别、版本兼容、引用关系和编辑授权始终由消费者决定。

## 已实现的公共包

| 包 | 提供的能力 |
|---|---|
| `source` | 不可变文本、UTF-8 字节范围、UTF-16 坐标和顺序编辑 |
| `yamlview` | 带位置的容错 YAML 结构、多文档边界和语法问题 |
| `yamlvalue` | 严格有效值、alias/merge 的有界投影及来源归属 |
| `schemaquery` | 查询调用方已编译的 Schema 图，得到字段/类型/有限候选 |
| `edit` | 基于原始 revision 的非重叠、原文校验与原子替换 |
| `index` | 增量定义/引用摘要、失效票据与可取消、有工作量上界的符号查询 |
| `workspace` | 打开缓冲区、读票据、任务调度、扫描对账及分批目录发现 |

库只持有调用方创建的本地实例；不启动后台服务、不维护共享插件缓存，也不访问 Kubernetes API。

## 与两个 Argo 插件的关系

- [zed-argocd](https://github.com/lazysnail1024/zed-argocd) 和 [zed-argo-workflows](https://github.com/lazysnail1024/zed-argo-workflows) 各自拥有独立 Go LSP、领域语义、版本目录、诊断/重命名策略和 Zed 适配器。
- 两个插件目前都锁定 toolkit 源码提交 `db4adba0093ee2a709356aa1c2517e6d62644745`（`toolkit-design-5` 合同），并保留自己的来源/合同校验；**发布 alpha tag 不会自动改变它们的依赖**。
- 使用者可以按 tag 引入库；需要可重现构建的消费者应继续显式锁定审阅过的版本或完整提交、`go.sum` 和必要的原生 C 来源。

各模块的状态所有权及具体集成入口见[当前架构](docs/architecture.md)。

## 验证与发布

```sh
GOWORK=off CGO_ENABLED=1 go build ./...
python3 tests/native.py --expected-platform linux/amd64
```

替换为实际主机平台：`linux/amd64`、`linux/arm64`、`darwin/amd64`、`darwin/arm64` 或 `windows/amd64`。原生验收不接受交叉编译替代。Python 3 用于仓库验证和源码快照，不是消费者运行时依赖。

CI 在 PR 和每次推送 `main` 时执行五平台独立 Go Module 消费者与 Linux 竞争/消融回归；**只有 `main` 同一提交全部通过后**才可发布 VERSION 对应的 alpha 预发布版本。已有 tag/release 不会覆盖。流程、边界及当前版本说明见[验证指南](tests/README.md)和[发行说明](RELEASE_NOTES.md)。

## 文档与许可

- [当前模块架构与消费者边界](docs/architecture.md)
- [公开接口合同](CONTRACT.md)（消费者锁定的 `toolkit-design-5`）
- [验证与交付说明](tests/README.md)
- [已发生问题及回归经验](docs/lessons-learned.md)
- [文档导航](docs/README.md)

本仓代码及文档采用 [MIT](LICENSE)；第三方代码各保留自己的许可。直接依赖版本由 [go.mod](go.mod)、[go.sum](go.sum) 固定，包括 tree-sitter Go v0.25.0、jsonschema/v6 v6.0.3、yaml.v3 v3.0.1 和 sigs YAML v1.6.0。YAML grammar 对应 tree-sitter-yaml `a1c4812a73ec`，附带本地有界 scanner 补丁；源码、补丁与适用许可证见 [grammar 说明](yamlview/internal/parser/grammar/README.md)。
