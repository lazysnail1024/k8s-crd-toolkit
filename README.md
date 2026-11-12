# k8s-crd-toolkit

面向 Go 语言服务器的公共源码依赖，提供不可变文本、保留位置的 YAML 结构、有来源的有效值、编译后 Schema 查询、校验式编辑、增量摘要索引和有界文件发现。资源识别、Argo 语义、LSP 传输、worker、配置及编辑器集成由消费者实现。

## 接入

将 `TOOLKIT_COMMIT` 设置为经过审阅的完整 Git SHA：

```sh
go get github.com/lazysnail1024/k8s-crd-toolkit@"$TOOLKIT_COMMIT"
```

提交生成的 Go pseudo-version 和 `go.sum`；源码引用不要求 tag 或 release。当前两个插件采用的提交和合同见[接入边界](docs/architecture.md#两个插件的实际引用与隔离)。公共包路径均位于 `github.com/lazysnail1024/k8s-crd-toolkit` 下：

| 包 | 职责 |
|---|---|
| `source` | 不可变快照、UTF-8 字节 span、UTF-16 坐标和顺序修改 |
| `yamlview` | 容错结构解析、文档边界、重复键、原始位置和语法问题 |
| `yamlvalue` | 严格 YAML 有效值、alias/merge 有界投影、exact/derived/ambiguous 来源与共享 anchor |
| `schemaquery` | 查询调用方已有的 jsonschema/v6 编译图，提供字段、类型、分支及有限候选 |
| `edit` | 基础 revision、原文和区间重叠校验，原子生成新快照 |
| `index` | 平铺定义/引用、缺失目标的反向依赖、过时结果拒绝、摘要复用和可取消、按访问量计费的符号查询 |
| `workspace` | 权威打开缓冲区、读票据、工作队列、扫描轮次对账和可续扫的目录发现 |

从当前打开文件开始处理，并让关联目录优先于后台发现。索引事实是摘要；目录扫描完成与业务分析完成需要分别判断。公共库不访问集群或写入用户文件。

## 文档

- [文档入口](docs/README.md)
- [当前模块实现、状态所有权和两个插件的引用边界](docs/architecture.md)
- [已发生问题与实验反例的经验教训](docs/lessons-learned.md)
- [公共 API 合同](CONTRACT.md)：消费者按字节锁定的有效接口合同。
- [验证指南](tests/README.md)：公共消费者、原生平台、源码快照和反例检查。

## 构建与验证

使用 Go 1.27.3。`yamlview` 及全包构建需要本机 C 编译器与 `CGO_ENABLED=1`；`source`、`edit`、`index`、`workspace`、`schemaquery` 可独立在无 CGO 环境使用。Python 3 仅用于本仓库的验证及源码快照工具。

```sh
GOWORK=off CGO_ENABLED=1 go build ./...
python3 tests/native.py --expected-platform linux/amd64
```

将平台参数换为实际主机的 `linux/amd64`、`linux/arm64`、`darwin/amd64`、`darwin/arm64` 或 `windows/amd64`。原生消费者拒绝以交叉编译作为验收；库级验证不替代插件 LSP 与 Zed 集成验证。

vendored 消费者需要在 `go mod vendor` 后补齐 grammar 的 `tree_sitter` 头文件，以及 go-tree-sitter 的 `include`/`src` 目录；[vendor 验证入口](tests/vendor_e2e.py)覆盖这个交付要求。

本地独立验证可以生成未发布的内容快照：

```sh
python3 tools/snapshot.py --output ./artifacts/module-proxy
```

manifest 记录源码、合同和 ZIP 摘要。正式消费者依赖仍以已审阅提交、Go sums 及消费者锁文件为准；工具不发布版本。

## 许可证与依赖来源

本项目代码和文档使用 [MIT](LICENSE)。重新分发时保留相应版权和许可声明。

| 依赖 | 锁定版本 | 许可证 |
|---|---|---|
| [go-tree-sitter](https://github.com/tree-sitter/go-tree-sitter) | v0.25.0 | MIT |
| [tree-sitter-yaml](https://github.com/tree-sitter-grammars/tree-sitter-yaml) | a1c4812a73ec，包含有界 scanner 序列化补丁 | MIT |
| [go-pointer](https://github.com/mattn/go-pointer) | v0.0.1，间接依赖 | MIT |
| [jsonschema](https://github.com/santhosh-tekuri/jsonschema) | v6.0.3 | Apache-2.0 |
| [yaml.v3](https://gopkg.in/yaml.v3) | v3.0.1 | MIT 与 Apache-2.0，按文件适用 |
| [sigs.k8s.io/yaml](https://github.com/kubernetes-sigs/yaml) | v1.6.0 | MIT |
| [go.yaml.in/yaml/v2](https://github.com/yaml/go-yaml) | v2.4.2，间接依赖 | Apache-2.0 |
| [golang.org/x/text](https://pkg.go.dev/golang.org/x/text) | v0.14.0，间接依赖 | BSD-3-Clause |

语法来源、补丁与原始许可见 [grammar 说明](yamlview/internal/parser/grammar/README.md)。Tree-sitter 的 Unicode 实现另有[独立版权和许可](https://github.com/tree-sitter/go-tree-sitter/blob/v0.25.0/src/unicode/LICENSE)。本仓 MIT 许可不替代第三方许可；源码和二进制交付需包含实际组件的适用声明。Go 依赖内容由 `go.mod`、`go.sum` 锁定。
