# sol-scanner-service 项目开发宪法

Version: 1.1.0 · Ratified: 2026-11-21 · Last amended: 2026-11-22

本文件约束本仓库的架构判断、实现取舍与质量底线。`AGENTS.md` 是项目上下文入口；非简单任务开始前应主动读取本文、已批准规格（`docs/superpowers/001-sol-scanner-v1/`）和相关源码。本文高于普通协作建议，但不高于系统/平台规则，也不高于用户在当前任务中的明确指令。

产品与数据契约的逐字权威是 `docs/superpowers/001-sol-scanner-v1/design.md`；验收权威是同目录 `spec.md`。本文规定“怎么做对”，不改写已确认的产品决策。

---

## 第一条：分层边界原则 (Layer Boundaries First)

核心：扫链服务按命令入口、配置、领域、扫块、处理/解析、事件、仓储、DAO 和基础设施分层；同一仓库两个进程互不阻塞，不能因为方便而制造反向依赖。

- **1.1** `cmd/sol-scanner` 负责扫块进程、信号和资源生命周期；`cmd/sol-processor` 负责处理进程（消费、解析、落库、下游发布）；`cmd/migrate` 独立负责迁移命令。三者不互相调用，也不在服务启动时自动运行 migration。
- **1.2** 装配树：父包 `internal/ioc` 仅提供两进程共用的 `InitDB`/`CloseDB`；扫块 `InitX` 在 `internal/ioc/scanner`，处理 `InitX` 在 `internal/ioc/processor`。`main.go` 逐个装配和关闭。业务与领域代码不得引用命令包、IoC 或测试装配。父包生产代码不得 import scanner/provider/processor/parser。
- **1.3** `internal/domain` 表达区块、交易、转账、余额变化、checkpoint、停扫等值对象与编码常量，不带 ORM、YAML、Kafka 或供应商 SDK 细节。`internal/repository` 的接口和返回值使用 domain 类型；`internal/repository/postgre` 以 entity/DAO 处理 GORM 与 SQL。
- **1.4** entity → domain 转换在 Repository 边界完成；不得把 GORM entity 或 `*gorm.DB` 透传给 scanner、processor、`event/block`、`event/transaction` 或 parser。DAO 只负责持久化读写和驱动错误，不能承载扫描策略或 Kafka 发送。
- **1.5** `internal/processor/parser` 是纯函数：输入区块结果，输出 `domain.BlockBatch`；不得访问网络、数据库或日志。`internal/scanner` 决定拉哪些 slot 并投递区块队列；不解析、不写业务表、不发下游交易 topic。`internal/processor` 单写协程按 slot 重排、校验连续性并落库；不访问 RPC。发布侧对齐 `notification-platform/internal/event`：消息契约分包在 `internal/event/block`（区块队列）与 `internal/event/transaction`（下游交易与发布游标）；禁止顶层 `internal/publisher/`。
- **1.6** `internal/scanner/provider` 封装 `chain-api-go`，统一超时、重试、错误分类与脱敏；不解析业务转账语义。`internal/event/block` 承载区块队列契约；`internal/event/transaction` 承载交易消息契约、`FromBlockBatch` 与游标 `Producer`；`internal/pkg/mqx` 只做带 key 的批量生产；`internal/pkg/alert` 只做运维告警接口（与 Kafka 业务消息无关）。
- **1.7** `internal/pkg/postgre` 只承载服务与迁移入口真实复用的 PostgreSQL 连接、schema gate 与 migration runner。新内容进入 `internal/pkg` 前，至少证明两个真实生产调用方和稳定职责；不是任意代码的兜底目录。

---

## 第二条：配置与启动契约 (Explicit Configuration and Lifecycle)

核心：配置必须显式、可校验、可脱敏；“进程启动成功”不能夸大成“已经扫到最新块”或“Kafka 已追平”。PLAN-028 总体验收完成前不得声称服务已上线。

