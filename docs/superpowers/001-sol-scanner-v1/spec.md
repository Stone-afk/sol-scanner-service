# Solana 扫链服务 v1 规格 / Solana Scanner Service v1 Specification

**规格文档 / Spec**: [`docs/superpowers/001-sol-scanner-v1/spec.md`](spec.md)  
**技术方案 / Plan**: [`docs/superpowers/001-sol-scanner-v1/plan.md`](plan.md)  
**任务清单 / Tasks**: [`docs/superpowers/001-sol-scanner-v1/tasks.md`](tasks.md)  
**产品方案附件 / Product Design Attachment**: [`docs/superpowers/001-sol-scanner-v1/design.md`](design.md)（2026-11-22 已确认，最高产品权威 / confirmed, highest product authority）  
**基线版本 / Baseline Version**: `PLAN-v1.6.5-SOL-SCANNER-v1`  
**日期 / Date**: 2026-11-23  
**状态 / Status**: 已批准 / Approved

> 本文件定义“要什么、怎样算正确”。表结构 DDL、区块/交易消息结构体与示例 JSON 的逐字权威是 `design.md` §4、§5.3；本文件引用而不改写。
> This file defines what is required and how correctness is judged. `design.md` §4 and §5.3 remain the verbatim authority for DDL, message structs, and the example JSON.

## 1. 变更记录 / Change Log

| 版本 / Version | 日期 / Date | 变更 / Change | 范围 / Scope | 兼容性影响 / Compatibility |
|---|---|---|---|---|
| PLAN-v1.6.5-SOL-SCANNER-v1 | 2026-11-23 | 修订 / Amended（A-04 逻辑与环境分离；Human 于 2026-11-23 批准）：A-04 判定逻辑抽为离线可测纯函数，三条判定分支（跟得上→pass；跟不上且无可验证限速→fail；注入 429/TRANSIENT→recorded）必须离线自动化，不碰主网。判定优先级：① 非限速错误→fail；② 可验证限速（窗口内 429/TRANSIENT，或 live 吞吐探针判定 insufficient）→record-only；③ 否则 `last_lag>150`→fail。`last_lag>150` 单独不得成为 record-only 条件（AUD-028-02）。live 环境门禁：吞吐 insufficient/unknown 时 FA-04 记 blocked，不得记 passed；仅 gate=sufficient 且判定 pass 时 FA-04=passed。旧 FA-04 recorded 证据作废。不改表结构、消息契约、双进程边界；不声称已上线。 | PLAN-028 / T133、T134、T126 重跑；AC-076、NFR-06；`design.md` §6.6 | 无产品行为变化；仅收紧验收判定与证据语义 |
| PLAN-v1.6.4-SOL-SCANNER-v1 | 2026-11-23 | 修订 / Amended（AUD-028 路径扩权与 live 稳定性；Human 于 2026-11-23 批准）：正式批准 PLAN-028 配套路径（`ErrQueueNotReady`、处理侧不计缺口、Kafka 32MiB、`E2E_TIMEOUT`）；A-02 须独立 jsonParsed 对照 DB；A-04 仅限速时 record-only。不改表结构、消息契约；不声称已上线。 | PLAN-028 / T126；AC-074、AC-076 | 无新产品链上语义；验收对照与 live 稳定性修复 |
| PLAN-v1.6.3-SOL-SCANNER-v1 | 2026-11-23 | 修订 / Amended（写协程澄清，对齐已落地实现；Human 于 2026-11-23 批准）：① 缺口「一轮」= **空 poll** 且缓冲仍无应用位置；读到更晚 slot 只缓冲，不计轮、不等待 `pollInterval`。② slot &lt; 应用位置的重复消息丢弃时必须 **Commit** 位点。③ 停扫回退 `resumeOffset &lt; 0`（第一条消息之前）时 **只 Commit、不 Seek**（避免 Kafka `OffsetEnd`）。不改表结构、消息契约或双进程边界。 | FR-11/FR-12、AC-090/AC-093、S-04；`design.md` §3.2 | 纠正与 S-02 乱序投递冲突的假缺口语义；消费位点在丢弃已覆盖消息时前进 |
| PLAN-v1.6.2-SOL-SCANNER-v1 | 2026-11-22 | 修订 / Amended（A-026-01 / AUD-026-01；Human 于 2026-11-22 批准）：澄清装配包边界。共享 `internal/ioc` 仅保留两进程共用的 DB `InitX`；扫块 / 处理各自的 `InitX` 分属 `internal/ioc/scanner` 与 `internal/ioc/processor`，使 FR-17 与 AC-094 可同时成立。`cmd` 不得叶子装配替代 InitX。不改表结构、消息契约、启动顺序或退出码语义。 | PLAN-026（授权路径扩展；触及扫块 `cmd`/`ioc` 迁文件） / PLAN-026 path split | 无新产品行为；修复同一 `package ioc` 下 FR-17 与 AC-094 的架构互斥 |
| PLAN-v1.6.1-SOL-SCANNER-v1 | 2026-11-22 | 修订 / Amended（A-025-01；Human 于 2026-11-22 批准）：澄清扫块 `Run` 的致命与可重试错误分类。游标冲突、投递游标/停扫检查的数据库错误与 `UNSUPPORTED_VERSION` 时 `Run` 返回、不写停扫、进程退出码 1。`getBlocks`/`getBlock`（非 UNSUPPORTED_VERSION）与区块队列生产失败仍整步重试、游标不前进。不改表结构、消息契约或双进程边界。 | PLAN-025 授权路径与 FR-10/AC-069 对齐 / PLAN-025 paths + FR-10/AC-069 alignment | 无新产品行为；修复基线与实现缺口导致的验收矛盾 |
| PLAN-v1.6-SOL-SCANNER-v1 | 2026-11-22 | 修订 / Amended（Human 已批准按已确认 `design.md` 重做规格；Human 于 2026-11-22 批准本基线，需求文本不因该批准而改变）：架构改为同一仓库两个进程。`cmd/sol-scanner` 只扫块并投递区块队列；`cmd/sol-processor` 消费、解析、一块一事务落库，再按发布游标发给下游。新增 `sol_scan_cursor`。扫块顺序改为先 `FinalizedSlot()`，再按 `stepSlots` / `slotsPerWorker` 分段 `getBlocks`。连续性改由处理服务单写协程执行：skipped 只推进应用位置；无 checkpoint 时不比较父哈希，但 `startSlot` 到第一块之间必须已是 skipped。停扫后退回投递游标和应用位置。删除目标配置 `scanWindowSlots` / `fetchConcurrency`，新增 `scanner.stepSlots`、`scanner.slotsPerWorker`、`kafka.blockTopic`。解析口径、七张链上表、下游交易消息、Token-2022、失败交易、90 天不清理、不分区、不建签名索引、`LogAlerter`、`chain-api-go v0.1.0`、禁止 `replace` 保持。`AC-038`–`AC-051` 与 `INV-004` 的旧单进程语义废止。`AC-006`、`AC-007`、`AC-064`–`AC-072`、`INV-004`、`INV-005` 按双进程规则改写。新增 `AC-078`–`AC-097`、`INV-009`–`INV-013`。 | 进程边界、扫块、落库连续性、配置、游标表 / process boundary, scan, continuity, config, cursor table | 目标行为相对 v1.5 单进程基线不兼容。已落地的严格配置机制、`000001` 七表、provider 语义、解析器和 `ScanRepository` 保留；路径迁到 `internal/scanner/provider` 与 `internal/processor/parser`。现有 `internal/scanner` 直接写库的实现不是目标，不得当作已完成。 |
| PLAN-v1.5-SOL-SCANNER-v1 | 2026-11-22 | 修订：迁移目录 `scripts/postgre`；仓储与 DAO 分层。 | 路径与分层 | 无产品 SQL 行为变化 |
| PLAN-v1.4-SOL-SCANNER-v1 | 2026-11-22 | 修订：夹具 tx158 的消息 `index` 为 `"2"`，`ix_index=1`。 | 验收序号 | 无转账语义变化 |
| PLAN-v1.3-SOL-SCANNER-v1 | 2026-11-21 | 修订：发布侧放入 `internal/event/transaction`，禁止顶层 `internal/publisher/`。 | 包路径 | 无消息字段变化 |
| PLAN-v1.2-SOL-SCANNER-v1 | 2026-11-21 | 修订：补齐 `AGENTS.md` 与 `constitution.md`。 | 治理文档 | 无链上行为变化 |
| PLAN-v1.1-SOL-SCANNER-v1 | 2026-11-21 | 修订：主网只读录制使用 gitignore 的 `config/config.local.yaml`。 | 录制方式 | 无产品行为变化 |
| PLAN-v1-SOL-SCANNER-v1 | 2026-11-21 | 新增：首个全量规格基线。 | 整个需求 | 新仓库；PostgreSQL 契约与 Kafka 交易消息 v1 |

