# 公共消费者验证指南

这里说明当前可运行的验证入口。模块职责见[架构](../docs/architecture.md)，问题与对应防线见[经验教训](../docs/lessons-learned.md)。以下命令从仓库根目录运行；行为期望保存在 `consumer/*.go`，不从被测实现动态推导。

## 环境与原生入口

需要 Go 1.27.3、Python 3、本机 C 编译器和可写 Go cache。设置 `GOWORK=off`、`CGO_ENABLED=1`；`GO` 可选择 Go 可执行文件，显式 `GOROOT` 必须与它匹配，`CC` 选择编译器。

```sh
python3 tests/native.py --expected-platform linux/amd64
```

平台参数必须匹配实际主机与目标：`linux/amd64`、`linux/arm64`、`darwin/amd64`、`darwin/arm64` 或 `windows/amd64`。runner 拒绝交叉编译；`--cc-version` 校验编译器版本子串，`--output` 选择输出目录，默认 `artifacts/native`。

[`native.py`](native.py) 创建内容寻址的本地 module proxy，在无 replace 的独立模块中构建消费者，必须完整运行以下十二个非空套件且全部通过：`source`、`edit`、`yaml`、`yamlvalue`、`schemaquery`、`audit`、`budget`、`retention`、`resume`、`resources`、`index`、`workspace`。未知阶段、缺少工具、编译失败、超时或断言失败均不能产生成功报告。

可加 `--source-commit <完整提交>`，要求所有打包运行文件逐字节匹配该提交的原始 Git 对象；无 Git 元数据、提交不存在或字节不同均失败。默认入口验证当前工作区，也支持无 Git 元数据的源码包。[原生 CI](../.github/workflows/native.yml)在五个平台使用同一入口并传入实际提交；runner image 和工具链写入报告。

## 按模块验证

| 阶段与主要输入 | 验证的行为 |
|---|---|
| `source`、`edit`；[`main.go`](consumer/main.go) | UTF-8/UTF-16/CRLF 坐标、顺序输入变化、基础 revision、原文与重叠校验 |
| `yaml`、`audit`；[`yaml.go`](consumer/yaml.go)、[`audit.go`](consumer/audit.go) | YAML 结构、损坏恢复、重复键、alias/merge 的保留与位置 |
| `yamlvalue`；[`yamlvalue.go`](consumer/yamlvalue.go)、[`yamlvalue_comments.go`](consumer/yamlvalue_comments.go) | alias/merge 值、来源独立性、共享 anchor、显式 tag 策略、展开预算、部分根字段提示；尾注释的语义与原文范围、Unicode/控制字符、投影分配 |
| `schemaquery`；[`schemaquery.go`](consumer/schemaquery.go) | 非 Argo schema、引用/组合分支、有限兄弟约束、候选验证、oneOf、预算和显式未支持状态 |
| `budget`、`resume`；[`budget.go`](consumer/budget.go)、[`resume.go`](consumer/resume.go) | 输入/文档/深度边界、scanner 序列化、分片恢复及取消 |
| `retention`、`resources`；[`retention.go`](consumer/retention.go)、[`main.go`](consumer/main.go) | 成功/错误/取消后的 context 持有、原生对象释放和 RSS 增长 |
| `index`；[`index.go`](consumer/index.go)、[`relations.go`](consumer/relations.go)、[`reuse.go`](consumer/reuse.go) | 票据时效、缺失目标、候选歧义、位置与语义失效、scope、容量、摘要所有权、查询与复用分配 |
| `workspace`；[`workspace.go`](consumer/workspace.go)、[`coordination.go`](consumer/coordination.go)、[`workqueue.go`](consumer/workqueue.go) | 打开缓冲区、读取消、通知合并、公平性、执行预约、扫描优先与续扫、完成度和对账 |

选择需要的阶段运行，例如：

```sh
python3 tests/e2e.py index
python3 tests/e2e.py workspace
python3 tests/e2e.py retention
GOFLAGS=-race python3 tests/e2e.py index-race
GOFLAGS=-race python3 tests/e2e.py workspace-race
```

`e2e.py` 使用临时独立 Go 模块和本地 replace；仓库交付的 `go.mod` 不包含本机路径。无 replace 的消费验证以 `native.py` 和下述 delivery 入口为准。

查询分配测试用大量符号与小结果上限检测 collect-all 实现；存储测试用大输入的短字符串切片检测全文持有；扫描用例先推进大后台目录，再提升当前目录，检查有界批次内的响应以及最终不漏不重。Application/AppProject 和 Workflow/WorkflowTemplate fixture 的 Argo 提取逻辑位于测试消费者，不进入公共库。

尾注释投影测试先构建并解析一个恰好 5 MiB、语义值很小的快照，再测三次 `yamlvalue.Project` 的 `TotalAlloc` 增量，总量须小于 3 MiB；不把 snapshot/CST 分配混入投影测量，也不依赖机器速度。把 Project 输入恢复为完整原范围的消融仍须编译并返回正确值，但必须因重复扫描大注释触发指定分配失败。literal/folded 标量、显式缩进、CRLF、多文档、Unicode 换行和非法字符另有固定行为期望，防止性能优化吞掉内容或错误。

