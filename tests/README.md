# 公共消费者验证指南

这里说明当前可运行的验证入口。模块职责见[架构](../docs/architecture.md)，问题与对应防线见[经验教训](../docs/lessons-learned.md)。以下命令从仓库根目录运行；行为期望保存在 `consumer/*.go`，不从被测实现动态推导。

## 环境与原生入口

需要 Go 1.27.3、Python 3、本机 C 编译器和可写 Go cache。设置 `GOWORK=off`、`CGO_ENABLED=1`；`GO` 可选择 Go 可执行文件，显式 `GOROOT` 必须与它匹配，`CC` 选择编译器。

GitHub CI 在 PR 和推送 `main` 时使用原生五平台矩阵、Go 1.27.3、Python 3.14.x；`setup-go` 按平台、Go 版本、`go.sum`、实际 C 编译器版本、runner image 和原生 `.inc`/`.h` 内容缓存 Go 模块及编译产物。编译器指纹位于 checkout 内的 `artifacts/cache/compiler.txt`。`go test -race -count=1 ./index` 禁用成功测试结果缓存；所有公共消费者和消融仍实际执行。**测试入口、失败条件、证据留存不随编译缓存命中变化**。主分支上只有矩阵、Linux 回归及源码交付验证全部成功，且仍是当前 `main` 提交时，才会尝试发布 `VERSION` 的 alpha tag。发布使用[当前发行说明](../RELEASE_NOTES.md)，不会重写已存在的 tag/release。

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
| `index`；[`index.go`](consumer/index.go)、[`index_context.go`](consumer/index_context.go)、[`relations.go`](consumer/relations.go)、[`reuse.go`](consumer/reuse.go) | 票据时效、缺失目标、候选歧义、位置与语义失效、scope、容量、摘要所有权、查询与复用分配；符号查询文件/符号访问预算、取消、scope 变化、空 scope 和成员迁移 |
| `workspace`；[`workspace.go`](consumer/workspace.go)、[`coordination.go`](consumer/coordination.go)、[`workqueue.go`](consumer/workqueue.go) | 打开缓冲区、读取消、通知合并、公平性、执行预约、扫描优先与续扫、完成度和对账 |

选择需要的阶段运行，例如：

```sh
python3 tests/e2e.py index
python3 tests/e2e.py workspace
python3 tests/e2e.py retention
GOFLAGS=-race python3 tests/e2e.py index-race
GOFLAGS=-race python3 tests/e2e.py workspace-race
go test -race -count=1 ./index
```

`e2e.py` 使用临时独立 Go 模块和本地 replace；`-trimpath` 去掉编译输入中的临时目录路径，使内容不变的编译任务能复用构建缓存。Go 的 CGO 缓存不会自动跟踪任意 `#include` 的 `.inc` 内容，所以 runner 对 `.inc`/`.h`/`.c` 字节计算 `nativeInputSHA256`，读取所选 Go 工具链的有效 `CGO_CFLAGS` 后追加摘要宏。这样保留默认优化级别，并让仅修改 native include 的消融重新编译。每次仍实际执行消费者；报告不作为测试结果缓存。仓库交付的 `go.mod` 不包含本机路径。`native.py`/`delivery.py` 使用内容摘要确定的不可变 module version，原生输入变化会产生新的 module 身份。

查询分配测试用大量符号与小结果上限检测 collect-all 实现；存储测试用大输入的短字符串切片检测全文持有；扫描用例先推进大后台目录，再提升当前目录，检查有界批次内的响应以及最终不漏不重。Application/AppProject 和 Workflow/WorkflowTemplate fixture 的 Argo 提取逻辑位于测试消费者，不进入公共库。

`index_context.go` 的公开消费者使用真实 Index 与可取消 context，验证零匹配、pending/零符号文件也消耗预算，预算耗尽不交付部分结果；查询中同 scope 修改及 remove/recreate 拒绝过时结果，不相关 scope 的修改不使查询失效。跨 scope 迁移时，失败 Commit 不得发布旧位置，原 ticket 仍能重试。50,000 个零符号文件配合小预算的分配检查，防止先复制全部 URI 再分页。`index/query_context_test.go` 只补充无法从外部精确控制的边界：持有 provider 写锁时取消仍能返回、单批不得扫描超过 128 次访问、generation 耗尽后拒绝变更而非回绕。它不替代独立消费者或插件真实 LSP 验收。