## 2. 概述 / Overview

`sol-scanner-service` 是只读 Solana 主网 finalized 扫链服务，由同一仓库里的两个进程组成。扫块进程用 HTTP 拉取 finalized 区块，把每个 slot 的原始区块或 skipped 标记投进内部区块队列。处理进程按 slot 顺序解析交易、指令级转账（含内层）和 Token-2022 余额变化，一个区块一个事务写入 PostgreSQL，再按发布游标给每笔交易发送一条 Kafka 消息（至少一次，key=签名）。服务不判断充值或提现。原始区块 JSON 只存在于区块队列消息里，不写入 PostgreSQL。

`sol-scanner-service` is a read-only Solana mainnet finalized scanner split into two processes in one repository. The scanner process fetches finalized blocks over HTTP and hands each slot to an internal block queue as a raw block or a skipped mark. The processor parses transactions, instruction-level transfers (including inner instructions), and Token-2022 balance changes, writes one block per database transaction, then emits one Kafka message per transaction (at-least-once, key = signature). Deposit and withdrawal semantics are out of scope. Raw block JSON stays in the queue message and is not stored in PostgreSQL.

核心价值 / Core value：不漏 slot、不把未 finalized 的 slot 当成空槽；连续性异常时停扫告警且不改已入库行；落库与下游发布互不阻塞；扫块与落库可以独立重启。

## 3. 调用场景 / Call Scenarios

当前范围 / Current scope:

- **US-01** 作为运维人员，我希望扫块进程在没有投递游标时从显式 `startSlot` 开始，从而不会从创世误扫。 / As an operator, I want the scanner to start at an explicit `startSlot` when no handoff cursor exists, so it never scans from genesis by accident.
- **US-02** 作为运维人员，我希望一步里的每个协程只拉取自己那一段 slot，从而失败时整步不推进投递游标。 / As an operator, I want each worker to fetch only its own slot span, so a failure does not advance the handoff cursor.
- **US-03** 作为下游业务，我希望每笔 finalized 交易收到一条带转账和 Token-2022 余额变化的消息，从而自行判断入账。 / As downstream, I receive one message per finalized transaction with transfers and Token-2022 balance changes.
- **US-04** 作为下游业务，我希望重复消息逐字节相同，从而按签名去重。 / Duplicate messages are byte-identical, so signature dedup is safe.
- **US-05** 作为运维人员，我希望链与库接不上时处理进程停止落库并告警，已入库数据继续发送，两个进程都不退出。 / On a chain/database mismatch the processor stops writing and alerts, while already stored data keeps publishing, and neither process exits.
- **US-06** 作为运维人员，我希望下游 Kafka 故障不影响落库，恢复后从发布游标补发。 / A downstream Kafka outage does not stop commits; publishing resumes from the cursor.
- **US-07** 作为开发者，我希望链访问只经过已发布的 `chain-api-go v0.1.0`，从而依赖可复现。 / Chain access goes only through released `chain-api-go v0.1.0`.

未来场景（非本次范围）/ Future, out of scope: Lark 告警、过期清理与分区、多 RPC 切换、监控指标、人工改库后的重发入口、处理服务多写实例、质押与 DEX 等更多程序、回补 `startSlot` 之前的历史。

## 4. 功能性需求 / Functional Requirements

规范强度 / Normative keywords：必须 MUST、不得 MUST NOT、可以 MAY。

### FR-01 链访问依赖 / Chain Access Dependency

- 本仓库 `go.mod` 必须 `require github.com/Stone-afk/chain-api-go v0.1.0`，且不得出现任何 `replace`。
- `solana.Client` 必须提供 `GetBlocks` 与 `GetFirstAvailableBlock`。该版本已经在当前 `go.mod` / `go.sum` 中解析到。本基线不得重新设计或重发 `chain-api-go`。

### FR-02 配置 / Configuration

命令 / Commands：

- 扫块 / scanner：`sol-scanner --config <path>`
- 处理 / processor：`sol-processor --config <path>`
- 迁移 / migrate：`migrate <up|down|status> --config <path> [--migrations-dir <path>]`

目标 YAML 键（严格解析，未知字段失败）/ Target keys:

| 键 / Key | 类型 / Type | 规则 / Rule |
|---|---|---|
| `chain.name` | string | 必填，非空、无空白 |
| `chain.genesisHash` | string | 必填，32–44 位 base58 字符 |
| `chain.startSlot` | uint64 | 可省略；省略或 `0` 表示未配置 |
| `rpc.httpURL` | string | 扫块进程必填，`http`/`https` 且有主机名；可含 `${VAR}` |
| `scanner.pollInterval` | duration | 必填，`> 0` |
| `scanner.stepSlots` | uint64 | 必填，`1..500000`。示例值 `20000` |
| `scanner.slotsPerWorker` | uint64 | 必填，`1..500000`。示例值 `500`。不要求整除 `stepSlots` |
| `database.dsn` | string | 必填，可含 `${VAR}` |
| `kafka.brokers` | []string | 必填非空，每项 `host:port`，可含 `${VAR}` |
| `kafka.blockTopic` | string | 必填，`[A-Za-z0-9._-]{1,249}`。示例 `sol-scanner.blocks` |
| `kafka.transactionTopic` | string | 处理进程必填，同一字符规则。示例 `sol-scanner.transactions` |

- 目标配置不得再接受 `scanner.scanWindowSlots` 或 `scanner.fetchConcurrency`。当前 `config/config.example.yaml` 仍是这两个旧键；实现任务必须改成上表，不得把旧键保留为别名。
- `${VAR}` 只在被对应加载器实际解码到的标量里要求存在。缺失时错误必须指出变量名，不得包含变量值、URL、DSN 或密钥。
- `LoadForMigration` 只读取 `database`。其他段出现时，不得因为其中的环境变量缺失而失败。
- `LoadForScanner` 读取 `chain`、`rpc`、`scanner`、`database`、`kafka.brokers`、`kafka.blockTopic`。`kafka.transactionTopic` 可以出现；空字符串不阻止扫块进程启动，非空则必须符合 topic 规则。加载不得要求处理进程专有行为。
- `LoadForProcessor` 读取 `chain.name`、`chain.genesisHash`、`chain.startSlot`、`scanner.pollInterval`、`database`、`kafka.brokers`、`kafka.blockTopic`、`kafka.transactionTopic`。它不得解码或展开 `rpc.httpURL`，因此缺少 `INFURA_API_KEY` 不得导致处理进程配置失败。它不连接 RPC。
- 仓库只提交 `config/config.example.yaml`（占位符）。`config/config.local.yaml` 必须被 `.gitignore` 忽略。

### FR-03 Schema 与迁移 / Schema and Migrations

