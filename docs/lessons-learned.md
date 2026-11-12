# 经验教训

本文按问题记录已经发现的实现负担、已复现的依赖行为和被实际实验否决的方案。实验反例不代表线上事故；每项保留当前约束和仍可运行的回归入口。模块职责见[当前架构](architecture.md)，执行命令见[验证指南](../tests/README.md)。

## 查询结果少，不代表查询工作量少

**发生的问题。** 静态审查确认原 `SymbolsWithPrefix` 只限制结果 heap，低命中和零命中仍在一个读锁内检查整个 scope；外层请求的 context 不能打断这次内层扫描。Workflows 的展示符号还曾在 catalog 锁内复制并排序所有 URI。这里区分源码可确认的边界缺口与实际工作区的延迟，不能把未测量的用户规模描述为已复现耗时事故。

**当前处理。** 新 `SymbolsWithPrefixContext` 分开结果数量和文件/符号访问量，以既有 membership 链接保存位置，每批最多 128 次访问、锁外匹配和排序、可取消等锁。查询 scope generation 变化拒收结果，不相关 scope 变化不使查询重启；错误不交付部分 Items。旧 API 仍兼容，真实交互式效果取决于消费者是否迁移并去掉自己外层的长锁。

**回归入口。** [`index_context.go`](../tests/consumer/index_context.go) 检查小预算、零匹配、pending 文件、取消、scope 变化与小预算下不复制全表；锁边界由 `go test -race ./index` 补充。`index-query-budget`、`index-query-files`、`index-query-scope` 消融分别移除预算、防漏计文件和返回前 generation 复核，必须编译成功并命中指定行为断言。高预算是兼容合法容量，不是性能改善的证据；锁持有期、可取消性和真实 LSP 的响应仍应分别验证。

**减法实验留下的边界。** 删除重复的 scope 指针栅栏，只保留不会回绕或复用的 generation；membership 直接借用已有 entry，去掉重复 URI 和逐符号 files-map 查询。两项等价替代均通过公开消费者与 race 检查，跨 scope pending、失败 Commit 后继续 pending、删除重建仍受保护。另两项“更短”的替代未采用：删除批间 `Gosched` 后功能仍通过，但 Linux/amd64、Go 1.27.3、单 P 的 1ms 定时取消在本地 100,000 符号实验中由约 1–2ms 延至 10–27ms，恢复后回落；将新旧查询合用 eager heap 后输出也正确，却使旧 API 的 100,000 结果查询每次多约 100,000 次分配、12.8MB 临时分配。当前保留批间调度机会，以及旧 API 的 lazy heap 和新 API 可取消的逐步排序。延迟数字只描述该实验，不是跨机器时限；测试通过不等于所有替代设计都值得采用。

## 1. 引用文件不应被迫声明人工 owner

