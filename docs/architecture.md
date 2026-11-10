# 当前实现与隔离边界

`k8s-crd-toolkit` 提供文件输入、YAML 结构、文本编辑、摘要索引和有界工作调度。它的输入是调用方选定的文本、目录和不透明标识；识别 Argo 资源、解释引用、决定诊断和生成 LSP 响应由插件完成。精确 API 以[公共合同](../CONTRACT.md)为准。

## 模块与状态所有者

| 模块与实现 | 输入 → 输出 | 持有的状态与依赖 |
|---|---|---|
| [`source`](../source/snapshot.go) | URI、revision、UTF-8 文本 → 不可变 `Snapshot`；字节位置 ↔ UTF-16 位置 | 快照持有文本和行索引；不读取文件、不解释 URI |
| [`yamlview`](../yamlview/view.go) | context、快照 → 多文档 `View`、结构节点、原始 span、语法问题 | Go 结构视图；依赖 `source` 和内部 parser；`Raw` 可共享输入字符串的底层存储 |
| [`yamlview/internal/parser`](../yamlview/internal/parser/parser.go) | 文本片段 → 复制出的 Go 节点 | 每次解析独占 Tree-sitter parser/tree，调用结束关闭；通过 CGO 接入 vendored grammar |
| [`edit`](../edit/plan.go) | 基础快照、下一 revision、替换计划 → 校验后的新快照 | 无持久状态；依赖 `source`；不执行用户文件写入 |
| [`index`](../index/index.go) | URI、scope、revision、`Facts` → 查询结果和直接受影响 URI | 文件条目、scope 状态、定义与反向依赖、容量计数；依赖 `source`，不持有全文或 YAML 树 |
| [`workspace.Documents`](../workspace/documents.go) | 已接受的编辑器文本或带票据的磁盘结果 → 快照 | 打开缓冲区、读票据、内部 revision 和 `OpenBytes`；关闭文件的全文不缓存 |
| [`workspace.Queue`](../workspace/queue.go) | URI、优先级 → 待执行通知 | 有界 pending 集合、三个优先级队列；`Pop` 后不跟踪执行 |
| [`workspace.WorkQueue`](../workspace/workqueue.go) | URI、优先级 → `WorkTicket` → 完成 | ready、overflow、running 和运行期间收到的后续通知；无 worker goroutine |
| [`workspace.Membership`](../workspace/membership.go) | 扫描轮次、文件观察 → 是否允许判为未见 | 当前轮次及其完成状态；文件表与每项 `Observation` 留在调用方 |
| [`workspace.Scanner`](../workspace/scanner.go) | roots、筛选策略、单批工作量 → 文件路径、问题、扫描状态 | 待处理目录、已见目录身份、active/paused 游标；串行调用，不读取文件内容 |

`Index`、`Documents`、两种队列和 `Membership` 各自同步内部状态；调用方仍须保护跨模块的“接受输入—记录文件状态—入队—发布”操作。`Scanner` 由一个所有者串行使用。公共库没有会话级总锁、文件 watcher、工作线程、重试定时器、插件配置解析或二进制发布流程。

## 文本、YAML 与编辑

### source：一种内部坐标和 revision

`Span` 是同一快照内的半开 UTF-8 字节区间。`Position` 使用零起点行号及 UTF-16 列。转换拒绝 UTF-8 字符内部、UTF-16 代理对内部和 CRLF 中间的位置，BOM 保留为原文本。`Snapshot.Apply` 按顺序应用 `Change`，每个范围相对于前一项修改后的文本；任一项非法时不改变基础快照。实现见 [`snapshot.go`](../source/snapshot.go)。

`source.Revision` 是内部输入身份，不等于 LSP 的客户端 version。`Documents` 为每次接受的输入分配递增 revision；客户端版本校验、URI 规范化、文件路径归属和配置 epoch 由消费者维护。

### yamlview：保留结构及位置，不构造业务资源

[`View`](../yamlview/view.go) 保存文档边界、mapping 的有序成员、sequence、scalar、alias、损坏标记和语法问题。重复 key 保留为多个成员；alias 不展开，merge key 的含义不由此包解释。它保留原始 span 和文本，不做格式化，也不把 YAML 转成会丢失重复键和位置的普通 map。