- `scripts/postgre/000001_initial_schema.up.sql` 保持现有七张表，不得改写已落地 SQL。七张表是 `sol_blocks`、`sol_transactions`、`sol_transfers`、`sol_token_balance_changes`、`sol_scan_checkpoint`、`sol_publish_cursor`、`sol_scan_halts`。`.down.sql` 继续删除这七张表。
- 新增成对迁移 `scripts/postgre/000002_scan_cursor.up.sql` / `.down.sql`。Up 只创建 `design.md` §4 的 `sol_scan_cursor`：`id SMALLINT PRIMARY KEY CHECK (id = 1)`、`handed_slot BIGINT NOT NULL`、`updated_at BIGINT NOT NULL`。Down 只删除这张表。
- 两个服务运行中都不得建表或改表。启动时 schema gate 必须看到 `(1, initial_schema)` 和 `(2, scan_cursor)`。
- 迁移参数错误退出码 2；配置、数据库或迁移错误退出码 1；成功退出码 0。`status` 必须能显示 `000001 initial_schema` 与 `000002 scan_cursor` 的 applied 或 pending。

### FR-04 RPC 访问 / RPC Access

- `Reader` 必须位于 `internal/scanner/provider`，方法为 `GenesisHash`、`FinalizedSlot`、`FirstAvailableBlock`、`BlockSlots`、`Block`。全部经 `chain-api-go`。
- `Block` 的 `getBlock` 固定参数：`encoding=base64`、`transactionDetails=full`、`rewards=false`、`commitment=finalized`、`maxSupportedTransactionVersion=1`。
- `FinalizedSlot` 与 `BlockSlots` 必须使用 `commitment=finalized`。`FinalizedSlot()` 的返回值以及小于等于该返回值的 slot 都已 finalized。
- 网络错误、超时、HTTP 429 或 5xx：同一次调用最多 3 次尝试，间隔 250ms、1s。退避期间取消必须立即返回 `context.Canceled`。
- RPC 错误码、空区块：不重试，归类 `RPC`。错误码 `-32015` 归类 `UNSUPPORTED_VERSION`。其他 HTTP 4xx、协议或解码错误归类 `PERMANENT`。
- `BlockSlots(from,to)` 的结果必须全部落在 `[from,to]` 且严格递增，否则归类 `PERMANENT`。
- provider 错误字符串不得包含端点 URL、API key 或节点原始响应体。
- 现有语义已经在 `internal/rpc/provider` 落地。v1.6 只迁移包路径并删除旧路径，不改变上述语义。

### FR-05 区块与交易解析 / Block and Transaction Parsing

- `ParseBlock(slot uint64, blk *rpc.GetBlockResult) (domain.BlockBatch, error)` 必须位于 `internal/processor/parser`，是纯函数，不访问网络、数据库或日志。
- 必须取出 slot、blockhash、parentSlot、previousBlockhash、blockHeight（可空）、blockTime（可空）、交易数。
- 每笔交易必须记录 tx_index、首签名、状态（成功 1 / 失败 2）、手续费、付费账户（账户列表第 0 项）、版本（legacy=-1、0、1）。
- 失败交易不得产出 transfer 或余额变化。
- 成功交易的完整账户列表 = 静态账户 + `meta.loadedAddresses.writable` + `meta.loadedAddresses.readonly`，按此顺序。
- 交易二进制无法解码时必须返回 `parser.ErrDecode`。
- 现有解析器已经在 `internal/parser` 落地。v1.6 只迁移包路径、夹具和测试支撑路径，不改变解析口径。

### FR-06 转账解析 / Transfer Parsing

- 成功交易必须遍历全部顶层指令和内层指令。范围内指令各产出一条 transfer。范围与 kind 编码以 `design.md` §3.5 和 §4 为准。
- `ix_index` 是顶层指令从 0 起的序号。`inner_index` 是该顶层指令的内层列表中从 0 起的序号；顶层为 `-1`。
- `amount` 必须是指令里的数。`transferCheckedWithFee` 的手续费写入 `fee`，kind=7。
- mint、decimals、owner：指令自身字段优先；缺失项按账户序号先查 pre 再查 post；仍没有则 mint/owner 为空串、decimals 为 NULL。System 转账的 mint、decimals、owner 为空。
- mint 类指令的 `from_*` 为空。burn 类指令的 `to_*` 为空。
- `closeAccount`、`harvestWithheldTokensToMint` 及其他范围外指令不得产出 transfer。
- 成功交易里范围内指令的数据或账户不足时必须返回解码错误，不得跳过。
- 夹具 slot `471991643` 第 158 笔交易唯一范围内 transfer 的 `ix_index=1`、`inner_index=-1`。下游消息 `index` 必须是 `"2"`。

### FR-07 Token-2022 余额变化 / Token-2022 Balance Changes

- 成功交易中，programId 为 Token-2022 且交易前后余额不同的账户必须各记一行。缺失的 pre 或 post 按 0。owner、mint、decimals 取 post，缺失再取 pre。
- programId 为空或不是 Token-2022 的账户不得记录。`program` 本期恒为 3。

### FR-08 持久化 / Persistence

- 一个真实区块的 block、transactions、transfers、balance changes 与 checkpoint 必须在一个事务中写入。
- checkpoint 条件更新：`WHERE last_block_slot = <prev>`。无 checkpoint 时插入。影响行数不为 1 时整事务回滚并返回 `repository.ErrCheckpointConflict`。
- skipped 不得写业务行，也不得修改 checkpoint。
- 已写入的业务行不得被修改。原始区块 JSON 不得写入任何 PostgreSQL 列。
- 停扫写入 `sol_scan_halts`。`resolved_at IS NULL` 表示未处理。
- `SaveBlockBatch` 遇到“该 slot 的区块行已经存在，且 checkpoint 已经覆盖该 slot”时必须返回已应用结果，不得把重复投递变成进程失败。checkpoint 还没覆盖该 slot 却发生主键冲突时，必须返回错误，调用方以数据库错误退出。

### FR-09 投递游标 / Handoff Cursor

- `ScanCursorRepository` 位于 `internal/repository`。DAO 位于 `internal/repository/postgre`。postgre 不得 import `internal/repository`。
- 没有 `sol_scan_cursor` 行时，下一 slot 是 `startSlot`，等价于虚拟 `handed_slot = startSlot-1`。
- `AdvanceHandedSlot(prev, next)` 必须条件更新 `WHERE id=1 AND handed_slot=prev`；没有行且 `prev=startSlot-1` 时插入。影响行数不为 1 是数据库一致性错误，扫块进程以退出码 1 结束，游标保持原值。
- 只有一整步的 `getBlocks`、列表内 `getBlock` 和队列回执都成功，才把 `handed_slot` 推进到这一步右端。
- 停扫回退使用显式改写，不用向前条件比较：有 checkpoint 时 `handed_slot = last_block_slot`；没有 checkpoint 时 `handed_slot = startSlot-1`。没有行则插入。

### FR-10 扫块 / Scanning

扫块进程不得解析交易、不得写七张链上业务表、不得给下游交易 topic 发消息。

每一轮顺序必须是：

1. 若存在未处理停扫，等待，不开下一步。
2. 调用 `FinalizedSlot()`。此调用之前不得划范围，也不得调用 `getBlocks`。
3. 设下一 slot 为 `handed_slot+1`；无行时为 `startSlot`。若下一 slot 大于 finalized 返回值，等待 `pollInterval`，不得把更大的 slot 发成 skipped。
4. 左端 = 下一 slot。右端 = `min(左端+stepSlots-1, finalized返回值)`。
5. 把闭区间按 `slotsPerWorker` 切成若干段。除最后一段外每段长度等于 `slotsPerWorker`；最后一段可以更短。段数是区间长度除以 `slotsPerWorker` 后向上取整，至少 1。
6. 每个协程只对自己那一段调用一次 `BlockSlots` / `getBlocks`。不得对整步调用一次 `getBlocks`。

`getBlocks` 成功时：列表中的 slot 再 `getBlock` 并发送 `kind=block`；该协程范围内不在列表中的 slot 发送 `kind=skipped`。`getBlocks` 失败，或列表中某个 `getBlock` 在重试后仍失败：不得发送该段剩余的 skipped，整步不推进投递游标，下一轮重做整步。已经发出的区块消息允许重复，由处理侧主键消除。

`UNSUPPORTED_VERSION` 不写停扫记录。扫块进程以退出码 1 结束，游标不前进。