尾注释投影测试先构建并解析一个恰好 5 MiB、语义值很小的快照，再测三次 `yamlvalue.Project` 的 `TotalAlloc` 增量，总量须小于 3 MiB；不把 snapshot/CST 分配混入投影测量，也不依赖机器速度。把 Project 输入恢复为完整原范围的消融仍须编译并返回正确值，但必须因重复扫描大注释触发指定分配失败。literal/folded 标量、显式缩进、CRLF、多文档、Unicode 换行和非法字符另有固定行为期望，防止性能优化吞掉内容或错误。

资源阈值属于可复现实验条件，不是新增 API 配额或“没有任何泄漏”的证明。RSS 在 Linux 使用 `/proc/self/status` 的 VmRSS，macOS 使用 `ps`，Windows 使用 `GetProcessMemoryInfo` 的 WorkingSetSize；测量失败必须使验证失败，不能以 Go heap 替代原生 RSS。

## 可编译的故障注入

```sh
python3 tests/ablate.py
python3 tests/ablate.py index-ticket index-reuse index-retention
python3 tests/ablate.py index-prefix schemaquery-validation yamlvalue-origin
python3 tests/ablate.py index-query-budget index-query-files index-query-scope
python3 tests/ablate.py yamlvalue-comment-input yamlvalue-comment-characters yamlvalue-comment-tabs
python3 tests/ablate.py workspace-workqueue-fairness workspace-workqueue-followup workspace-membership-complete
python3 tests/ablate.py resources retention
```

[`ablate.py`](ablate.py) 先为每个受影响的套件运行一次未修改基线，要求指定断言为真，再在隔离源码副本中只改变一项防线。每个变体必须重新完成构建、正常执行消费者并使指定断言变为假；崩溃、编译错误、超时、空输出、无关断言失败或不存在的旧报告均不算检出。每次使用独立输出目录，报告绑定实际目标目录。未知或重复 variant 直接失败；默认运行 `variants` 注册表中的全部 33 项，CI 不另行维护容易遗漏的部分名称清单。

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

delivery 先在临时种子模块中用仓库的 `go.mod`/`go.sum` 下载完整依赖图，并记录实际补全的 sums；这样能取得仅出现在完整图中的 `.info`，不会修改仓库锁文件或被测 module ZIP。随后使用全新 module cache、仅文件代理、`GOSUMDB=off`、`GOWORK=off` 和无 replace 的独立消费者。它运行 `source`、`edit`、`yaml`、`yamlvalue`、`schemaquery`、`audit`、`resources`、`index`、`workspace` 九个交付套件，不读取相邻仓库；缓存缺失应正常下载，不能伪造 metadata。

`native_delivery_e2e.py` 在 Linux 检查隔离源码、空套件、RSS 中间采样失败、平台不符及缺失编译器；`delivery_failures_e2e.py` 检查缺失工具、损坏子报告和超时不能复用旧成功报告，也验证移除 RSS 全样本检查后能检出该反例。

## 有效性审计与正交覆盖

验收分成互补的维度；不把每个交付故障再乘以全部平台，也不把功能正确、取消、资源上界合并成一个“命令返回成功”。已有业务断言及资源阈值全部保留。

