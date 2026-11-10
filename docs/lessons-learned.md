# 经验教训

本文按问题记录已经发现的实现负担、已复现的依赖行为和被实际实验否决的方案。实验反例不代表线上事故；每项保留当前约束和仍可运行的回归入口。模块职责见[当前架构](architecture.md)，执行命令见[验证指南](../tests/README.md)。

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