`Run` 错误分类（与 FR-09、AC-069 一致，不引入新产品行为）/ `Run` error classes (same as FR-09 and AC-069; no new product behavior)：

- 致命 / Fatal（`Run` 返回；不写停扫；进程退出码 1）：`UNSUPPORTED_VERSION`；`AdvanceHandedSlot` 条件冲突；`GetHandedSlot` / `AdvanceHandedSlot` / `HasUnresolvedHalt` 的数据库错误。
- 可重试 / Retriable（记录错误，等待 `pollInterval`，重做整步；游标不前进）：`getBlocks` / `getBlock` 在重试后仍失败且不是 `UNSUPPORTED_VERSION`；区块队列 `Produce` 失败。
- 等待 / Wait（不退出）：已追平 finalized；存在未处理停扫。

示例：投递游标 1000，`stepSlots=20000`，`slotsPerWorker=500`，`FinalizedSlot()` 返回 21000。左端 1001，右端 21000，40 个协程，区间依次为 `[1001,1500]` … `[20501,21000]`。返回值若只有 1800，则右端是 1800，只有 `[1001,1500]` 和 `[1501,1800]`。

### FR-11 区块队列 / Block Queue

- 消息类型位于 `internal/event/block`。一条消息一个 slot。key 是十进制 slot，不带前导零。
- 字段以 `design.md` §5.3 `BlockMessage` 为准：`version=1`、`commitment=finalized`、`kind` 只能是 `block` 或 `skipped`。`kind=block` 时 `block` 是 `getBlock` result 的 JSON 字节。`kind=skipped` 不得带 `block`。
- 同一 JSON 编码必须能经 `encoding/json` 还原为 `*rpc.GetBlockResult`，并且 `ParseBlock` 的结果与直接解析原结构体一致。做不到时停止实现并返回方案缺陷，不得改解析口径。
- 生产者必须等该批回执全部成功才返回。失败时调用方不推进投递游标。
- 消费者必须关闭自动提交。在该 slot 成功落库、被确认为 skipped，或作为「已覆盖重复」被丢弃之后，才提交该消息的位点。
- 停扫时，消费者必须把消费位点回退到与回退后的应用位置一致：对 `resumeOffset` 执行 Commit（next = offset+1）。若 `resumeOffset >= 0`，先 Seek 再 Commit；若 `resumeOffset < 0`（第一条消息之前），**只 Commit、不 Seek**（Kafka 将 `-1` 视为队尾）。没有 checkpoint 时回到该消费组开始读取 `startSlot` 的位置。应用位置与这个位点一致。

### FR-12 处理服务写协程 / Processor Apply Loop

处理进程只有一个写协程。它可以在同一协程里取消息和按 slot 重排。不得再启动第二个写库协程。应用位置是内存中的下一个 slot，不等于 checkpoint。重启后从已提交的区块队列位点恢复。

- 有 checkpoint 时，应用位置的初值是 `last_block_slot+1`。没有 checkpoint 时，初值是 `startSlot`。
- slot 小于应用位置的消息是重复投递：丢弃，不写库，**必须 Commit** 该消息位点，且不得因此失败退出。
- slot 大于应用位置的消息放入缓冲；此轮 **不计** 缺口轮，也 **不** 等待 `pollInterval`，继续 Poll。
- 应用位置上的消息连续缺少 10 轮：停扫 `GAP_UNRESOLVED`。一轮 = 一次 **空 poll**（无新消息）且缓冲里仍没有该应用位置；仅空 poll 之间等待处理配置的 `scanner.pollInterval`。先到达的更晚 slot（即使 ≥10 条）不得单独触发该停扫。
- `kind=skipped`：应用位置加一，不写业务行，不改 checkpoint，然后提交该 slot 的队列位点。
- `kind=block`：先解码再 `ParseBlock`。解码失败停扫 `DECODE_ERROR`，不提交该区块。
- 有 checkpoint 时，只有 `parentSlot == checkpoint.slot` 且 `previousBlockhash == checkpoint.blockhash` 才提交。
- `parentSlot > checkpoint.slot`：立即 `GAP_UNRESOLVED`，即使中间应用过 skipped。
- 父哈希不同：立即 `HASH_MISMATCH`。
- `parentSlot < checkpoint.slot`：立即 `PARENT_BEHIND`。
- 没有 checkpoint 时，不得把父哈希与数据库比较。从 `startSlot` 到该区块 slot 之前的每个 slot 都必须已经按 skipped 应用。缺消息则不提交。区间为空（第一块就在 `startSlot`）时允许提交。

### FR-13 停扫与告警 / Halt and Alert

- 停扫顺序必须是：先写 `sol_scan_halts`，再调用 `Alerter.SendAlert` 一次，再回退投递游标、应用位置和区块队列位点。告警失败只记日志，不撤销停扫。
- 本期实现保持 `LogAlerter`。接口名保持 `Alerter` / `SendAlert`。告警与下游 Kafka 业务消息无关。
- 停扫后：写协程不再提交新区块；发布循环继续；扫块进程不开下一步；已入库行不改。人把 `resolved_at` 填上之前，两个进程重启也不恢复扫块和落库。人确认后，扫块进程对回退区间重新 `getBlocks`。
- 两个进程都不得因为停扫自行退出。

### FR-14 Kafka 生产者 / Kafka Producer

- `internal/pkg/mqx` 提供 `Keyed[T]` 与 `Producer[T].ProduceBatch`。每条消息设置 key，值为 JSON。全部回执成功才返回 nil。
- `Flush` 的超时参数必须是毫秒 `1000`。ctx 取消时 `ProduceBatch` 必须在 2 秒内返回。`Close` 必须有界返回。
- 相对参考实现的三处修改只这些：加 key、按批等待全部回执、Flush 使用 1000。不得把单条 `Flush(int(time.Second))` 原样搬来。
- 直接依赖 `github.com/confluentinc/confluent-kafka-go/v2 v2.11.0`。编译 Kafka 相关包和两个服务进程时 `CGO_ENABLED=1`。本基线不新增其他第三方依赖。
- 空批次立即返回 nil。

### FR-15 下游交易消息 / Downstream Transaction Message

- 类型位于 `internal/event/transaction`。禁止创建 `internal/publisher/`。
- 字段、JSON 名和示例以 `design.md` §5.3 为准。`version=1`，`commitment=finalized`，`chain` 等于 `chain.name`。
- `status` 为 `SUCCESS` 或 `FAILED`。`tx_version` 为 `legacy`、`0` 或 `1`。`program` 为 `system`、`spl-token` 或 `token-2022`。`kind` 为指令名。
- `transfers[].index`：顶层为十进制 `ix_index+1`；内层为 `"<ix_index+1>.<inner_index+1>"`。
- 金额是十进制字符串。`change = post-pre`，负数带 `-`，正数不带 `+`。无 `fee` 时省略该字段。无 decimals 时 JSON 为 `null`。`block_time` 为空时输出 `0`。
- 失败交易的两个数组必须是 `[]`，不得为 `null`。
- 消息只能由已提交的库行经 `FromBlockBatch` 构建。相同行必须产出逐字节相同的 JSON。
- 夹具第 158 笔的 JSON 与 `design.md` §5.3 示例在去掉无关空白后逐字节一致，且 `transfers[0].index` 为 `"2"`。

### FR-16 发布 / Publishing

- 发布循环在处理进程中，与写协程分开。它只读已提交行，不访问 RPC，不重解析区块队列。
- 无发布游标时从库里最早的区块开始，首次成功后插入游标行。已有游标时读取 `slot > published_slot`，按 slot 升序。
- 一个区块的全部交易消息一次 `ProduceBatch`。全部成功后才把 `published_slot` 推进到该 slot。失败则整块重发，退避从 1s 倍增，上限 30s。退避期间取消必须立即返回。
- 无交易的区块不发消息，游标仍推进。
- 游标只前进。游标行的 topic 与 `kafka.transactionTopic` 不同时必须报错且不发送。
- 下游 topic 不通时，checkpoint 可以继续前进，发布游标不动，写协程仍可消费区块队列。