顶层 `---` 是恢复边界，因此前一文档的损坏不会必然吞掉后续文档。当前硬限制为输入 5 MiB、结构嵌套 256 层、最多 2,000 个文档；超限或取消返回错误，语法损坏则可通过 `Issues`、`Damaged`、`Complete` 表达。业务提取器决定损坏文档中的哪些事实仍然可信。

内部 parser 在一次调用中创建 parser/tree，将节点复制成 Go 值后关闭原生对象。当前桥接使用 10 ms 的原生解析时间片，在片间检查取消并恢复同一次解析；时间片不是输入总时限。vendored scanner 的序列化预算通过线程局部标志报告，Go 在解析、检查和关闭期间锁定 OS 线程。细节见 [parser](../yamlview/internal/parser/parser.go) 与 [grammar 补丁说明](../yamlview/internal/parser/grammar/README.md)。

`View` 不需要调用 `Close`，但保留 `View` 或 `Node.Raw` 可能保留输入全文。持久索引只应接收提取后的摘要；消费者自己的业务摘要也必须检查其字符串和结构的所有权。

### edit：同一基础快照上的原子校验

[`Plan`](../edit/plan.go) 的所有替换都针对同一个基础 revision。`Apply` 先验证 revision、每段 `ExpectedText`、UTF-8 边界及重叠，再逆字节顺序生成新快照。同一偏移的多次插入、替换内部的插入会被拒绝；替换边界上的插入允许。它不判断重命名语义是否安全，也不生成 LSP `WorkspaceEdit` 或保存文件。

## index：事实、失效和查询

### 模型与容量

[`Facts`](../index/types.go) 包含平铺的 `Symbols`、有位置的 `References`、无导航位置的 `Dependencies` 和 `SemanticDigest`。文件可以只包含引用。符号 ID 仅在文件内唯一；scope 内相同 key 的多个定义均保留，不自动选胜者。

`Scope`、`Key` 均由调用方构造；公共库不把它们解释成 namespace、目录或 Argo 资源类型。`SemanticDigest` 必须覆盖未直接写在 facts 字段中的相关语义。稳定的文件内 ID 与忽略位置的比较，使纯位置变化只影响本文件；真正语义变化才调度直接依赖者。实现见 [`facts.go`](../index/facts.go)。

`Limits` 分别约束文件、符号、引用加依赖条目和逻辑字符串字节数，均由调用方给出正值。`Commit` 复制输入容器并分离字符串，避免短名字切片把整份源文本留住；查询返回独立的可变容器。逻辑字节预算不等于进程 RSS，也不涵盖消费者自己持有的业务摘要。

### 提交协议

1. `Begin(uri, scope, revision)` 立即撤下旧导航位置，生成一次性 `Ticket`；旧摘要只在内部保留作语义比较和依赖调度。相同 revision 可用于配置重算，降低 revision 会被拒绝。
2. 使用本次输入生成 facts，以原 ticket 调用 `Commit`。只有当前实例、该 URI 最新且未消费的票据有效；删除、重新建立条目或更新的 `Begin` 会使旧结果失效。
3. 无效 facts 或容量不足不会消费 ticket，条目继续 pending；调用方可以在处理原因后重试。成功提交消费票据，并返回排序、去重后的 `Affected`。
4. 若调用方已证明输入字节和全部解释配置未变，可调用 `Reuse(ticket)`，直接恢复同 scope 的旧摘要。`ErrNoPrevious` 保留 ticket 供正常分析、提交使用；`Reuse` 本身不计算内容 hash。
5. `Remove` 撤销票据并释放摘要和配额。`Affected` 只表达直接依赖，后续传播和去重循环由消费者调度。

以上行为集中在 [`index.go`](../index/index.go)。缺失目标的引用也进入反向依赖，因此目标后来出现、变化或消失时能唤起调用者。

### 查询证明的边界