| 测试家族与持久入口 | 独立判定依据 | CI 执行位置与边界 |
|---|---|---|
| `source`、`edit`：`consumer/main.go` | 固定坐标与编辑后原文字节；非法 revision、重叠、原文不符各自隔离 | 五个平台正常消费；Linux `source`/`edit` 消融 |
| YAML：`yaml.go`、`audit.go`、`budget.go`、`resume.go` | 固定文档/节点/位置；256/257 深度、文档/字节上限；取消与恢复 | 五个平台正常消费；Linux bytes/depth/scanner-budget 消融 |
| 投影/查询：`yamlvalue*.go`、`schemaquery.go` | 固定 JSON 值、来源范围、独立 schema 候选；预算失败不冒充空成功 | 五个平台正常消费；Linux origin、validation、注释内容/分配三种消融 |
| 保留与原生资源：`retention.go`、`audit.go` | 不同成功/错误/取消路径的持有量；实际 RSS 全部采样有效 | 五个平台正常消费；Linux `retention`/`resources` 消融；RSS 读失败与缺失 guard 分别验证 |
| 索引：`index*.go`、`reuse.go`、`relations.go` | 字面期望位置/引用/顺序；原子 ticket、scope、复用与内存边界 | 五个平台正常消费；Linux race 与所有 index 消融 |
| Workspace：`workspace.go`、`coordination.go`、`workqueue.go` | 真实文件、打开缓冲区、通知/执行时序、8:4:1 公平性、完整扫描后的对账 | 五个平台正常消费；Linux race 与所有 workspace 消融；Windows 无权限创建 symlink 时记录不可用，其他平台须执行 |
| 索引锁边界：`index/query_context_test.go` | 写锁仍被持有时取消返回；合同规定的最多 128 次批次访问；generation 不回绕 | Linux `go test -race -count=1 ./index`，启动边界和返回边界都有超时 |
| 不可变交付：`native.py`、`delivery.py` | 五平台原生身份、完整套件、严格 boolean 输出；独立 module / 无 replace；离线空缓存实际消费 | 五平台 `consumer` 与 Linux `delivery_regression`；delivery 的九个套件缺一即失败 |
| 字节与包装：`checkout_e2e.py`、`snapshot_e2e.py`、`vendor_e2e.py` | Git blob 为 checkout 真值；跨 ZIP 元数据/顺序/压缩环境不变；恢复 headers 后实际 vendor 构建 | Linux `delivery_regression`；ZIP 确定性不替代 native 的源码清单/消费验证 |
| 失败路径：`native_delivery_e2e.py`、`delivery_failures_e2e.py` | 每个负例必须失败于指定阶段；先保留成功 sentinel，再执行失败重跑 | Linux `delivery_regression`；平台不符不能由缺 Go、缺编译器不能由任意别的错误代替 |
| Runner 合同：`runner_effectiveness_e2e.py`、`assertions.py` | 受控真实子进程，逐次只改变一个条件，验证判定器自身不会假通过 | Linux `delivery_regression`；协议夹具不计入公共 API 行为证明 |
| 观察探针：`probes/index-query`、`probes/index-scope` | 实际执行量、固定排序/计数；零查询、非法参数须拒绝 | Linux 验证探针可构建及参数边界；时间/分配数仅作观察，不能替代消费者中的资源断言 |

```sh
python3 tests/runner_effectiveness_e2e.py
```

Runner 的表驱动用例覆盖：合法 boolean 对照、空/非对象/字符串/数值输出、重复 JSON 键、输出和退出码矛盾；基线失败、缺失/损坏报告、错误目标目录、构建/工具失败及无关断言失败；未知/重复变体；缺 Go 时不得复用上轮成功检出记录。另执行实际消费者的 `check` 累积逻辑，确认同名 fixture 后续成功不能覆盖先前失败。探针先构建一次再测试参数边界，避免把编译失败误算成“参数被拒绝”。

`source`/`edit` 的失效保护、CGO scanner-budget、原生资源释放及 context 持有消融，与原有 index/workspace 消融在同一个默认注册表中执行。一个基线可以为同套件的多个独立变体复用；每个变体仍用独立源码和消费者进程，不减少被测行为。`index-race` 与 `workspace-race` 分别检查各自并发机制，正常五平台套件继续保留完整行为验收。

## 输出与结论范围

输出位于忽略的 `artifacts/`：普通套件为 `<stage>/report.json`，消融为 `ablations/<runID>/<variant>/report.json`，同轮基线为 `baseline-<suite>`；`ablations/report.json` 保存最新汇总。独立入口默认使用 `native`、`delivery`、`checkout`、`snapshot`、`vendor-probe`、`native-delivery`、`delivery-failures`、`runner-effectiveness` 子目录。`consumer/` 和各验证脚本是仓库中的持久输入，运行报告和二进制不作为文档提交。

报告记录命令、环境、工具版本、源码/输入/二进制 hash、期望与断言、stdout/stderr 和退出码；`native.py` 与 `e2e.py` 还记录各命令及整轮的 `elapsedSeconds`。原生来源核验使用三次 Git 命令批量读取提交清单和原始 blob hash，保留逐文件字节校验，并拒绝被删的运行输入。原生报告还包含 module ZIP、manifest、消费者 go.mod/go.sum 与适用许可。断言必须是非空对象、非空名称及真实 JSON boolean；重复键、字符串 `"false"`、零断言、缺报告和空成功入口不通过。

普通消费者先在本轮独立目录编译，按 `consumer-<SHA256>` 保留一份可执行文件，报告的 `retainedBinary` 指向该文件；编译失败不能复用历史二进制。Linux 回归证据先打包为 `toolkit-index-regression.tar.gz`，上传时使用零级压缩，避免再对已压缩包进行压缩。源码/测试输入 ZIP、指定失败断言、历史报告和二进制摘要仍保留；原生五平台的小型证据目录保持直接上传。使用报告和 Actions step 时长分别比较构建、测试与归档，冷缓存和命中缓存的运行分开统计。

本仓验证公共机制及其源码交付。业务资源识别、跨文件跳转、LSP 发布时序、真实编辑器启动时长，由 Argo CD 和 Workflows 仓库各自验证。