### FR-17 启动、退出与装配 / Startup, Exit, and Wiring

扫块进程启动顺序：加载扫块配置 → 连接并 ping PostgreSQL → schema gate 通过版本 2 → 创世哈希必须等于配置，否则拒绝启动 → 无投递游标时 `startSlot` 必须非 0，否则拒绝启动 → `startSlot` 不得小于 `FirstAvailableBlock()`，否则拒绝启动 → 有未处理停扫时进入等待，不退出。

处理进程启动顺序：加载处理配置 → 连接并 ping PostgreSQL → schema gate 通过版本 2 → 连接区块 topic 与下游 topic。它不得调用 RPC。有未处理停扫时不启动写协程，发布循环仍启动。没有 checkpoint 时，从队列里 slot 不小于 `startSlot` 的位置开始应用。

- 缺 `--config` 或存在多余位置参数：退出码 2。
- 配置、schema、创世哈希、`startSlot`、数据库或非停扫组件错误：退出码 1，并关闭已打开的连接。
- `SIGINT` / `SIGTERM`：取消组件、关闭 producer 和数据库，退出码 0。
- 停扫不是错误退出。
- 装配使用 `internal/ioc` 树内的显式 `InitX`：共享数据库装配留在 `internal/ioc`；扫块专用 InitX 在 `internal/ioc/scanner`；处理专用 InitX 在 `internal/ioc/processor`。两个 `main` 逐个调用本进程所需 InitX，并在退出时关闭。不得在 `cmd` 内叶子装配（直接 Open/Ping/schema gate/Kafka/仓储构造）替代上述 InitX。不得使用 Wire、容器对象或 `init` 注册。
- 小结构体、小字段、参数和返回值使用值语义。`Scanner`、`Processor`、provider reader、Kafka producer 这类拥有多方法和连接生命周期的对象可以保留指针。不得为了这条规则批量改写已经落地的指针。

## 5. 非功能性需求 / Non-Functional Requirements

| ID | 要求 / Requirement |
|---|---|
| NFR-01 安全 / Security | 受控文件、日志、错误和测试输出不得包含完整 URL、DSN 密码、API key、Kafka 凭据或节点原始响应体。示例 RPC 只写 `https://solana-mainnet.infura.io/v3/${INFURA_API_KEY}`。 |
| NFR-02 依赖 / Dependencies | 只使用已发布模块。禁止 `replace`。本基线新增的直接依赖只有 `confluent-kafka-go/v2 v2.11.0`。 |
| NFR-03 分层 / Layering | 目录以 `design.md` §5.1 为准。`internal/repository/postgre` 不实现领域仓储接口，不向上返回 `*gorm.DB`。SQL 只在 `scripts/postgre`。 |
| NFR-04 确定性 / Determinism | 重发的下游消息逐字节相同。同一输入的分段结果相同。 |
| NFR-05 可测试 / Testability | 普通测试离线。真实 PostgreSQL 或 Kafka 只在 `internal/test/integration`，使用 `//go:build e2e`，由 `make e2e` 运行。主网只读验收另需 `SOL_SCANNER_LIVE_E2E=1`。 |
| NFR-06 性能 / Performance | 总体验收 A-04：连续运行 10 分钟后，落后最新 finalized slot（`last_lag`）不超过 150；同时记录 `max_lag`。判定逻辑与运行环境分离：判定分支由离线测试证明（AC-076）；只有可验证的限速信号（RPC 返回 HTTP 429 或 provider `TRANSIENT`，或 live 吞吐探针判定不足）才允许只记录、不判失败；落后值本身不是限速证据。live 环境吞吐不足或无法判定时 FA-04 记为 blocked，不记为 passed。 |
| NFR-07 可观测 / Observability | `log/slog`。停扫日志含 slot 与 reason，不含密钥。 |
| NFR-08 可靠性 / Reliability | 区块队列和下游交易都是至少一次。单写库由 checkpoint 条件更新保护。本期不部署第二个写协程。 |
| NFR-09 Go / Go | `go 1.26.0`。 |
| NFR-10 数据保留 / Retention | 90 天保留是容量背景。本期不清理、不分区、不建签名索引、不建地址索引。 |

## 6. 不变量 / Invariants

| ID | 不变量 / Invariant |
|---|---|
| INV-001 | 真实区块的业务行与 checkpoint 只在同一事务内前进。 |
| INV-002 | 业务行写入后不修改；表没有状态列或回滚列。 |
| INV-003 | 发布游标只前进，且只在整块下游回执成功后前进。 |
| INV-004 | 有 checkpoint 时，父 slot 与父哈希同时接上才提交。没有 checkpoint 时不比较父哈希，但 `startSlot` 与第一块之间的每个 slot 都已经是 skipped。未通过校验的区块不入库。 |
| INV-005 | 存在未处理停扫时，扫块进程不开下一步，处理进程不启动写协程；发布循环仍可运行。 |
| INV-006 | 受控文件、日志和错误信息没有密钥明文。 |
| INV-007 | `go.mod` 没有 `replace`。 |
| INV-008 | `startSlot=0` 或省略表示未配置。零值 `domain.Checkpoint{}` 只表示尚无 checkpoint。 |
| INV-009 | 投递游标只在整步成功后前进；任一分段的 `getBlocks` 或列表内 `getBlock` 失败时不前进。 |
| INV-010 | 应用位置不是 checkpoint。skipped 只把应用位置加一。 |
| INV-011 | 大于 `FinalizedSlot()` 返回值的 slot 不进入这一步，也不得发成 skipped。 |
| INV-012 | PostgreSQL 不保存原始区块 JSON。 |
| INV-013 | 不存在 `internal/publisher/`。链访问只在 `internal/scanner/provider`。解析只在 `internal/processor/parser`。 |

## 7. 验收标准 / Acceptance Criteria

格式 / Format：Given / When / Then。`AC-001`–`AC-037` 与 `AC-052`–`AC-063`、`AC-073`–`AC-077` 继续有效。`AC-038`–`AC-051` 被 v1.6 废止，编号保留，不得复用。

### 7.1 已落地且仍然有效 / Landed and still required

