# 文档入口

本目录描述当前代码的职责、接入方式和已有问题留下的约束。

| 文档 | 回答的问题 |
|---|---|
| [当前实现与隔离边界](architecture.md) | 每个模块处理什么、谁持有状态、如何调度文件，以及两个插件怎样使用公共库 |
| [经验教训](lessons-learned.md) | 哪些问题已经出现或被实验复现、当前怎样处理、用什么回归验证 |
| [公共 API 合同](../CONTRACT.md) | 精确签名、数据含义、错误和资源约束；消费者按字节校验的有效接口合同 |
| [开发入口](../DEVELOPMENT.md) | 本地构建、按修改范围选择验证、依赖与发布维护方式 |
| [验证指南](../tests/README.md) | 如何运行独立消费者、原生平台验证、反例和源码交付检查 |
| [alpha 发行说明](../RELEASE_NOTES.md) | `VERSION` 对应发布标签的范围；与当前主分支能力的区别 |

安装、编译条件、当前版本和许可证见[仓库首页](../README.md)。语法依赖的来源、补丁与许可保存在[grammar 说明](../yamlview/internal/parser/grammar/README.md)，属于随源码交付的依赖资料。

架构文档说明现行实现；API 的规范以 `CONTRACT.md` 为准。模块行为变化时同步修改对应章节和回归入口，消费者升级时显式更新其合同与源码锁定。

本仓仅保留当前态的架构、接口、使用/验收、许可/来源和已有问题的经验约束。运行报告保存在 `artifacts/`，不作为过程文档提交。

[`testing-ci-baseline.json`](testing-ci-baseline.json) 是仍被 [`tools/ci_report_contracts.py`](../tools/ci_report_contracts.py) 读取的机器验收输入，其 mutation 清单和指定失败断言参与报告核验；它不是待办计划或审计结论。该文件及 `tests/*-contract.json`、`tests/downstream-pins.json` 必须随对应验证规则一起维护，不能按过时叙述材料删除。`CONTRACT.md` 和 grammar 来源说明也参与源码交付及摘要校验。
