# 经验教训

本文保留已发现问题对应的根因、当前防线和仍可运行的回归入口。故障注入或依赖行为复现不等于线上事故；测试中的工作量、分配和 RSS 阈值也不是任意工作区的耗时保证。模块职责见[当前架构](architecture.md)，运行方式见[验证指南](../tests/README.md)。

## 查询结果少，不代表查询工作量少

**问题。** 只限制返回结果数量的 `SymbolsWithPrefix` 仍可能在一个读锁内遍历整个 scope；外层请求取消不能中断内层扫描。

**根因。** 结果容量、遍历工作量和持锁时间是不同限制，低命中或零命中也需要扫描文件与符号。

**修复。** `SymbolsWithPrefixContext` 分别限制结果和文件/符号访问量，每批最多 128 次访问；匹配和排序在锁外，等锁可取消，查询 scope generation 变化时拒绝结果。已有 entry 的 membership 链接支持游标，无需复制全部 URI；批间保留调度机会。旧 API 保持兼容，消费者需要显式迁移，并检查自身外层锁。

**防回归。** [`index_context.go`](../tests/consumer/index_context.go) 覆盖预算、零匹配、pending 文件、取消、scope 变化与小预算分配；`go test -race -count=1 ./index` 补充锁边界和 generation 不回绕检查。`index-query-budget`、`index-query-files`、`index-query-scope` 消融必须成功构建并命中指定行为断言。提高预算不能作为响应性改善的证据。

## 引用文件不应被迫声明人工 owner

**问题。** 要求每个引用都有声明 owner，会迫使仅包含跨文件引用的 YAML 构造没有业务含义的符号。

**根因。** 公共索引将业务父子关系与目标 key、引用位置混在一起，增加了提取和迁移负担。

**修复。** [`Facts`](../index/types.go) 使用平铺 symbol/reference，接受 reference-only 文件；保留文件内 symbol ID 和 scope/key，业务归属及调用图留在消费者。

**防回归。** [`relations.go`](../tests/consumer/relations.go) 的 reference-only 用例检查无人工声明时的引用、目标到达/移除与导航位置。Argo fixture 只证明消费者能在公共机制上表达这些事实，不证明全部 Argo 语义。

## 抽出函数后，生命周期状态仍可能重复

**问题。** 只有 pending Queue 时，两个插件仍各自保存 running、overflow、扫描轮次、打开文本字节数和为复用保留的公共 facts。

**根因。** 抽取同名函数没有明确状态的唯一所有者，消费者仍承担相同的生命周期账目。

**修复。** `WorkQueue` 管理执行预约及后续通知，`Membership` 只管理扫描轮次并借用消费者文件表的 observation，`Documents.OpenBytes` 提供打开输入账目，`Index.Reuse` 使用内部同 scope 摘要。消费者继续拥有业务状态及跨模块同步；读入、分析、执行票据不能互相替代。

**防回归。** [`workqueue.go`](../tests/consumer/workqueue.go) 检查公平性、运行期间通知、删除重建和旧 worker 完成；[`reuse.go`](../tests/consumer/reuse.go) 检查同 scope、原票据和不重复复制；[`workspace.go`](../tests/consumer/workspace.go) 检查接受、拒绝、关闭后的 OpenBytes。

## 代码更短仍可能破坏资源上界或当前目录响应

**问题。** collect-all-sort-truncate 可以返回正确符号，却使小 limit 查询的临时分配随全部命中增长；单 Scanner 游标能完整枚举目录，却会让当前目录等待大后台目录。

**根因。** 结果正确性无法单独证明分配有界、队列操作复杂度或发现优先级。去重身份与仍需执行的工作对象也有不同生命周期。

**修复。** [`query.go`](../index/query.go) 保留有界 heap；[`scanner.go`](../workspace/scanner.go) 保留 active 与至多一个 paused 游标；[`queue.go`](../workspace/queue.go) 使用 map 和分队列链表。完成目录保留去重身份，释放不再需要的工作对象。新旧查询分别保留适合其取消与分配约束的处理方式。

**防回归。** [`index.go`](../tests/consumer/index.go) 检查小 limit 分配和确定排序；[`workspace.go`](../tests/consumer/workspace.go) 在后台扫描推进后提升目录，检查有界批次内得到候选及最终不漏不重；[`coordination.go`](../tests/consumer/coordination.go) 检查通知到执行的输入所有权。工作量界限不等于 OS 调用的毫秒时限。

## 原生对象关闭后，回调句柄仍可能保留 context

**问题。** 当前 go-tree-sitter v0.25.0 的 progress-options 路径会保留捕获调用 context 的回调；只关闭 parser/tree 不足以释放该持有链。

