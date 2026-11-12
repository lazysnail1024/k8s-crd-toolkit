# k8s-crd-toolkit v0.0.1-alpha.1

首个公开 alpha 版本，作为 **Go Module 源码依赖**提供，不包含独立可执行文件。公共 API 合同为 `toolkit-design-5`。

## 当前能力

- 不可变源码快照、UTF-8/UTF-16 位置转换，以及校验式顺序/批量编辑。
- 带原文范围的容错、多文档 YAML 结构；严格有效值、alias/merge 和位置来源投影。
- 基于调用方编译好的 JSON Schema 的有界字段、类型和候选查询。
- 增量定义/引用摘要、取消感知的工作量有界查询、过期结果拒绝和依赖失效。
- 打开缓冲区优先、有界任务队列、分批目录发现与完整扫描后对账。

## 接入要求与边界

- Go 1.27.3；YAML parser 需要 CGO 和目标平台的 C 编译器。
- 支持原生验证：Linux amd64/arm64、macOS amd64/arm64、Windows amd64。
- 公共库不提供独立 LSP、Zed 扩展、Argo 特定规则、Schema 注册表或集群访问。
- 两个 Argo 插件现有发行版仍锁定源码提交 `db4adba0093ee2a709356aa1c2517e6d62644745`，不会因本次 tag 自动切换。
- 如需 vendoring，消费者须额外包含 grammar 的 tree-sitter 头文件及 go-tree-sitter 原生 include/src 来源，并保留第三方许可。

## 发行验证

GitHub Actions 为 PR 和 `main` 执行五种原生平台消费者、Go vet/race、独立回归与可编译故障注入；仅在同一 `main` 提交全部通过后创建此预发布版本。源码 tag 对应被验收的提交，不重写已存在的 tag 或 release。

详细 API、当前架构与验收命令分别见 [CONTRACT.md](CONTRACT.md)、[docs/architecture.md](docs/architecture.md) 和 [tests/README.md](tests/README.md)。本版本仍为 alpha，公共接口尚不承诺稳定性。