**发生的问题：API 设计负担。** 原先的符号父子树与引用 owner 要求，让只包含跨文件引用的 YAML 必须额外构造没有业务含义的声明。关联查询真正需要的是目标 key 和引用位置，人工 owner 增加了提取、校验及迁移代码。相关设计消融与实现见 [PR #1](https://github.com/lazysnail1024/k8s-crd-toolkit/pull/1)。

**当前处理。** [`Facts`](../index/types.go) 使用平铺 symbol/reference；reference-only 文件直接可接纳。公共库保留文件内 symbol ID 和 scope/key，业务归属及调用图留在消费者。

**回归入口。** [`relations.go`](../tests/consumer/relations.go) 的 `referenceOnlyCases` 使用 Application/Workflow fixture 检查无人工声明时的引用、目标到达/移除与导航位置。这些 fixture 不能扩展解释为全部 Argo 语义或任意损坏 YAML 都受支持。

## 2. 抽出函数后，生命周期状态仍可能在三处重复

**发生的问题：两插件接入后的重复所有权。** 公共 Queue 只负责 pending，消费者仍分别保留 running、overflow、扫描轮次、打开文本字节计数和为重用而保存的公共 facts。代码搬进依赖库并没有自动消除这些状态。共享生命周期改造见 [PR #2](https://github.com/lazysnail1024/k8s-crd-toolkit/pull/2)。

**当前处理。** `WorkQueue` 管理执行预约及后续通知；`Membership` 只管理轮次，使用消费者现有文件表上的 observation；`Documents.OpenBytes` 返回权威账目；`Index.Reuse` 使用内部保留的同 scope 摘要。两插件分别在 [incremental.go](https://github.com/lazysnail1024/zed-argocd/blob/main/internal/lsp/incremental.go) 与 [catalog.go](https://github.com/lazysnail1024/zed-argo-workflows/blob/main/internal/lsp/catalog.go) 组合这些机制，继续拥有业务状态与配置。

**回归入口。** [`workqueue.go`](../tests/consumer/workqueue.go) 检查小 ready 容量的公平性、运行期间再次通知、删除后重建、旧 worker 完成和扫描对账；[`reuse.go`](../tests/consumer/reuse.go) 检查同 scope、原 ticket、保留依赖和不重复复制摘要；[`workspace.go`](../tests/consumer/workspace.go) 检查接受、拒绝、关闭后的 OpenBytes。`tests/ablate.py` 中相应反例必须成功编译并命中指定失败断言。

**保留下来的原则。** 按状态所有权消除重复，比仅按同名函数抽取更有效；输入、分析和执行三种票据承担不同责任，不能合成一个万能 generation。

## 3. 代码更短仍可能破坏资源上界或当前目录响应

**发生的问题：实际消融中的反例。** [PR #1](https://github.com/lazysnail1024/k8s-crd-toolkit/pull/1) 的 collect-all-sort-truncate 能返回正确符号，却使小 `limit` 查询的临时分配随全部命中增长；单 Scanner 游标能完整且不重复地枚举目录，却使新优先目录被大后台目录阻塞；单 slice Queue 的功能用例通过，但线性查找使大量入队退化。这些是被否决的替代实现，并非当前算法的已知故障。

**当前处理。** [`query.go`](../index/query.go) 保留有界 heap；[`scanner.go`](../workspace/scanner.go) 保留 active 与至多一个 paused 游标；[`queue.go`](../workspace/queue.go) 保留 map 查找和分队列链表。已完成目录从 `seen` 中保留身份但释放工作对象，使“去重需要记住目录”不变成“永久持有目录游标”。

**回归入口。** [`index.go`](../tests/consumer/index.go) 的符号查询用例测小 limit 下的分配并检查确定排序；[`workspace.go`](../tests/consumer/workspace.go) 在后台扫描已推进后再提升目录，检查后续批次及时返回候选、最终完整且不重复，以及完成目录不重新入队；[`coordination.go`](../tests/consumer/coordination.go) 检查通知到执行的输入所有权。

**证据边界。** 这些回归覆盖输出、分配和工作量约束；单 slice Queue 的性能结论来自上述实验，现行语义测试不能独自重新证明它。工作量界限也不等于 OS 调用的毫秒时限。

## 4. 原生对象关闭了，Go 上下文仍可能被回调句柄保留

**发生的问题：可复现的依赖生命周期行为。** 当前所用 go-tree-sitter v0.25.0 的 progress-options 句柄会保留捕获调用 context 的回调；仅关闭 parser/tree 不足以消除该持有链。桥接源码的注释和 [`retention` 反例](../tests/ablate.py)保留了可复现入口。

**当前处理。** [`parser.go`](../yamlview/internal/parser/parser.go) 使用没有 progress options 的 `Parse`，依靠原生时间片恢复解析，在时间片之间检查取消；parser/tree 均在调用内关闭。输出为复制出的 Go 节点，不暴露原生句柄。

**回归入口。** [`retention.go`](../tests/consumer/retention.go) 分别覆盖成功、深度错误和取消路径的 context 释放；[`resume.go`](../tests/consumer/resume.go) 检查分片恢复不丢输入与取消；`resources` 阶段检查原生 RSS。Go heap 指标不能替代原生 RSS，而实验阈值也不构成无任何泄漏的证明。

## 5. YAML scanner 的固定序列化缓冲区必须逐次检查写入

**发生的问题：可复现的原生越界。** vendored 上游 scanner 的检查只验证当前偏移小于 1,024，偏移为 1,022 时仍可写入 4 字节并返回 1,026，触发 Tree-sitter 断言。简单截断缩进栈又会损坏支持范围内的深层 YAML。原因和对应补丁保存在随源码交付的 [grammar 说明](../yamlview/internal/parser/grammar/README.md)。

**当前处理。** 本地补丁以 LEB128 编码完整状态并在每次写入前校验缓冲区；超过预算返回显式错误。Go 桥接在锁定 OS 线程期间读取线程局部预算标志，语义深度限制仍在解析后检查。补丁没有靠丢弃栈帧假装解析成功。

**回归入口。** [`budget.go`](../tests/consumer/budget.go) 覆盖支持的 256 层、宽缩进、scalar、超限后恢复及并发；[`ablate.py`](../tests/ablate.py) 的 `budget` 反例验证不能通过降低支持深度来绕过问题。

## 6. 短字符串摘要也能意外保留整份输入

**发生的问题：已复现的存储反例。** 从大字符串切出的符号名字即使只剩几个字节，仍可能共享整份输入的底层存储。删除克隆步骤的 buildable variant 会在专门保留测试中失败；这不等同于已观察到用户生产环境 OOM。

**当前处理。** [`Index.Begin`](../index/index.go) 分离 URI/scope 字符串；`Commit` 通过 [`cloneFacts`](../index/facts.go) 分离事实字符串并复制可变容器。`Reuse` 恢复已分离的内部 facts，避免消费者或索引再次复制同一摘要。YAML View 的 `Raw` 仍允许共享输入，调用方应在摘要提取后释放 View；不能由索引的保证推导出业务缓存也没有全文引用。

**回归入口。** [`index.go`](../tests/consumer/index.go) 的摘要存储测试用来自独立大字符串的小名字检查持有量，`index-retention` 反例移除名字克隆后必须命中指定断言；[`reuse.go`](../tests/consumer/reuse.go) 检查重复重用的分配。

## 7. 本机能构建不足以证明消费者得到同一份源码

**发生的问题：交付验证中出现的边界差异。** Git 换行转换、ZIP creator 默认值、平台路径排序及 deflate 实现都能让相同逻辑源码产生不同内容字节；标准 `go mod vendor` 又不会自动保留全部原生头文件和被 include 的实现目录。相关可复现用例见 [`checkout_e2e.py`](../tests/checkout_e2e.py)、[`snapshot_e2e.py`](../tests/snapshot_e2e.py) 和 [`vendor_e2e.py`](../tests/vendor_e2e.py)。

**当前处理。** checkout 使用仓库 LF 策略；[`snapshot.py`](../tools/snapshot.py) 按相对 POSIX 路径排序、固定 Unix ZIP 元数据并采用不压缩存储，拒绝覆盖同版本不同字节。vendored 消费者显式补齐 grammar 头文件及 go-tree-sitter 的 `include`/`src`，连同来源和许可证校验。两个插件锁定源码提交、Go sums 和合同摘要，不依赖可移动分支来表示实现版本。

**回归入口。** [`native.py`](../tests/native.py) 从独立模块、无 replace 的源码快照运行公共消费者；[`delivery.py`](../tests/delivery.py) 用全新 module cache 与仅文件代理验证离线交付；[`native_delivery_e2e.py`](../tests/native_delivery_e2e.py) 和 [`delivery_failures_e2e.py`](../tests/delivery_failures_e2e.py) 检查空套件、错误平台、缺失编译器、损坏报告和超时不能沿用成功结果。原生 CI 定义见 [native.yml](../.github/workflows/native.yml)。

**证据边界。** 这些测试证明本次输入和运行条件下的公共库交付行为。模拟 Windows ZIP 环境不等于原生 Windows 验收，公共消费者通过也不等于真实 Zed 启动和所有业务跳转通过；相应结论由消费者的原生运行、LSP 和编辑器证据支持。

## 8. 能解析 YAML 不等于能独立修改投影来源

**发生的问题。** 两个插件各自维护 alias/merge 值和来源规则，迁移时还发现 Workflow 的局部修复尝试与完整范围回退共用逻辑结果预算，导致大 alias fixture 的合法修复被误拒绝。问题由真实应用修复后的诊断检查复现，不能靠增大配额掩盖。

**当前处理。** 公共 `yamlvalue` 统一值与 exact/derived/ambiguous 来源；消费者保留各自 tag 政策及整个请求的预算。每次 before/after 分析和回退获得独立且明确的预算上下文，不继承前一条失败路径的消耗。共享 anchor、alias 使用点和 merge 来源不作为独立标量编辑。

**回归入口。** 公共 `yamlvalue` 消费者与 `yamlvalue-origin` 可编译反例覆盖来源；两个插件各自 CLI/LSP fixture 校验真实诊断、位置和修复，Workflow 的 `wf-semantic-origin-budget.py` 覆盖大投影与局部修复。

## 9. Schema 查询必须复用正在校验的编译图

**发生的问题。** Argo CD 的默认 auto 版本在已能报告 enum 错误时，修复查询仍从原始 Schema 结构寻找候选，无法得到等价的可用枚举修复。独立维护 raw-schema 遍历与校验编译图还容易在组合分支和候选接受规则上分歧。

**当前处理。** 两个消费者都用公共 `schemaquery` 查询原有 compiled schema；候选继续经原 validator 验证。删去没有真实调用方的额外公共 Accepts 接口，未引入 Schema registry、compiler cache 或业务 provider。未知分支和预算失败显式返回，不把空候选当完整证明。

**回归入口。** 公共非 Argo Schema 消费者和 `schemaquery-validation` 反例；CD 的 auto/精确版本 codeAction → resolve → 应用 → 诊断清除真实链路。移除候选验证必须在编译成功后被行为断言检出。

## 10. 先截断再筛选会漏掉相关资源

**发生的问题。** 全域候选超过 1000 时，如果先取 1000 项再按 kind/namespace 筛选，排在后面的相关模板或项目不会出现，即使相关集合本身很小。

**当前处理。** `SymbolsWithPrefix` 在同一有界查询内先按消费者编码的 key 前缀筛选，再应用结果上限。不在索引锁内运行消费者回调；超限仍返回 Truncated，插件不能把它当完整编辑范围。

**回归入口。** `index_prefix.go` 的 1001 个不相关资源反例与 `index-prefix` 消融；Workflow 的真实 LSP 用例同时覆盖相关候选可见和相关候选自身超限后的修复拒绝。

## 11. 原文范围正确，不代表重复解码成本合理

**发生的问题。** Workflow 的原 128 MiB 输入边界验收中，小资源后附的大段 ASCII 注释反复进入 schema 与外部引用分析的严格投影，触发原有 5 秒期限。CPU profile 显示主要成本在 yaml.v3 和 YAML-to-JSON 解码器的注释扫描。`Document.Span` 与 mapping 根的范围都可能覆盖尾注释，不能假设根范围更短，也不能用最后一个值节点的位置截断整个文档。

**当前处理。** `yamlview` 根据现有 CST 中的真实 comment 节点建立保守 `ContentSpan`，`yamlvalue.Project` 在完整原文匹配后仅缩短解码输入。原文本、位置、块标量内容、输入字节配额和来源证明全部保留；无需在插件增加 decoder、绕过路径或投影缓存。

**已验证的边界。** 审阅复现了 CST 把 NEL/LS/PS 后的字段包含在 comment 内、但严格 YAML 解码器仍把它解释为字段的差异；非法控制字符也可能被 CST 接受，却必须由严格解码器拒绝。因此只省略 ASCII 可打印字符注释；非 ASCII 一律保留原路径。可编译消融还发现，注释之间带 TAB 的空行会被 YAML-to-JSON 解码器拒绝，不能作为普通空白删除。含任意 TAB 的文档都保留完整范围，不再复制一套缩进语法来猜测哪些 TAB 可以删除。普通字符串删尾、正则删 `#`、删尾空白或按叶节点结束位置裁剪均不能提供这个证明。

**回归入口。** [`yamlvalue_comments.go`](../tests/consumer/yamlvalue_comments.go) 校验 literal/folded chomping、字面 `#`、flow 闭括号、null、tag、merge、显式文档边界、CRLF、多文档位置、Unicode 换行、非法字符和完整原文错配。5 MiB 尾注释快照先完成 CST 解析，再单独测 Project 分配；`yamlvalue-comment-input` 可编译消融恢复全范围解码，必须命中分配断言；`yamlvalue-comment-characters` 移除字符防线，必须命中真实 Unicode 字段丢失断言；`yamlvalue-comment-tabs` 移除全篇 TAB 检查，必须命中原始 TAB 输入拒绝断言。资源阈值属于验证条件，不构成任意工作区的耗时保证；整个请求的原期限仍由真实消费者验收。