- **2.1** 扫块配置、处理配置与迁移配置分别加载。迁移命令只依赖数据库配置，不强制要求 RPC 或 Kafka 变量；扫块配置必须满足链/扫描/库/区块 topic 约束；处理配置必须满足库/区块 topic/下游 topic 约束，且加载时不打开 RPC。
- **2.2** 扫块配置必须包含 `chain.name`、`chain.genesisHash`、`rpc.httpURL`、`scanner.*`、`database.dsn`、`kafka.brokers`、`kafka.blockTopic`。处理配置必须包含库、区块 topic 与下游 `kafka.transactionTopic` 等约束。库中无 checkpoint 时 `chain.startSlot` 必须显式且非 0；`0` 或省略表示“未配置”，不得从创世误扫。
- **2.3** RPC URL 仅接受 `http`/`https`。`${VAR}` 仅在配置实际引用时才需要；缺失或无效变量使加载失败。仓库只提交占位符示例；真实地址写在被忽略的 `config/config.local.yaml`。不得把真实 DSN、API key 写入仓库或日志。
- **2.4** 扫块启动顺序：加载扫块配置 → 连接并 ping PostgreSQL → schema gate → 校验节点创世哈希 → 无 checkpoint 时校验 `startSlot` 与最早可用区块 → 有未处理停扫时不开下一步。处理启动顺序：加载处理配置 → 连接并 ping PostgreSQL → schema gate → 连接区块 topic 与下游 topic；有未处理停扫时不启动写协程，发布循环仍启动；无 checkpoint 时从队列里 slot 不小于 `startSlot` 的位置开始应用。失败时关闭已打开连接并以非零状态退出。
- **2.5** 收到 SIGINT/SIGTERM 后取消组件、关闭 mqx producer/consumer 与数据库连接池并以 0 退出。任一进程因停扫停止扫块或写协程时，进程本身不得退出；处理侧发布循环继续排空。
- **2.6** 必需依赖在构造/装配处保证有效；业务方法不得用 `if p == nil || p.inner == nil { return }`、`if repo == nil { return nil, nil }` 之类的防御性 nil guard 吞掉装配错误。调用方契约由构造与装配保证，方法内对 receiver/依赖做 nil 早退属于禁止的傻瓜式防御。

---

## 第三条：迁移与 schema 权威原则 (Migrations Are Authoritative)

核心：`scripts/postgre` 的版本化 SQL 是 schema 唯一来源，GORM entity 只是映射。

- **3.1** 每个 migration 使用六位版本号、同名 `.up.sql`/`.down.sql` 成对提交；文件名、内容、版本和名称必须与已应用历史一致。禁止在进程启动时 `AutoMigrate` 或运行时建表改表。
- **3.2** `cmd/migrate up|down|status` 是唯一迁移入口。Up/Down 在专用连接上使用 PostgreSQL advisory lock 和事务，迁移 SQL 与 `schema_migrations` 历史记录保持原子性。
- **3.3** 已应用版本的顺序或名称漂移时应显式报错。已发布的 migration 内容必须保持不变；新的 schema 变更添加新版本，不直接改写旧 SQL。
- **3.4** Down 必须说明数据影响，并在 E2E 中验证回滚后表被删除。不可因本地测试方便而在共享或生产库执行破坏性重置。
- **3.5** 服务 schema gate 校验其所需迁移身份；不要把“当前检查 version 1”误写成“数据库不得有更新版本”。
- **3.6** 本期表不分区、不建签名索引；业务联合键用 UNIQUE，大表主键用自增 `id`（`sol_blocks` 主键仍为 `slot`）。过期清理与分区改造不在本期范围。

---

## 第四条：链数据身份、连续性与发布原则 (Slot Continuity and Publish Cursors)

核心：Solana 本期只扫 finalized；扫块先取 finalized 再分段投递；入库必须可证明连续；Kafka 至少一次投递，本服务自己记住发到哪。