**根因。** 原生对象与 Go 回调句柄的生命周期并不相同，输入 context 不能留在未释放的依赖回调中。

**修复。** [`parser.go`](../yamlview/internal/parser/parser.go) 使用没有 progress options 的 `Parse`，依靠原生时间片恢复同一次解析并检查取消。两个固定 OS 线程 worker 只执行原生片段；每个调用独占 parser/输入，成功后调用方复制节点并关闭 tree，错误/取消先清理再返回。空闲 worker 不保留上一调用。

**防回归。** [`retention.go`](../tests/consumer/retention.go) 检查成功、深度错误和取消路径的 context 释放；[`resume.go`](../tests/consumer/resume.go) 检查分片恢复与取消；`retention` 消融恢复有问题的依赖路径，`resources` 检查真实 RSS。Go heap 指标不能替代原生 RSS。

## Scanner 的固定缓冲区必须逐次检查写入

**问题。** 上游 scanner 在序列化偏移 1,022 时仍可写入 4 字节并返回 1,026，超过 1,024 字节 ABI 缓冲区，触发 Tree-sitter 断言。

**根因。** 循环只检查当前偏移，未核对下一次完整写入的容量；直接截断缩进栈又会损坏支持范围内的深层 YAML。

**修复。** [本地 grammar 补丁](../yamlview/internal/parser/grammar/README.md) 用 LEB128 编码完整状态，每次写入前校验容量，超限返回明确错误。线程局部预算标志在固定线程上的每片解析内重置、读取；解析后的语义深度仍限制为 256。

**防回归。** [`budget.go`](../tests/consumer/budget.go) 覆盖 256 层、宽缩进、scalar、超限后恢复及并发；`budget` 消融必须命中指定深度断言，不能以降低支持深度来绕过错误。

## 短字符串摘要也能保留整份输入

**问题。** 从大字符串切出的短符号名仍可能持有整份输入的底层存储，摘要尺寸看起来很小却无法释放源文本。

**根因。** Go 字符串切片共享存储，复制结构体或 slice 不会自动分离其字符串数据。

**修复。** [`Index.Begin`](../index/index.go) 分离 URI/scope，`Commit` 通过 [`cloneFacts`](../index/facts.go) 分离字符串并复制可变容器；`Reuse` 复用已经分离的内部 facts。YAML View 的 `Raw` 仍允许共享输入，业务摘要要独立检查所有权并及时释放 View。

**防回归。** [`index.go`](../tests/consumer/index.go) 使用独立大字符串的小名字检查持有量，`index-retention` 消融移除克隆后须命中指定断言；[`reuse.go`](../tests/consumer/reuse.go) 检查重复复用分配。公共索引的保证不扩展到消费者自己的缓存。

## 本机构建不证明交付的是同一份源码

**问题。** 换行转换、ZIP 元数据、排序与压缩实现会改变包装字节；标准 `go mod vendor` 又不自动包含全部 native headers 和 include 实现。

**根因。** 编译成功只说明当前环境找到所需输入，不能说明模块包装、原始 Git 字节和消费者实际编译输入一致。

**修复。** checkout 使用 LF 策略；[`snapshot.py`](../tools/snapshot.py) 固定 Unix ZIP 元数据、POSIX 路径排序及不压缩存储，拒绝同版本异字节。vendored 消费者显式恢复 C 头文件、实现、来源及许可，并锁定提交、sums、合同和文件清单。

**防回归。** [`checkout_e2e.py`](../tests/checkout_e2e.py)、[`snapshot_e2e.py`](../tests/snapshot_e2e.py)、[`vendor_e2e.py`](../tests/vendor_e2e.py) 检查上述边界；[`native.py`](../tests/native.py) 验证原生独立 module，[`delivery.py`](../tests/delivery.py) 验证空缓存、仅文件代理和无 replace 的消费。模拟 ZIP 环境不等于原生 Windows，公共消费者也不等于真实 Zed。

## 有效值不等于可独立编辑的原文

**问题。** alias/merge 展开后的值可以正确，但其来源未必能独立修改；Workflow 局部修复与完整回退共用预算还会误拒绝大 alias 输入的合法修复。

**根因。** 有效值、独立来源、共享 anchor 和每次验证的工作预算承担不同责任。

**修复。** 公共 `yamlvalue` 表达 exact/derived/ambiguous 来源；插件保留 tag 政策和请求总预算。before/after 分析及回退分别获得独立预算，共享 anchor、alias 使用点和 merge 来源不直接当作独立标量编辑。

**防回归。** 公共 `yamlvalue` consumer 与 `yamlvalue-origin` 消融覆盖来源；两个插件通过真实 CLI/LSP 验证位置和修改效果，Workflows 的 `wf-semantic-origin-budget.py` 检查大投影及局部修复。