- **AC-001** `GetBlocks(5,&10,finalized)` 的请求为 `getBlocks [5,10,{"commitment":"finalized"}]`；`GetBlocks(5,nil,"")` 的参数为 `[5]`。结果按节点顺序解码。
- **AC-002** `GetFirstAvailableBlock()` 的请求为 `getFirstAvailableBlock`，无参数，结果为整数。
- **AC-003** chain-api-go 场景目录与客户端方法集一致，Solana 离线重放通过。该验收属于已发布的 `v0.1.0`，本仓库不重做。
- **AC-004** 使用 Infura 对两个方法各调用一次时无错误，且 `GetBlocks` 结果落在请求区间内。这是 `v0.1.0` 发布时的检查，不重复放入每个 Plan。
- **AC-005** `github.com/Stone-afk/chain-api-go@v0.1.0` 可解析，`go.mod` 依赖该版本且无 `replace`。
- **AC-006** 合法 v1.6 配置在对应变量已设置时，由 `LoadForScanner` 返回规范化配置。未知字段、多文档、残留占位符、缺失的被引用变量都失败，错误只含变量名。
- **AC-007** 违反 FR-02 任一规则时加载失败并指出字段路径。`startSlot` 省略得到 0 且不因此失败。`scanWindowSlots` 与 `fetchConcurrency` 是未知字段并失败。
- **AC-008** 只有 `database.dsn` 的变量存在时，`LoadForMigration` 成功，不要求 RPC 或 Kafka 变量。
- **AC-009** 示例配置只有占位符；`config/config.local.yaml` 被忽略；配置错误不含 URL 或 DSN 值。
- **AC-010** 空库执行 `000001` Up 后，七张表的列、类型、空值、默认值、主键、唯一约束、CHECK 与 `design.md` §4 对应表一致，`schema_migrations` 含 `(1, initial_schema)`。
- **AC-011** 对只含 `000001` 的库执行 Down 后，七张表和该历史行删除；再次 Up 成功。
- **AC-012** 迁移参数错误退出码 2；配置错误退出码 1；`status` 能显示版本 1 和版本 2 的 applied 或 pending。
- **AC-013** 未迁移库上 `CheckSchemaVersion(ctx, db, 2)` 失败；两个版本都应用后成功。
- **AC-014** 假 Caller 上五个 Reader 方法的方法名和参数符合 FR-04。
- **AC-015** 传输错误、429、5xx 最多 3 次，等待序列是 `[250ms,1s]` 的前缀，成功即返回。
- **AC-016** RPC 错误码、空区块、`-32015`、HTTP 401、协议错误各尝试 1 次，归类依次为 `RPC`、`RPC`、`UNSUPPORTED_VERSION`、`PERMANENT`、`PERMANENT`。
- **AC-017** 退避期间取消立即返回，且 `errors.Is(err, context.Canceled)`。
- **AC-018** 底层错误含端点和密钥时，provider 错误字符串不含二者。
- **AC-019** `getBlocks` 越界或非递增结果归类 `PERMANENT`。
- **AC-020** 区块 471991643 的哈希、父 slot、父哈希、高度、时间、交易数 457 与 `design.md` §6.1 P-01 一致。
- **AC-021** 第 158 笔成功、手续费 10000、付费账户 `9uAkfUJCYh1HyKaK19wce1PctPjXhRMCGwTTVeAuPTq`、版本 legacy。
- **AC-022** 失败交易每笔一行、状态失败、无 transfer 和余额变化；笔数等于夹具中 `meta.err != null` 的交易数。
- **AC-023** 每笔交易补全后的账户列表与 jsonParsed `accountKeys` 一致，覆盖 legacy、v0、v1。
- **AC-024** 夹具至少有 1 条内层范围内 transfer，且 legacy、v0、v1 各至少 1 笔。不满足则停止并修订基线，不得另选未记录区块。
- **AC-025** program 1/2/3、kind 1–11、status 1/2、version -1/0/1、四个 halt reason 与 `design.md` §4 一致。
- **AC-026** 第 158 笔只有 1 条 transfer，`ix_index=1`、`inner_index=-1`，其余字段与 P-03 一致。
- **AC-027** 第 158 笔余额变化两行，金额与 P-04 一致，收款变化 `+96040000`。
- **AC-028** 同笔交易的 `closeAccount` 与 `harvestWithheldTokensToMint` 不产出 transfer。
- **AC-029** 全部成功交易的 transfer 集合与 jsonParsed 在序号、程序、指令名、转出、转入、金额上一致。
- **AC-030** 构造的 `transferCheckedWithFee` 其 `fee` 等于指令手续费，kind=7。
- **AC-031** 范围内指令数据被截断时返回 `parser.ErrDecode`。
- **AC-032** 所有者、代币、精度回填与 System 转账空值规则成立。
- **AC-033** Token-2022 余额变化的缺失 pre/post、非 Token-2022、空 programId 边界成立。
- **AC-034** 写 transfer 失败时，该区块在所有表都没有行，checkpoint 不变。
- **AC-035** 旧 checkpoint 与库不一致时返回 `repository.ErrCheckpointConflict`，不写任何行。
- **AC-036** 写入后读回的领域值逐字段相等，包括 `18446744073709551615`，以及 NULL 与 0 的区别。
- **AC-037** 停扫可写入；`resolved_at` 为空时 `HasUnresolvedHalt` 为 true，填写后为 false。

### 7.2 被 v1.6 废止的编号 / IDs retired by v1.6

`AC-038`–`AC-051` 描述旧的单进程窗口扫描、空窗口后移、无 checkpoint 时第一块不校验，以及 `scanner.Run` 返回 `ErrHalted`。这些不再是验收条件。对应行为由 `AC-085`–`AC-094` 取代。编号不得复用。

### 7.3 Kafka、下游消息与发布 / Kafka, downstream messages, and publishing

- **AC-052** 真实 Kafka 上一批 3 条带 key 的消息全部投递后才返回 nil，消费端 key 与 JSON 值一致。
- **AC-053** broker 不可达且 `message.timeout.ms=1000` 时返回非 nil。
- **AC-054** Kafka 不通时取消 ctx，`ProduceBatch` 在 2 秒内返回。
- **AC-055** 空批次返回 nil；不可达 broker 上 `Close` 在 6 秒内返回；Flush 参数是 1000。
- **AC-056** 由夹具第 158 笔已入库行构建的 JSON 与 `design.md` §5.3 示例逐字节一致（比较前去掉示例里的排版空白）。`index` 为 `"2"`。
- **AC-057** 失败交易两个数组为 `[]`；index 编号符合 FR-15；正负 change、省略 fee、decimals null 符合 FR-15。
- **AC-058** 一个区块含成功且有 2 条 transfer、失败、成功但无 transfer 时，发出 3 条消息，key 为各自签名；失败条为 `FAILED` 且两个数组为空；游标推进到该 slot。
- **AC-059** 发送中途失败时发布游标不动；重试整块重发且 JSON 逐字节相同。
- **AC-060** 无交易区块不发消息，发布游标仍推进。
- **AC-061** 无游标时从最早区块开始；游标不后退；topic 不符时报错且不发送。
- **AC-062** `ListBlockBatches` 按 slot 升序分组，同一区块内子行顺序稳定。
- **AC-063** 发布失败退避为 1s、2s、4s，上限 30s；退避中取消立即返回。

### 7.4 启动与进程 / Startup and processes

- **AC-064** 扫块进程没有投递游标且 `startSlot` 省略或为 0 时拒绝启动，退出码 1。
- **AC-065** 节点创世哈希与配置不同时，扫块进程拒绝启动，退出码 1。
- **AC-066** `startSlot` 小于节点最早可用区块时，扫块进程拒绝启动，退出码 1。
- **AC-067** 有未处理停扫时，扫块进程不开下一步，处理进程不启动写协程，两个进程都不退出。
- **AC-068** 停扫后发布循环继续，两个进程保持运行，直到信号才以退出码 0 结束。
- **AC-069** 扫块或处理进程遇到数据库错误、游标冲突或 `UNSUPPORTED_VERSION` 时，不写停扫记录，关闭资源，退出码 1。
- **AC-070** `SIGINT` 或 `SIGTERM` 后有序关闭，退出码 0。
- **AC-071** 任一进程缺 `--config` 或带多余位置参数时退出码 2。
- **AC-072** 下游交易 topic 一直不通时，checkpoint 继续前进，发布游标不动，区块队列仍可被写协程消费。

### 7.5 总体验收 / Final acceptance

- **AC-073** A-01：两个进程从某个 slot 连续处理 200 个 slot 后，入库区块集合与该范围 `getBlocks` 一致，相邻区块父 slot 和父哈希接上。
- **AC-074** A-02：其中 20 个区块与节点 jsonParsed 对照，交易数、签名、成败、手续费、transfer 集合、Token-2022 前后余额一致。
- **AC-075** A-03：下游 topic 中这 200 个 slot 的消息按签名去重后，条数等于库里的交易行数，且每条都能按 §5.3 解析。
- **AC-076** A-04：连续 10 分钟后 `last_lag ≤ 150` 个 slot（记录 `max_lag`）。判定按以下优先级，先命中者为准：
  1. 观察期内出现非限速错误（checkpoint/DB 错误，或 RPC 错误既非 HTTP 429 也非 provider `TRANSIENT`）→ **fail**。
  2. 存在可验证限速信号：观察期内 RPC 返回 HTTP 429 或 `TRANSIENT`；或 live 吞吐探针（同扫块 worker 数对最近 40 个 finalized slot 执行 `getBlocks`+`getBlock`）出现 429/`TRANSIENT`，或探针 slot/s 低于观察期 tip slot/s → **recorded**（只记录实测与瓶颈，不 Fatal）。
  3. 无有效样本，或 `last_lag > 150` → **fail**（必须 Fatal）。`last_lag > 150` 单独不得成为 recorded 条件。
  4. 否则 → **pass**。
  离线必测（不访问主网）：tip 慢且处理跟得上 → pass；tip 快、处理跟不上且无限速信号 → fail；注入 429/`TRANSIENT` → recorded；注入非限速 RPC 错误 → fail；吞吐门禁 insufficient 且 lag 合格 → recorded。
  FA-04 状态：判定 pass 且吞吐门禁 sufficient → passed；判定 recorded，或判定 pass 但门禁 insufficient/unknown → blocked；判定 fail → failed。live 证据须含一行 `verdict`、`gate`、`fa04`、样本数、`last_lag`、`max_lag`、限速信号数、tip 与探针速率，不得只凭退出码判定。