资源阈值属于可复现实验条件，不是新增 API 配额或“没有任何泄漏”的证明。RSS 在 Linux 使用 `/proc/self/status` 的 VmRSS，macOS 使用 `ps`，Windows 使用 `GetProcessMemoryInfo` 的 WorkingSetSize；测量失败必须使验证失败，不能以 Go heap 替代原生 RSS。

## 可编译的故障注入

```sh
python3 tests/ablate.py
python3 tests/ablate.py index-ticket index-reuse index-retention
python3 tests/ablate.py index-prefix schemaquery-validation yamlvalue-origin
python3 tests/ablate.py yamlvalue-comment-input yamlvalue-comment-characters yamlvalue-comment-tabs
python3 tests/ablate.py workspace-workqueue-fairness workspace-workqueue-followup workspace-membership-complete
python3 tests/ablate.py resources retention
```

[`ablate.py`](ablate.py) 在隔离的运行源码副本中只去掉一项相关防线，再编译相同独立消费者。必须成功编译并使指定行为断言失败；崩溃、编译错误、超时或无关断言失败不算检出。脚本支持的 variant 名称以其 `variants` 列表为准。

这种验证证明测试能识别指定错误，不自动证明某个更简单的完整设计不可行。已有完整设计替代实验留下的规则见[经验教训](../docs/lessons-learned.md#3-代码更短仍可能破坏资源上界或当前目录响应)。

## Checkout、源码快照与 vendor

```sh
python3 tests/checkout_e2e.py
python3 tests/snapshot_e2e.py
python3 tests/vendor_e2e.py
```

[`checkout_e2e.py`](checkout_e2e.py) 用真实临时 checkout 比较 `core.autocrlf` 两种配置，期望字节来自 Git blob；另外检查默认入口接受工作区修改、显式提交验证拒绝无效来源。`--without-attributes` 只在临时 fixture 移除换行策略，作为必须失败的反例，不修改调用者 Git 配置。

[`snapshot_e2e.py`](snapshot_e2e.py) 比较完整 module ZIP，覆盖 ZIP 默认元数据、路径排序和有效 deflate 差异。打包器固定 Unix creator、相对 POSIX 路径顺序及不压缩存储，拒绝覆盖同版本不同字节。这个环境实验不替代原生 Windows 验证；包装字节改变后使用新的 proxy 目录，并显式更新消费者 ZIP 锁。

[`vendor_e2e.py`](vendor_e2e.py) 验证标准 Go vendor 与原生 include/头文件的交付边界。消费者应补齐 grammar 的 `tree_sitter` 头文件以及 go-tree-sitter 的 `include`/`src`，同时保留来源及许可资料。

## 独立交付与失败路径

```sh
python3 tools/snapshot.py --output ./artifacts/module-proxy
python3 tests/delivery.py ./artifacts/module-proxy/manifest.json ./artifacts/module-proxy
python3 tests/native_delivery_e2e.py
python3 tests/delivery_failures_e2e.py
```

[`snapshot.py`](../tools/snapshot.py) 打包公共运行源码、合同、原生 includes/headers、来源和许可，写入 `manifest.json`，不发布 module。运行 [`delivery.py`](delivery.py) 前，将 `GOMODCACHE` 设为可写绝对路径，并用正常的 `go mod download` 填充依赖缓存。

delivery 从 `go.sum` 锁定的真实下载缓存取得依赖及 `.info`，使用全新 module cache、仅文件代理、`GOSUMDB=off`、`GOWORK=off` 和无 replace 的独立消费者。它运行 `source`、`edit`、`yaml`、`yamlvalue`、`schemaquery`、`audit`、`resources`、`index`、`workspace` 九个交付套件，不读取相邻仓库；缓存缺失应正常下载，不能伪造 metadata。

`native_delivery_e2e.py` 在 Linux 检查隔离源码、空套件、RSS 中间采样失败、平台不符及缺失编译器；`delivery_failures_e2e.py` 检查缺失工具、损坏子报告和超时不能复用旧成功报告，也验证移除 RSS 全样本检查后能检出该反例。

## 输出与结论范围

输出位于忽略的 `artifacts/`：普通套件为 `<stage>/report.json`，反例为 `<stage>-ablation/report.json`；独立入口默认使用 `native`、`delivery`、`checkout`、`snapshot`、`vendor-probe`、`native-delivery`、`delivery-failures` 子目录。`consumer/` 和各验证脚本是仓库中的持久输入，运行报告和二进制不作为文档提交。

报告记录命令、环境、工具版本、源码/输入/二进制 hash、期望与断言、stdout/stderr 和退出码；原生报告还包含 module ZIP、manifest、消费者 go.mod/go.sum 与适用许可。零断言、缺报告和空成功入口不通过。

本仓验证公共机制及其源码交付。业务资源识别、跨文件跳转、LSP 发布时序、真实编辑器启动时长，由 Argo CD 和 Workflows 仓库各自验证。