## Schema 查询必须复用正在校验的编译图

**问题。** Argo CD 已能报告 enum 错误时，从另一份 raw Schema 遍历得到的修复查询仍可能没有可用候选。

**根因。** 独立 raw-schema 遍历会与编译 validator 在引用、组合分支及候选接受规则上分歧。

**修复。** `schemaquery` 查询调用方原有 compiled schema，候选仍经原 validator 验证；未知分支和预算失败明确返回。公共包不引入 Schema registry、compiler cache 或业务 provider。

**防回归。** 公共非 Argo Schema consumer 与 `schemaquery-validation` 消融检查候选验证；CD 验证 auto/精确版本的 codeAction → resolve → 应用 → 诊断清除。删除候选验证必须在构建成功后被行为断言检出。

## 先截断再筛选会漏掉相关资源

**问题。** 全域候选超过结果上限时，先截断再按 kind/namespace 筛选会漏掉后面的相关模板或项目。

**根因。** 结果上限被错误地应用在业务筛选之前，与真正需要的候选集合不一致。

**修复。** `SymbolsWithPrefix` 在有界查询内先按消费者编码的 key 前缀筛选，再应用结果上限；交互式调用可使用对应 Context API。超限返回 Truncated，不能将其作为完整编辑闭包。

**防回归。** [`index_prefix.go`](../tests/consumer/index_prefix.go) 的 1,001 个不相关资源与 `index-prefix` 消融检查相关候选仍可见；Workflow LSP 另验证相关集合自身超限时拒绝不完整修复。

## 原文范围正确，不代表重复解码成本合理

**问题。** 大段尾部 ASCII 注释反复进入严格投影，会让语义很小的资源花费大量重复解码工作；单靠根节点范围裁剪又可能删掉真实内容。

**根因。** `Document.Span` 和 mapping 根范围可能覆盖尾注释，CST 与严格 YAML 解码器对 Unicode 换行、TAB 和非法字符的分类也不完全一致。

**修复。** `ContentSpan` 只省略已确认的 ASCII 可打印 comment 节点及允许的间隙；含 TAB 的文档走完整范围，非 ASCII/非法控制字节交严格解码器处理。投影先核对完整原文，原位置、scalar 内容、来源及输入字节计费不变。

**防回归。** [`yamlvalue_comments.go`](../tests/consumer/yamlvalue_comments.go) 检查块标量、flow、tag/merge、边界、CRLF、Unicode 换行、非法字符和原文错配。`yamlvalue-comment-input`、`yamlvalue-comment-characters`、`yamlvalue-comment-tabs` 分别检测重复大注释解码分配、真实字段丢失和被错误接受的 TAB 输入；整个请求期限仍由真实消费者验证。

## 子进程失败，不等于测试检出了故障

**问题。** 消融曾在消费者缺工具、未构建时沿用旧失败报告返回成功；只看退出码又会接受空输出或字符串 boolean，重复检查还可能覆盖先前失败。

**根因。** 报告没有绑定本轮输入和阶段，判定器未独立校验输出类型、必需断言及指定失败原因。

**修复。** 先写失败态收据，使用独立目标及输出目录；基线成功后才运行变体。检出必须同时具备成功构建、真实执行、指定 boolean 断言失败和本轮来源；检查结果按逻辑与累积。CI 进一步绑定 SHA、run、attempt、lane/platform 和实际断言清单。

**防回归。** [`runner_effectiveness_e2e.py`](../tests/runner_effectiveness_e2e.py) 检查协议判定；[`native_delivery_e2e.py`](../tests/native_delivery_e2e.py) 与 [`delivery_failures_e2e.py`](../tests/delivery_failures_e2e.py) 验证平台、工具、报告及超时的具体失败路径。`evidence_semantics_e2e.py` 以真实 fuzz/downstream 输出检查完整证据链，协议夹具不冒充产品测试。

## 修改 native include 后，缓存仍可能复用原实现

**问题。** 只修改 `scanner.c.inc` 的可编译故障曾仍通过基线深度断言，强制改变 CGO 编译输入后相同故障才被检出。

**根因。** Go 的 CGO 构建缓存不自动追踪任意 `#include` 的 `.inc` 字节，缓存身份未覆盖实际原生实现。

**修复。** replace consumer 对 `.inc`/`.h`/`.c` 计算摘要并追加到有效 `CGO_CFLAGS`，保留工具链默认优化标志；CI cache 也包含 native includes。不可变 module consumer 用全部运行输入摘要产生新 module 身份。

**防回归。** `python3 tests/ablate.py budget` 在同一缓存中构建正常基线和 include 变体，后者必须命中指定深度失败。完整 CI 默认运行全部注册 ablation，不能因为缓存命中跳过对应行为。