- **4.1** 只使用 HTTP、`commitment=finalized`；不设回滚 topic，不实现 WebSocket 与非 finalized 确认级别。区块以 `slot` 为主身份；交易以 `(slot, tx_index)` / 签名区分；转账以 `(slot, tx_index, ix_index, inner_index)` 区分。
- **4.2** 扫块循环每一步先调用 `FinalizedSlot()`，再按 `stepSlots`/`slotsPerWorker` 分段调用 `getBlocks`/`getBlock` 投递 `kind=block|skipped`。大于 finalized 返回值的 slot 不得进入本步，也不得发成 skipped。任一段失败则整步不推进投递游标。
- **4.3** 连续性由处理进程单写协程执行，不在扫块侧校验父哈希。有 checkpoint 时：新区块的 `parentSlot` 与 `previousBlockhash` 必须接上 checkpoint；`parentSlot` 更大立即 `GAP_UNRESOLVED`，父哈希不符立即 `HASH_MISMATCH`，父 slot 更早立即 `PARENT_BEHIND`；同一应用位置连续 10 轮仍缺则 `GAP_UNRESOLVED`。无 checkpoint 时：不得把父哈希与库比较，但从 `startSlot` 到第一块之前的每个 slot 都必须已经是明确的 `skipped`；缺消息则不提交第一块。未通过校验的区块不得入库。
- **4.4** `kind=skipped` 只把应用位置加一，不写业务行，也不修改 checkpoint。一个真实区块的 block、transactions、transfers、balance changes 与 checkpoint 必须在同一事务中写入；checkpoint 条件更新失败则整事务回滚。业务行写入后不被修改；表无状态列、回滚列。
- **4.5** 停扫时：先写 `sol_scan_halts` 再告警；投递游标回退（有 checkpoint 时 `handed_slot = last_block_slot`，无 checkpoint 时 `handed_slot = startSlot-1`）；处理侧应用位置与区块队列位点退到同一处；已入库行不改。两个进程均不得因停扫自行退出。
- **4.6** 失败交易入库并发送，标记失败，不带 transfer 与余额变化。成功交易按指令级（含内层）解析范围内转账；Token-2022 账户余额变化附带 `change`；`closeAccount` 等范围外指令不产出 transfer。
- **4.7** `sol_publish_cursor` 记录本服务“已成功交给下游 Kafka 的最后一个 slot”，不是 Kafka 消费位点，也不是下游业务进度。游标只前进；整块 `ProduceBatch` 全部成功后才推进。崩溃后的自动重发内容与第一次相同；人工改库后的重发入口本期不做。
- **4.8** 消息 key 为签名；跨交易不保证分区内顺序。下游按签名去重；Token-2022 入账应使用余额 `change`，不能只看 transfer `amount`。
- **4.9** 所有链访问必须经由已发布的 `github.com/Stone-afk/chain-api-go` 版本；`go.mod` 禁止 `replace`。provider 错误字符串不得包含端点 URL、API key 或节点原始响应体。

---

## 第五条：测试先行铁律 (Test-First Imperative) — 不可协商

核心：所有新功能和 Bug 修复从可观察行为的失败测试开始，遵循 Red → Green → Refactor。

