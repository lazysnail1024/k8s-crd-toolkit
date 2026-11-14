# k8s-crd-toolkit v0.0.1-alpha.1

`VERSION` 对应的公开 alpha 标签作为 **Go Module 源码依赖**提供，不包含独立可执行文件。该已发布标签使用 `toolkit-design-5` 合同；当前主分支使用 `toolkit-design-6`。本文件描述发布标签的范围，主分支的当前实现请查阅 [README](README.md)、[公共合同](CONTRACT.md) 和[架构](docs/architecture.md)。

## 标签提供的能力

- 不可变源码快照、UTF-8/UTF-16 位置转换，以及校验式顺序/批量编辑。
- 带原文范围的容错、多文档 YAML 结构；严格有效值、alias/merge 和位置来源投影。
- 基于调用方编译好的 JSON Schema 的有界字段、类型和候选查询。
- 增量定义/引用摘要、取消感知的工作量有界查询、过期结果拒绝和依赖失效。
- 打开缓冲区优先、有界任务队列、分批目录发现与完整扫描后对账。

## 接入要求与边界

- Go 1.27.3；YAML parser 需要 CGO 和目标平台的 C 编译器。
- 原生验证平台：Linux amd64/arm64、macOS amd64/arm64、Windows amd64。
- 公共库不提供独立 LSP、Zed 扩展、Argo 特定规则、Schema 注册表或集群访问。
- 标签发布不会自动升级两个插件；它们各自的实际实现提交和合同以 `contracts/toolkit.lock.json` 为准，见[消费者边界](docs/architecture.md#两个插件的实际引用与隔离)。
- vendoring 需要保留 grammar 的 tree-sitter 头文件、go-tree-sitter 原生 include/src、第三方许可及来源。

## 发布与当前开发的关系

已发布 tag/release 保留其原始内容。`main` 的后续实现和测试改动不自动进入 `v0.0.1-alpha.1`，也不构成对该标签运行过新增测试的声明。仓库发行流程仅在完整主分支验收通过、该提交仍是远端 `main` 且新标签未被占用时创建发布；已存在的 release 不会被覆盖。

新增版本应显式更新 `VERSION` 和本文件，并按[验证指南](tests/README.md)运行当前验收流程。本项目仍为 alpha，公共接口尚不承诺稳定性。