[`query.go`](../index/query.go) 的 `Definitions`、`References`、`Symbols` 只提供 fresh 位置；`Dependents` 为调度保留 pending 摘要的依赖。`GetFile` 的 revision 与返回 facts 来自同一受锁保护的状态。

`Symbols` 按名字做区分大小写的子串匹配，按 URI、span、ID 稳定排序，保留至多 `limit` 个候选并显式返回 `Truncated`。实现使用有界 heap，仍需遍历已知候选；它不提供任意工作区的固定耗时保证。

`State.Pending == 0` 仅说明已知文件没有等待替换。扫描完整、读入成功、配置稳定、下游分析结束、目标唯一性均需消费者的额外证据。

## workspace：输入与工作生命周期

### Documents：输入接受与磁盘结果隔离

[`Documents`](../workspace/documents.go) 的打开缓冲区是权威输入：`Open` 使旧磁盘票据失效，`Change` 在内部 revision 和全部中间尺寸检查通过后原子替换文本，`OpenBytes` 只统计实际接受的打开文本。

磁盘 I/O 发生在调用方：先 `BeginRead`，在文件大小限制内读取，再 `PublishRead`。`ErrOverlay` 要求改用打开的 `Snapshot`。过时读结果会拒绝；有效票据被 `PublishRead` 消费，即使文本随后因非法或超限而拒绝，也需重新申请票据。成功返回的关闭文件快照由调用方短期持有，不进入 Documents 缓存。

失败或取消的读操作使用 `DiscardRead(ticket)`，只释放自己的预约。权威磁盘变更使用 `Invalidate(uri)`；该操作不覆盖打开缓冲区。`Close` 释放打开文本并撤销读票据，之后需要重新读盘或由消费者确认删除。`Entries` 包括待完成读操作；`FileBytes` 和 `OpenBytes` 不代替 I/O 前的读取限额。

### Queue 与 WorkQueue：通知和执行预约

两者均以 URI 合并通知，保留尚未开始工作中的最高优先级，并在有任务的 `Current`、`Related`、`Background` 队列间按 8:4:1 服务，同一队列 FIFO。优先级不能抢占正在阻塞的 worker 或系统调用。

| 接口 | 负责到哪一步 | 消费方式 |
|---|---|---|
| [`Queue`](../workspace/queue.go) | `Pop` 前的 pending 通知 | 调用方自行维护运行中任务；满容量返回 `ErrLimit`，需背压或重试 |
| [`WorkQueue`](../workspace/workqueue.go) | ready、overflow、running、运行期间的后续通知 | `Pop` 获得 `WorkTicket`，每条退出路径都调用 `Complete`；同 URI 不会并发执行 |

`WorkQueue` 的 ready 容量和所有未完成 URI 容量分开计算；overflow 仍参与公平选取，即使 ready 容量为 1 也不会饿死低优先级。`Remove` 删除未开始通知，但正在执行的 URI 继续保留预约，直到原 worker `Complete`；重建该 URI 的工作在此后才能开始。

`Pending` 包括等待和运行，`Waiting` 区分未开始工作，`Len` 仅计算 ready，`Backlog` 计算其他等待通知。`Wake` 是合并的唤醒提示，消费者先尝试 `Pop`，无工作时才等待 `Wake` 或自己的取消信号。它不携带任务，也不会关闭。

### 三种票据各自验证一件事

| 票据 | 证明什么 | 不能替代什么 |
|---|---|---|
| `Documents.ReadTicket` | 此磁盘读结果仍可接受为输入 | 编辑器版本、配置一致性、分析结果有效性 |
| `index.Ticket` | 此分析可替换该 URI 当前等待的摘要 | 磁盘读预约、worker 互斥、整个工作区完整性 |
| `WorkQueue.WorkTicket` | 此 worker 占用的 URI 执行预约 | 输入 revision、`Commit` 校验、LSP 发布检查 |

`Complete` 返回值不是分析成功或发布许可。队列只存通知，不存快照；worker 取出任务时读取消费者已接受的最新输入，分析完成后仍使用该输入原来的 index ticket。旧 worker 不得重新 `Begin` 来使自己的结果变成“最新”。