- **5.1 Red：**先写最小目标测试，运行并确认它因待实现行为缺失而失败。新 API 的编译失败可算 Red；网络、Docker、权限、测试自身错误不算。
- **5.2 Green：**写最小实现让同一测试通过；不得删除、跳过或弱化断言制造通过。需求变更时先修正测试契约并重新取得有效 Red。
- **5.3 Refactor：**保持测试通过，整理重复与命名。新增分支与错误路径必须有断言；Bug 修复要留下会复现旧故障的回归测试。
- **5.4 表格驱动：**单元与集成测试存在并列输入时优先 `tests := []struct{...}{...}` 加 `t.Run`；复杂迁移、生命周期和进程测试可使用更清楚的专门用例。
- **5.5 真实依赖：**配置、解析、扫描逻辑在普通测试中用替身或夹具验证；真实 PostgreSQL/Kafka/主网只读测试只放 `internal/test/integration`，使用 `//go:build e2e`（主网另需显式开关）。不得把密钥写入受控文件或证据。
- **5.6 数据断言：**测试应断言 slot、签名、状态、transfer 集合、余额变化、checkpoint/游标、迁移历史与错误语义；只断言退出码或不报错不完整。固化用例 ID（P-/S-/R-/K-/C-/A-*）不得无依据弱化。
- **5.7 隔离：**`make e2e` 只使用其专用 Compose 项目和测试数据库，退出时清理容器与 volume；不得指向共享/生产数据库。主网只读验收使用 gitignore 的本地配置，不回显完整 URL。
- **5.8 证据：**交付写明 Red 失败原因、Green 命令与结果、未运行检查和风险。用户明确要求暂停验证时停止，不把未验证行为说成已通过。

---

## 第六条：显式错误与可观测性原则 (Explicit Errors and Observability)

核心：失败可定位且不泄露凭据；context 和日志反映真实运行状态。

- **6.1** 用 `fmt.Errorf("...: %w", err)` 保留需要向上判断的内部原因，用 `errors.Is` / `errors.As` 分类；不要比较错误字符串。对外/启动错误不得打印 DSN、RPC token、Kafka 凭据或供应商密钥。
- **6.2** 数据库查询、迁移、RPC、Kafka 与扫描/处理/发布路径传递 `context.Context`，尊重取消与超时。资源获取成功后在失败路径关闭；迁移锁与事务在失败时释放或回滚。
- **6.3** 结构化日志使用 `log/slog`，记录命令、slot、停扫 reason、迁移版本等必要身份，不记录完整连接串、API key 或原始供应商响应。
- **6.4** 停扫时先写 `sol_scan_halts`，再调用 `Alerter.SendAlert`；告警失败只记日志，不影响停扫。`Alerter` 只服务运维告警，与下游 Kafka 消息无关。

---

## 第七条：最小必要变更原则 (Minimal Necessary Change)

核心：围绕已批准基线与当前 Plan 扩展，不把参考仓库的能力一次性移植进来。

- **7.1** 不为单一任务新增通用 CLI dispatcher、DI 容器、空的多节点 RPC 切换、HTTP 健康端口、指标体系或缓存层，除非需求和测试证明必要。
- **7.2** 修改接口时同步检查实现、转换、命令装配和测试；修改字段时从 SQL 权威契约检查到 GORM entity、domain、消息契约与测试。
- **7.3** 不修改与任务无关的文件，不回滚用户改动，不清理不相关未跟踪文件；脏工作区先看 `git status`，只处理本任务范围。
- **7.4** 参考 `evm-scanner-service` 的分层、迁移与配置约定，参考 `notification-platform` 的 mqx 思路，但不复制 EVM 重组/WS、支付钱包通知业务，也不把原 mqx 的无 key、逐条 Flush、错误 Flush 超时原样搬入。
- **7.5** 小字段和小结构体不得使用指针，包括 struct 字段、函数或方法参数、返回值以及构造器传参；应直接使用值语义。只有包含大量方法的实现对象，或持有大量依赖并负责复杂生命周期的对象（例如 app、server）才允许使用指针。不得借本条批量改写现有指针；未来触及既有指针时，不得破坏数据库 `NULL` 语义或第三方 API 契约。若冲突，必须说明事实并交由用户决定。
- **7.6 仓库内复用优先（铁律）：**实现新需求或修复缺陷前，先查找仓库内已有的封装、工具、方法、接口和相邻代码；能在保持分层边界、契约与语义正确的前提下复用时，必须优先复用。现有能力只缺少局部行为时，优先在其所属模块扩展并补测试；不得复制一份相似实现、另建平行工具包或为绕过既有接口引入新依赖。确实不能复用时，应指出具体不匹配之处，再选择最小的新实现；复用不等于把单模块逻辑强行提升到 `internal/pkg`。