- **AC-077** 集成树无 `replace`；受控文件无密钥；`go test ./...`、`go vet ./...`、`gofmt -l .` 与一次 `make e2e` 通过。主网命令不在这次 `make e2e` 中。

### 7.6 v1.6 新增 / Added in v1.6

- **AC-078** `LoadForProcessor` 在 `rpc.httpURL` 含未设置的 `${INFURA_API_KEY}` 时仍成功，且不提供 RPC 客户端配置。`LoadForScanner` 在同一文件上因该变量缺失而失败。
- **AC-079** `stepSlots=20000`、`slotsPerWorker=500` 通过校验。`stepSlots=0`、超过 500000、`slotsPerWorker=0` 失败并指出字段路径。
- **AC-080** 已有 `000001` 的库执行 `000002` Up 后只增加 `sol_scan_cursor`；Down 只删除该表，七张旧表仍在。`000001` 文件字节不变。
- **AC-081** `internal/rpc/provider` 不再存在。`internal/scanner/provider` 的测试继续满足 `AC-014`–`AC-019`。
- **AC-082** `internal/parser` 不再存在。`internal/processor/parser` 的测试继续满足 `AC-020`–`AC-033`。
- **AC-083** `kind=skipped` 的区块消息 key 为 slot 十进制字符串，JSON 无 `block`。`kind=block` 的 `block` 经往返后解析结果与原 `GetBlockResult` 相同。
- **AC-084** 投递游标 1000、步长 20000、每协程 500、finalized 为 21000 时，分段正好是 `[1001,1500]` 到 `[20501,21000]` 共 40 段。乱序回执下，游标仍一次推进到 21000。
- **AC-085** finalized 为 1800 时，只有 `[1001,1500]` 与 `[1501,1800]`。1801 及以后不产生消息。
- **AC-086** 一段 `getBlocks` 返回 101 和 103 时，只对 101 和 103 调用 `getBlock`，102 发送 skipped。空列表时该段每个 slot 都是 skipped，整步成功后游标前进。
- **AC-087** `getBlocks` 失败时不发送该段 skipped，游标不动。列表中某个 `getBlock` 失败时，整步游标不动。
- **AC-088** 条件推进的 `prev` 不匹配时返回冲突，库中 `handed_slot` 不变。
- **AC-089** 无 checkpoint，`startSlot=500`，500 和 501 是 skipped，502 是第一块：提交 502，不比较父哈希。500 还没有消息时不提交 502。
- **AC-090** 应用位置 102 连续 **10 次空 poll** 仍缺：`GAP_UNRESOLVED`，checkpoint 不变。先缓冲 ≥10 条更晚 slot、再齐应用位置前缀时不得假停扫。父哈希不同：`HASH_MISMATCH`。父 slot 更小：`PARENT_BEHIND`。101 已提交、102 是 skipped、103 的父 slot 是 102：立即 `GAP_UNRESOLVED`，不提交 103。
- **AC-091** skipped 使应用位置加一，checkpoint 的 slot 和哈希不变，业务表无该 slot 行。
- **AC-092** 停扫记录先于告警。告警失败后停扫行仍在。投递游标回到最后 checkpoint slot；无 checkpoint 时回到 `startSlot-1`。已入库行不变。写协程停止后发布循环仍能发送停扫前的区块。
- **AC-093** 队列位点在成功落库、确认 skipped，或丢弃已覆盖重复消息后提交。停扫后的消费位点与回退后的应用位置相同；`resumeOffset < 0` 时只 Commit 不 Seek。重复的已覆盖区块不使进程退出，且丢弃时必须 Commit。
- **AC-094** 扫块进程的生产代码不引用 parser、`ScanRepository.SaveBlockBatch` 或交易 topic producer。处理进程的生产代码不引用 provider 或 `chain-api-go`。
- **AC-095** 区块消息生产失败时 `handed_slot` 不变。
- **AC-096** 两个命令分别是 `sol-scanner` 与 `sol-processor`。处理进程配置加载不打开 RPC。
- **AC-097** 主网验收 harness 在 DSN 主机不是 `127.0.0.1` 或 `localhost` 时拒绝运行。未设置 `SOL_SCANNER_LIVE_E2E=1` 时测试 Skip，`make e2e` 不访问主网。

## 8. 具体测试 Case / Concrete Test Cases

这里定义行为样本，不指定实现里的私有函数名。命令里的包路径以迁址完成后的路径为准。

### 8.1 配置 / Configuration

| Case | Command / Input | Expected |
|---|---|---|
| CFG-01 | `LoadForScanner` 读取含 `${INFURA_API_KEY}` 与 `${POSTGRES_DSN}` 的示例形状，变量已设置 | 成功；`stepSlots=20000`；`slotsPerWorker=500`；`blockTopic=sol-scanner.blocks` |
| CFG-02 | 同一输入去掉 `INFURA_API_KEY` | `LoadForScanner` 失败且错误含变量名；`LoadForProcessor` 成功 |
| CFG-03 | YAML 含 `scanWindowSlots` 或未知字段 | 两个加载器都失败 |
| CFG-04 | `LoadForMigration` 只提供 `POSTGRES_DSN`，文件里仍有 RPC 占位符 | 成功 |
| CFG-05 | `stepSlots=0`、`slotsPerWorker=0`、topic 含空格、URL 不是 http(s) | 失败并含字段路径，不含 URL 或 DSN 值 |

### 8.2 迁移 / Migrations

| Case | Command / Input | Expected |
|---|---|---|
| MG-01 | `migrate` 无子命令 | 退出码 2 |
| MG-02 | `migrate up --config <缺数据库变量的文件>` | 退出码 1，错误不含 DSN |
| MG-03 | `migrate status` 在只应用了 000001 的库 | 输出含 `000001 initial_schema applied` 与 `000002 scan_cursor pending` |
| MG-04 | Up 000002 后再 Down 000002 | `sol_scan_cursor` 消失；七张旧表还在 |
| MG-05 | `CheckSchemaVersion(..., 2)` | 两个版本都应用才成功 |

### 8.3 扫块 / Scanner

| Case | Input | Expected |
|---|---|---|
| S-01 | 游标 1000，步长 20000，每协程 500，finalized 21000 | 40 段，`[1001,1500]` … `[20501,21000]`；全部回执后游标 21000 |
| S-02 | S-01 的回执顺序打乱 | 游标仍一次到 21000 |
| S-03 | 段内 `getBlocks` 返回 101、103 | 只 `getBlock` 101 和 103；102 为 skipped |
| S-07 | `getBlocks` 成功且空列表 | 段内每个 slot 都是 skipped；整步成功后游标前进 |
| S-07b | `getBlocks` 超时或报错 | 没有 skipped；游标不动 |
| S-08 | 列表中的 `getBlock` 重试后失败 | 游标不动 |
| S-10 | 无游标，`startSlot` 为 0 | 扫块进程退出码 1 |
| S-11 | 创世哈希不同 | 退出码 1 |
| S-12 | `startSlot` 小于最早可用区块 | 退出码 1 |
| S-13 | 未处理停扫 | 不调用 `getBlocks`，进程不退出 |
| S-15 | finalized 1800，游标 1000，步长 20000 | 只有两段，右端 1800；不产生 1801 |
| S-16 | finalized 等于游标 | 不调用 `getBlocks`，等待 `pollInterval` |

### 8.4 处理 / Processor