### Membership：扫描完成后才允许对账

[`Membership`](../workspace/membership.go) 不建立第二份文件表。消费者在每个文件条目保存一个 `Observation`，在新扫描时 `Start`，发现文件或收到权威输入事件时更新为 `Observe()`；更新观察与清理文件表须使用消费者同一把锁。

仅当前成功完整结束的扫描轮次，经 `Finish(pass, true)` 后才允许 `pass.Missing(observation)` 产生删除候选。取消、失败、被新扫描取代或重复结束均不能授权删除。已经扫描过的目录中新增文件也必须记录权威事件的 observation，避免对账误删。消费者额外保护打开文件、检查配置和业务归属，并执行实际删除。

### Scanner：有界发现和当前目录响应

[`NewScanner`](../workspace/scanner.go) 只规范化路径和登记根目录，不枚举目录或读取文件内容。调用方按批调用 `Next(ctx, maxWork)`；预算为 1 到 4,096 个工作单位，每个目录项、目录打开或空目录读取都计入，普通非 YAML 文件也消耗预算。

`Prioritize(directory)` 能把根目录内尚未完成的关联目录提到后台目录前，并暂停一个后台游标；已完成目录不在同一轮重新扫描。当前最多保留两个目录句柄和 512 个目录项；完成目录仅保留去重身份，目录总数另受 `MaxDirectories` 限制。扩展名和排除的目录 basename 由插件传入，不内置 Argo 配置策略。

Scanner 不跟随根以下的符号链接；根本身为符号链接会产生显式 issue。取消可返回已经找到的部分文件；`Done` 表示枚举结束，`Complete` 还要求无 I/O/容量失败且未关闭。操作间可以检查取消，但无法强制中断一个阻塞的 OS 调用，因此工作量上界不等于毫秒时限。

## 消费者的一次处理链

1. 插件解析工作区配置，规范化 URI，判定文件和目标归属，创建自己的会话状态以及公共机制实例。
2. 接受输入或收到变更通知时，插件立即通过 `Begin` 或自身的 withdraw 路径撤下旧 facts 位置；排队期间不能继续发布旧导航位置。打开文件先更新 `Documents` 并进入 `Current` 工作；同目录或关联目录通过 `Prioritize` 与 `Related` 工作处理，后台发现无需先完成。
3. Scanner 分批产生候选路径，消费者记录 observation、读取需要的输入，并按容量背压。发现路径不代表已经识别出特殊 YAML。
4. worker 使用当前已接受的快照和对应票据完成分析流程，经 `yamlview` 获取结构，由业务提取器识别资源并构造 scope、key、facts。具体 ticket 建立时点由消费者在输入失效与接受流程中协调；满足内容与配置不变条件时可复用摘要。
5. 成功提交后调度 `Affected`，释放临时快照、View 和解析上下文；持久保存分离的索引摘要及必要的业务摘要。
6. 每次执行结束释放 `WorkTicket`；发布结果还需消费者检查客户端版本、配置、取消状态和当前请求身份。
7. 扫描完整且其轮次仍有效时对账；工作区是否足以报告“缺失目标”或执行重命名，还须综合未完成工作、读错误、资源歧义和业务分析状态。

公共消费者的具体可运行用法见 [`workspace.go`](../tests/consumer/workspace.go)、[`coordination.go`](../tests/consumer/coordination.go)、[`workqueue.go`](../tests/consumer/workqueue.go) 和 [`relations.go`](../tests/consumer/relations.go)。

## 两个插件的实际引用与隔离