---

## 第八条：语义常量原则 (Semantic Constants)

核心：有限集合的业务语义必须由其 owner 包定义类型化常量，避免裸字面量扩散。

- **8.1** 交易状态、程序、指令 kind、停扫 reason、消息 status/program/kind 等有限集合使用命名类型与 typed constants；每个常量附清晰中文注释，同文不同义必须分类型。
- **8.2** 跨层链数据语义优先归 `internal/domain`；DAO 专属语义归 `internal/repository/postgre`；迁移方向归 `internal/pkg/postgre`；迁移命令 action 归 `cmd/migrate`；Kafka 消息 wire 字符串归 `internal/event`。禁止新增全局 constants/consts 包。
- **8.3** RPC 协议值、SQL 标识/SQLSTATE/constraint、配置键、日志与错误文案不因形式相似而常量化；历史 migration 内容不修改。协议 raw 值和用于验证非法输入的 sentinel 可以保留裸字面量。
- **8.4** 常量重构不得扩大测试边界：普通测试继续离线，真实 PostgreSQL/Kafka/主网只在 e2e；支持值应复用生产常量，非法 sentinel 可直接使用裸字面量。
- **8.5** 命名常量类型应按实际底层类型提供明确的边界转换方法：`string` 使用 `ToString()`，`int16`/`int`/`int64` 等使用对应 `ToInt16()`/`ToInt()`/`ToInt64()`。同类型内部比较或 `switch` 无需转换；只有进入基础类型字段、ORM、序列化、日志等边界时才转换。

---

## 第九条：对外 README 原则 (Public README Contract)

核心：根目录 `README.md` 是面向外部读者的中英双语项目介绍，不是安装、运维、测试或内部验收使用手册。

- **9.1** README 的中文与英文必须表达相同的项目定位、当前能力、架构边界和限制。
- **9.2** README 聚焦项目价值、架构、核心能力、模块职责、技术栈和明确非目标。启动命令、migration 操作、配置步骤、Docker 生命周期、内部 Task 编号及验收流水账应放入 `AGENTS.md` 或 `docs/superpowers/`，不在 README 中展开成使用手册。
- **9.3** README 必须包含与当前生产装配一致的架构图；优先普通 Markdown 代码块内的 ASCII 图，确保各阅读器可读。
- **9.4** 架构图和能力描述只能展示当前真实存在且已接入生产入口的组件与数据流。仅有 schema/规划的预留不得画成已运行能力。
- **9.5** 修改服务入口、IoC、provider、scanner、processor、event/block、event/transaction、repository/DAO、数据库边界或 migration 生命周期时，必须同步检查 README。README 与真实实现冲突时，以代码和版本化 SQL 为事实，并在同一变更中修正文档。本 Plan（PLAN-027）不修改 README。
- **9.6** README 不得包含 API key、DSN、完整私有 endpoint 或其他凭据；也不得把测试通过或 schema 预留夸大为生产部署保证。

---

## 治理 (Governance)

- 本宪法指导 Agent 与工程师在本仓库的默认判断；`AGENTS.md` 提供具体操作入口。若两者普通建议冲突，以本文件的原则为准。产品字段与用例冲突时，以已批准的 `design.md`/`spec.md` 为准，并走规格修订流程。
- 用户当前明确指令优先于普通项目约定，系统/平台安全规则优先于用户指令及本文。不能仅凭文档要求推断已获得额外权限。
- 修改本文件属于治理变更，应说明原因、受影响的目录和验证方式，并同步核对 `AGENTS.md`、README、已批准规格与真实实现。