| Case | Input | Expected |
|---|---|---|
| S-04 | 应用位置 102，连续 10 次空 poll 没有 102 | `GAP_UNRESOLVED`，checkpoint 不变 |
| S-04b | 应用位置 500，先 ≥10 条更晚 slot 再齐前缀与 502 | 不停扫；提交 502 |
| S-05 | 父哈希不同 | `HASH_MISMATCH`，不提交 |
| S-06 | 父 slot 更小 | `PARENT_BEHIND`，不提交 |
| S-09 | 无 checkpoint，500 与 501 skipped，第一块 502 | 提交 502，不读父哈希做比较 |
| S-09b | 无 checkpoint，500 缺失，先到 502 | 不提交 502 |
| S-14 | checkpoint 101，102 skipped，103 的父 slot 102 | 立即 `GAP_UNRESOLVED`，不提交 103 |
| S-17 | 应用位置上的 skipped | 应用位置加一；checkpoint 不变；无业务行 |
| S-18 | 停扫时已有 checkpoint slot 101 | `handed_slot` 变为 101；告警在停扫行之后 |
| S-19 | 停扫时无 checkpoint，`startSlot=500` | `handed_slot` 变为 499 |
| S-20 | `AdvanceHandedSlot` 返回游标冲突，或 `GetHandedSlot` 返回数据库错误 | `Run` 返回该错误；不写停扫；扫块进程退出码 1；`handed_slot` 不变 |
| SX-01 | 成功交易范围内指令截断 | `DECODE_ERROR`，该块不提交 |

### 8.5 存储、队列与下游 / Storage, queue, and downstream

| Case | Input | Expected |
|---|---|---|
| R-01 | 写 transfer 时失败 | 无该区块行，checkpoint 不变 |
| R-02 | 旧 checkpoint 不匹配 | `ErrCheckpointConflict`，无写入 |
| R-03 | 000001 Up/Down/Up | 七表与 `design.md` 一致，Down 后删除 |
| R-04 | 000002 Up/Down | 只增减 `sol_scan_cursor` |
| R-05 | 重复保存 checkpoint 已覆盖的 slot | 返回已应用，进程不退出，行不改 |
| K-01 | 三笔交易的区块 | 3 条消息，key=签名，失败条数组为空，游标前进 |
| K-02 | 批中失败 | 游标不动，重发逐字节相同 |
| K-03 | 下游 topic 不通 | checkpoint 前进，发布游标不动 |
| K-04 | 空交易区块 | 无消息，游标前进 |
| K-05 | 第 158 笔 | JSON 与 design §5.3 一致，`index` 为 `"2"` |
| K-06 | 取消不可达 producer | 2 秒内返回 |
| B-01 | skipped slot 102 | key `102`，`kind=skipped`，无 `block` |
| B-02 | 夹具区块 JSON 往返 | `ParseBlock` 结果与直接解析相同 |

### 8.6 主网只读 / Live read-only

A-01..A-04 只在 `final_acceptance_checks` 执行。输入是 `config/config.local.yaml` 加 `SOL_SCANNER_LIVE_E2E=1`。DSN 主机必须是本机。证据不得记录完整 URL、DSN 或 API key。

A-04 的判定逻辑另有离线测试（`internal/test/lagcheck`，不带 `e2e` tag，`go test ./...` 覆盖），用可控 tip 与 checkpoint 证明 AC-076 每个判定分支；离线测试不访问主网，不属于 `final_acceptance_checks`。

## 9. 输出格式或契约示例 / Output Format or Contract Examples

下游交易消息的完整示例就是 `design.md` §5.3 中 slot `471991643`、`tx_index=158` 的 JSON。本规格不复制第二份示例。实现必须满足的差异约束只有这些：`transfers[0].index` 为 `"2"`；两个余额变化的 `change` 为 `"-98000000"` 与 `"96040000"`；失败交易使用 `"transfers":[]` 与 `"token_balance_changes":[]`。

区块队列的完整形状：

```json
{"version":1,"chain":"solana-mainnet","commitment":"finalized","slot":102,"kind":"skipped"}
```

`kind=block` 只增加一个 `block` 字段，值是 `getBlock` result 对象，不是整段 JSON-RPC 信封，也不是 base64 原文。key 是字符串 `102`。

`sol_scan_cursor` 的一行示例：`id=1`，`handed_slot=21000`，`updated_at` 为 Unix 秒。没有行不使用 `handed_slot=0` 表示“未开始”；`startSlot=500` 且尚未投递时表为空，回退后才写入 `handed_slot=499`。

## 10. 治理/原则审查 / Governance or Principle Review

| 原则 / Principle | 审查结论 / Conclusion |
|---|---|
| `design.md` 是产品权威 | 本规格不改已确认决策。单进程扫描不再是目标。 |
| `constitution.md` 分层与连续性 | 当前宪法仍写 `internal/parser`、`internal/rpc/provider`、单进程启动和无 checkpoint 时第一块不校验。这些句子与已确认 design 冲突。在 PLAN-027 修订宪法之前，本规格和 `design.md` 是产品与目标结构的权威。修订任务只允许对齐这两个权威，不得另写一套分层。 |
| 测试先行 | 每个新行为先有失败测试。迁址任务先移动测试并观察到旧路径不存在导致的失败，再移动实现。 |
| 迁移不可改写 | `000001` 保持字节不变。新表只用 `000002`。 |
| 指针 | 新的小结构体使用值。已落地的 `*gorm.DB`、SDK 对象和多方法服务对象保留指针。 |
| 密钥 | 示例和证据继续只用占位符和本地忽略文件。 |
| 不夸大实现 | 配置机制、七表、provider 语义、解析器、一块一事务仓储已落地。扫块主循环、Kafka、下游发布、两个进程都未按新设计落地。 |

## 11. 非目标 / Out of Scope

- WebSocket、非 finalized、回滚 topic、自动修数据。
- 业务语义、地址过滤、回补 `startSlot` 之前的历史。
- 过期清理、分区、签名索引、地址索引、Lark、指标、多 RPC。
- System、SPL Token、Token-2022 以外的程序。
- 关闭账户的租金、按 transfer 计算实际到账额。
- 人工改库后的重发入口。崩溃后的自动重发保留，内容与第一次相同。
- 多个处理写库实例。
- 改写 `000001`，或把旧 `scanWindowSlots` / `fetchConcurrency` 留作兼容别名。
- 把现有 `internal/scanner` 直接写库的循环修到能通过旧 `AC-038`–`AC-051`。该循环被替换，不继续实现 PLAN-008..PLAN-013。
- 在每个 Plan 里重跑 A-01..A-04。
- 新增 `confluent-kafka-go/v2 v2.11.0` 以外的第三方依赖。

## 12. 假设与未决事项 / Assumptions and Open Points

| ID | 内容 / Item | 状态 / Status |
|---|---|---|
| ASM-01 | 夹具 tx158 的消息 index 是 `"2"`，库内 `ix_index=1`。 | 事实 / fact，夹具已核对 |
| ASM-02 | `chain-api-go v0.1.0` 已在 `go.sum` 解析。本仓库不再执行旧 PLAN-001。 | 事实 / fact |
| ASM-03 | `GetBlockResult` 的 `encoding/json` 往返足以喂给现有 `ParseBlock`。 | 待 PLAN-019 的失败测试证实。若夹具往返后解析结果不同，停止并按 F5 回到 Planner，不改解析口径。 |
| ASM-04 | `apache/kafka:3.9.0` 的单节点 KRaft 配置沿用 v1.5 已写明的端口 `19092` 与环境变量。容器不健康时按 F4 调整官方等价配置，不改消息契约。 | 推断 / inference，由 PLAN-018 证实 |
| ASM-05 | 处理进程需要 `scanner.pollInterval` 作为缺槽轮询间隔。它不需要 `stepSlots` 才能落库。 | 本规格的保守决定。加载器仍接受这两个字段，避免共享示例文件被未知字段拒绝。 |

没有留待以后决定的产品行为。ASM-03 与 ASM-04 是有边界的可行性证据，不是未写的需求。