两个语言服务器将公共包编译进各自 Go 程序，通过同一 Go module 源码提交接入；各自实例化 Documents、Index、WorkQueue 等，没有共享进程、缓存、文件表或 CD ↔ Workflows 的直接依赖。同一 source pin 和合同保证实现与 API 约定一致，不提供跨插件自动跳转或共享资源语义。当前两仓的 [Argo CD 锁文件](https://github.com/lazysnail1024/zed-argocd/blob/main/contracts/toolkit.lock.json)与 [Workflows 锁文件](https://github.com/lazysnail1024/zed-argo-workflows/blob/main/contracts/toolkit.lock.json)均记录：

| 锁定项 | 当前值与意义 |
|---|---|
| module | `github.com/lazysnail1024/k8s-crd-toolkit` |
| 源码提交 | `b1e411d4b3abe5c1ad70518be161c1daff717d61`；消费者实际采用的实现 |
| Go 版本标识 | `v0.0.0-20261110114800-b1e411d4b3ab`；该提交的 pseudo-version |
| 合同 | `toolkit-design-3`，`CONTRACT.md` 的 SHA-256 为 `2477677871f82f8bc9a1c84d9bb44c709edcaf03e92b421a4ffbbd2c41d28c92` |
| 交付方式 | `go-module-commit`，`published: false`；直接依赖源码，无需 alpha tag/release |

源码 pin 锁定实现，合同摘要锁定接口约定，`go.sum` 及消费者 vendored 文件清单锁定实际依赖内容。公共仓文档更新或分支前进不会自动升级插件。升级需在消费者显式更新这些锁、恢复并校验 CGO 支持文件，再跑消费者回归；不能以合同 ID 相同替代实现校验。

| 边界 | Argo CD 消费者 | Workflows 消费者 |
|---|---|---|
| 输入、索引、工作和发现 | [`internal/lsp/incremental.go`](https://github.com/lazysnail1024/zed-argocd/blob/main/internal/lsp/incremental.go) 组合 Documents、WorkQueue、Membership、Scanner、Index；查询在 `incremental_query.go` | [`internal/lsp/catalog.go`](https://github.com/lazysnail1024/zed-argo-workflows/blob/main/internal/lsp/catalog.go)、`catalog_inputs.go`、`catalog_discovery.go` 组合相同机制 |
| YAML 与事实提取 | [`internal/analysis/summary.go`](https://github.com/lazysnail1024/zed-argocd/blob/main/internal/analysis/summary.go)、`projection.go` 等处理 Application、ApplicationSet、AppProject 和项目引用 | [`internal/analysis/index.go`](https://github.com/lazysnail1024/zed-argo-workflows/blob/main/internal/analysis/index.go)、`projection.go`、`symbols.go` 等处理 Workflow 资源、模板和调用关系 |
| 文本修改 | analysis 的 fixes/fields 与 lsp/workspace 的 pins 使用 `edit` 校验计划，插件负责动作与客户端交互 | analysis 的 edits/pin/quickfixes 与 lsp 的 edits 使用 `edit`，插件负责业务适用性与客户端交互 |
| 保留在插件的策略 | `.argocd-ls.yaml`、版本/catalog、目标和资源归属、诊断、worker、watcher 与 LSP 发布 | `.argo-workflows-ls.yaml`、目标配置、外部模板绑定、调用语义、缓存策略、worker 与 LSP 发布 |
| 原生依赖交付 | [`tools/vendor_c.py`](https://github.com/lazysnail1024/zed-argocd/blob/main/tools/vendor_c.py) 校验并补齐 vendored 支持文件 | [`scripts/vendor-native.py`](https://github.com/lazysnail1024/zed-argo-workflows/blob/main/scripts/vendor-native.py) 补齐 vendored 支持文件 |

两者当前都使用 `WorkQueue` 管理实际执行；`Queue` 仍是面向仅需 pending 通知的公共接口。`Scope` 的业务划分、跨资源 key 编码、namespace、`templateRef`/项目引用解释始终由消费者定义。公共库可以索引引用和多个候选，但不能据此保证某个 Argo 跳转或修改合法。

## 验证范围

公共库通过独立 Go 消费者验证 API、竞争条件、容量、内存持有和发现响应；真实 Argo YAML fixture 仅证明消费者可以在公共机制上构造事实。源码快照、原生平台和故障注入的入口见[验证指南](../tests/README.md)。

当前文件优先与关联目录工作量由公共测试覆盖；编辑器启动耗时、LSP 通知/请求顺序、业务导航及诊断一致性由两个插件各自的集成测试负责。任何“全工作区完整”或启动时长结论，都必须有对应消费者运行的证据。
