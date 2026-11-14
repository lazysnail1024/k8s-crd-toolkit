# 公共消费者验证指南

这里说明当前可运行的验证入口。模块职责见[架构](../docs/architecture.md)，问题与对应防线见[经验教训](../docs/lessons-learned.md)。以下命令从仓库根目录运行；行为期望保存在 `consumer/*.go`，不从被测实现动态推导。

## 环境与原生入口

需要 Go 1.27.3、Python 3、本机 C 编译器和可写 Go cache。设置 `GOWORK=off`、`CGO_ENABLED=1`；`GO` 可选择 Go 可执行文件，显式 `GOROOT` 必须与它匹配，`CC` 选择编译器。

完整 CI 计划使用五平台原生矩阵和 `go.mod` 指定的 Go；consumer、index_regression、delivery_regression 显式安装 Python 3.14，其余 job 使用 runner 提供的 Python。上述三个 job 的 `setup-go` cache 按平台、Go 版本、`go.sum`、实际 C 编译器版本、runner image 和原生 `.inc`/`.h` 输入区分；编译器指纹位于 `artifacts/cache/compiler.txt`。`go test -race -count=1 ./index` 禁用成功测试结果缓存，公共消费者和已选中的消融每次仍实际执行。执行选择、完整 job 清单和发布条件见下文 [CI 门禁](#ci-执行与证据门禁)。

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

[`ablate.py`](ablate.py) 先为每个受影响的套件运行一次未修改基线，要求指定断言为真，再在隔离源码副本中只改变一项防线。每个变体必须重新完成构建、正常执行消费者并使指定断言变为假；崩溃、编译错误、超时、空输出、无关断言失败或不存在的旧报告均不算检出。每次使用独立输出目录，报告绑定实际目标目录。未知或重复 variant 直接失败；默认运行 `variants` 注册表中的全部变体，CI 不另行维护部分名称清单；指定的故障断言由机器验收合同独立核对。

这种验证证明测试能识别指定错误，不自动证明某个更简单的完整设计不可行。已有完整设计替代实验留下的规则见[经验教训](../docs/lessons-learned.md#代码更短仍可能破坏资源上界或当前目录响应)。

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

## 状态关系、确定性种子与 mutation fuzz

```sh
python3 tests/e2e.py properties
python3 tests/fuzz_seeds.py --platform linux/amd64
python3 tests/fuzz.py --seconds 60
```

[`consumer/properties.go`](consumer/properties.go) 用 16 个固定 seed、每个 seed 12 次文本变更，对比独立构造的最终文本和新的索引实例。关系覆盖如下；它们说明公共库已验证的范围，不把库内关系提升为未运行的 LSP 行为。

| 关系 | 当前入口与边界 |
|---|---|
| 顺序输入与最终加载一致 | `properties` 比较每次文本和最终 `Snapshot`；文本期望独立构造 |
| 复用与冷构建一致 | `properties` 比较显式 `Index.Reuse` 与新 Index 的定义和 pending 状态；`index` 同时检查复用修订、generation 与分配，不声称存在隐式 parser cache |
| 编辑、拒绝和撤销 | `source` / `edit` 验证固定编辑结果及原子拒绝；领域诊断/导航撤销由下游 LSP 验证 |
| scope 与输入生命周期隔离 | `properties` 验证 scope 切换、旧磁盘读拒绝、打开覆盖层优先、关闭后重新读盘和索引删除；文件系统 root 与 Argo 版本隔离仍是消费者策略 |
| 取消与后续恢复 | `index` 验证取消不发布部分结果；`workspace` 验证过时 worker 不覆盖新输入；完整 LSP 取消/冷加载等价由插件验收 |
| 重复通知 | `index` 的 repeated-begin 与 `workspace` 的合并、完成及后续通知用例分别覆盖对应状态 |

[`fuzz_seeds.py`](fuzz_seeds.py) 在实际主机执行 `source`、`edit`、`yamlview` 的固定 Go fuzz seeds，拒绝以交叉编译替代原生运行。五平台都运行 seed replay；这一步不声称执行过 mutation fuzz。

[`fuzz.py`](fuzz.py) 在 Linux 为 `FuzzCoordinates`、`FuzzAtomicEdit`、`FuzzParseRecovery` 分别运行实际 Go mutation campaign。预算只能为每目标 60 或 600 秒；PR、main 和普通手动执行用 60 秒，schedule 用 600 秒，均为两个 fuzz worker、30 秒最小化预算。报告绑定结构化 argv、当前 Go 版本、目标/语料 hash、原始输出与实际执行次数；只有构建或 seed/baseline 完成不能证明 campaign 已执行。失败时保留 stdout/stderr 与发现的 corpus，按 Go 输出的 `-run` 命令重放。

## 固定下游 LSP 消费者

```sh
python3 tests/downstream.py --output artifacts/downstream-check
python3 tests/downstream_receipt_e2e.py artifacts/downstream-check/report.json
```

[`downstream-pins.json`](downstream-pins.json) 定义两个不可变消费提交及 suite；它们是测试基线，并非两个插件最新 `main`。当前消费入口为：

| 消费者 | 实际 suite |
|---|---|
| Argo CD | `tests/publication_e2e.py`、`tests/alias_projection_lsp_e2e.py` |
| Workflows | `tests/e2e/wf-local-rename.py`、`tests/e2e/wf-shared-yaml-projection.py` |

[`downstream.py`](downstream.py) 克隆固定提交，只将候选 toolkit 运行文件注入隔离 checkout 的 vendor 目录，随后检查编译器实际输入路径与字节并构建真实 LSP。原消费者版本声明保留，报告明确标为 `downstream-candidate`；该过程不升级插件仓库或发布产物。候选运行文件必须逐字节匹配当前 toolkit 提交。

两份消费者结果绑定同一个 toolkit/Argo CD/Workflows 三提交组合。报告记录原始 Git commit、文件 mode/blob、唯一允许的 vendor 替换、派生清单和 patch digest；独立 verifier 重建这些信息，源码 tar 包保留跨 artifact 下载所需的文件 mode。必须使用不存在的新 `--output` 目录。`--checkout-cache DIRECTORY` 只把其中的固定本地 clone 用作 Git 对象来源，不能把已修改的工作树当输入；它们须包含 pins 所指提交。

`--control old-vendor`、`wrong-sha`、`missing-file`、`missing-consumer` 分别验证旧源码、错误身份、缺文件和缺消费者不能通过；每次使用独立输出目录。这些 controls 证明 runner 拒绝对应错误，不替代普通 suite 的产品行为断言。

## Runner 判定与故障有效性

```sh
python3 tests/runner_effectiveness_e2e.py
```

[`runner_effectiveness_e2e.py`](runner_effectiveness_e2e.py) 使用真实受控子进程，逐次只改变一个条件，覆盖合法 boolean、空/非对象/字符串/数值输出、重复 JSON key、输出与退出码矛盾、错误目标目录、旧报告、构建/工具失败和无关断言失败。它还执行消费者 `check` 的累积逻辑，保证后续成功不会覆盖同名 fixture 的先前失败。探针先构建再测参数边界，避免把编译失败算成参数拒绝；这些协议控制不计入公共 API 行为证明。

普通 ablation 需要基线通过、变体成功构建、执行返回指定失败及匹配本轮报告；崩溃、超时、缺工具或 build failure 不构成检出。原生资源、scanner budget、source/edit、index/workspace 均由同一默认注册表执行；同套件可以复用通过的基线，每个变体仍有独立源码和消费者进程。

手动运行原生工作流并设置 `audit_effectiveness=true`，可以增加以下故障有效性检查；该选项仍执行完整普通矩阵，并让失败使所在 job 失败：

| 入口 | 注入并要求检出的错误 |
|---|---|
| [`fuzz_effectiveness.py`](fuzz_effectiveness.py) | 三个可编译的 source/edit/YAML 故障，必须由真实 fuzz 引擎及随后重放命中指定行为失败 |
| [`property_effectiveness.py`](property_effectiveness.py) | source、cache/reuse、scope、delete、overlay 五种故障，必须在真实公共消费者中命中指定关系断言 |
| [`downstream_effectiveness.py`](downstream_effectiveness.py) | 四个实际 downstream driver control，以及公共 Schema 类型故障；后者须通过包内 Go 测试、构建固定 CD 消费者，再只使两个指定 `hoverType` 断言失败 |

本地也可直接运行这些脚本。property/downstream 有效性入口要求其默认 `artifacts/property-effectiveness` / `artifacts/downstream-effectiveness` 目录尚不存在；再次运行前先另存已有证据。downstream 入口接受 `--baseline` 及可选 `--checkout-cache`，并要求 baseline 属于本次候选和执行身份。当前普通 PR 不重复这些额外故障实验，但保留普通 ablation、报告负例和 fuzz/downstream 全链证据控制。`fuzz_effectiveness_e2e.py` 验证有效性 runner 的判定方式，本身不运行或宣称完成一次产品 fuzz campaign。

## CI 执行与证据门禁

[原生工作流](../.github/workflows/native.yml) 的完整计划包含以下 job。`plan` 和 `Toolkit required` 始终运行；lane/platform 收据共有十份，五份来自 native consumer，另外五份来自 Linux lanes。

| Job / lane | 执行内容 |
|---|---|
| `consumer` × 5 | 五平台原生 seed replay、十二阶段独立 module consumer；各一份收据 |
| `index_regression` | `go vet`、索引锁边界 race、index/workspace 公共 race consumer、全部注册 ablation |
| `delivery_regression` | selection/receipt/reporting controls、runner 负例、snapshot/checkout/vendor、空缓存源码消费及失败路径 |
| `properties` | 确定性状态关系；可选 property 故障有效性检查 |
| `fuzz` | 三个实际 mutation campaign；可选 fuzz 故障有效性检查 |
| `downstream` | 固定的两个真实 LSP 消费者及独立收据检查；可选下游故障有效性检查 |
| `required` / `Toolkit required` | 核对 execution plan、job 结果、同轮收据；完整计划另执行 fuzz/downstream 证据语义负例 |
| `release` | 仅在 main push 的 required 成功后运行，按 `VERSION` 和 `RELEASE_NOTES.md` 发布尚不存在的 alpha |

[`tools/ci_plan.py`](../tools/ci_plan.py) 仅允许 **PR 的全部修改均为 `README.md`、`DEVELOPMENT.md`、`RELEASE_NOTES.md` 的新增或修改** 时排除产品 lanes；门禁仍验证明确的排除计划及 skipped 结果。`docs/`、`tests/README.md`、合同、其他未知路径、删除/重命名、混合修改、损坏或不可用 diff 均选完整执行。main、schedule、workflow_dispatch 始终完整执行。排除计划不是接受缺失报告的通配规则。

[`tools/ci_receipt.py`](../tools/ci_receipt.py) 把报告绑定当前 SHA、run ID、attempt、lane 和平台，核对实际断言清单与报告语义。缺失、重复、过期或不完整证据失败；单独重跑失败 job 后若其余 producer 仍属旧 attempt，须重跑完整工作流。required 下载真实 fuzz/downstream 输出，用 [`evidence_semantics_e2e.py`](evidence_semantics_e2e.py) 同时检查 writer、plan gate 和独立 verifier 对正确及损坏证据的行为。

[`native-contract.json`](native-contract.json)、[`property-contract.json`](property-contract.json)、[`downstream-contract.json`](downstream-contract.json)、[`downstream-pins.json`](downstream-pins.json) 及 [`docs/testing-ci-baseline.json`](../docs/testing-ci-baseline.json) 是仍在执行的验收输入；最后一个文件提供 mutation 清单和故障 oracle，并非待办计划或可删除的运行报告。

原生矩阵为 Ubuntu 24.04 amd64/arm64（GCC 13.3.0）、macOS 15 Intel/arm64（Apple Clang 17.0.0）和 Windows 2025 amd64（GCC 15.2.0）；exact runner/compiler 配置以 workflow 为准。跨平台构建不替代原生执行。main 发布前再核对目标仍是最新 main；已有 release 不改写，已有 tag 但缺 release 则失败。当前发布标签与 main 的区别见[发行说明](../RELEASE_NOTES.md)。

## 性能证据的使用范围

[`tools/ci_trends.py`](../tools/ci_trends.py) 接受带 run、attempt、SHA、证据引用的显式 warm/cold 原始样本。每项还声明 platform、toolchain、selection、fixture、resources、baseline/candidate variant、passed、非空 phases 及可选 pairID；失败样本必须声明 failureType。分组保留这些维度及成功/失败分类。

至少五对 warm 或三对 cold 样本，且每对都有两个 variant、执行顺序交替，才支持中位数和离群点观察；未配对样本仍标记不足。报告保留原始样本及首个失败，不估计 p95。这些样本门槛本身不能证明优化已验收，也不能用 CI 总时长替代产品测量或改变资源限制。`python3 tests/ci_trends_e2e.py` 验证报告器自身；真实 Zed、长期观察和平台表现仍须各自的证据。

## 输出与结论范围

输出位于忽略的 `artifacts/`：普通套件为 `<stage>/report.json`，消融为 `ablations/<runID>/<variant>/report.json`，同轮基线为 `baseline-<suite>`；`ablations/report.json` 保存最新汇总。独立入口默认使用 `native`、`delivery`、`checkout`、`snapshot`、`vendor-probe`、`native-delivery`、`delivery-failures`、`runner-effectiveness` 等子目录；新增状态关系、fuzz seeds、mutation campaign 和下游验收分别使用 `properties`、`fuzz-seeds`、`fuzz`、`downstream`。`consumer/` 和各验证脚本是仓库中的持久输入，运行报告和二进制不作为文档提交。

报告记录命令、环境、工具版本、源码/输入/二进制 hash、期望与断言、stdout/stderr 和退出码；`native.py` 与 `e2e.py` 还记录各命令及整轮的 `elapsedSeconds`。原生来源核验使用三次 Git 命令批量读取提交清单和原始 blob hash，保留逐文件字节校验，并拒绝被删的运行输入。原生报告还包含 module ZIP、manifest、消费者 go.mod/go.sum 与适用许可。断言必须是非空对象、非空名称及真实 JSON boolean；重复键、字符串 `"false"`、零断言、缺报告和空成功入口不通过。

普通消费者先在本轮独立目录编译，按 `consumer-<SHA256>` 保留一份可执行文件，报告的 `retainedBinary` 指向该文件；编译失败不能复用历史二进制。Linux 回归证据先打包为 `toolkit-index-regression.tar.gz`，上传时使用零级压缩，避免再对已压缩包进行压缩。源码/测试输入 ZIP、指定失败断言、本轮报告和二进制摘要仍保留；原生五平台的小型证据目录保持直接上传。使用报告和 Actions step 时长分别比较构建、测试与归档，冷缓存和命中缓存的运行分开统计。

本仓验证公共机制及其源码交付。业务资源识别、跨文件跳转、LSP 发布时序、真实编辑器启动时长，由 Argo CD 和 Workflows 仓库各自验证。
