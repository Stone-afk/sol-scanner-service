# Solana 扫链服务 v1 技术方案 / Solana Scanner Service v1 Plan

**规格文档 / Spec**: [`docs/superpowers/001-sol-scanner-v1/spec.md`](spec.md)  
**技术方案 / Plan**: [`docs/superpowers/001-sol-scanner-v1/plan.md`](plan.md)  
**任务清单 / Tasks**: [`docs/superpowers/001-sol-scanner-v1/tasks.md`](tasks.md)  
**产品方案附件 / Product Design Attachment**: [`docs/superpowers/001-sol-scanner-v1/design.md`](design.md)（2026-11-22 已确认 / confirmed）  
**基线版本 / Baseline Version**: `PLAN-v1.6.5-SOL-SCANNER-v1`  
**日期 / Date**: 2026-11-23  
**状态 / Status**: 已批准 / Approved

> **For agentic workers / 执行者须知：** 本基线已获 Human 批准。按 `tasks.md` 一次只执行一个 Plan，使用 `superpowers:subagent-driven-development` 或经批准的 `superpowers:executing-plans`。行为变更先失败测试，再最小实现。

**Goal / 目标：** 在已落地的配置机制、七表迁移、provider 语义、解析器和一块一事务仓储之上，交付扫块进程与处理进程。  
**Architecture / 架构：** `cmd/sol-scanner` 只调用 `FinalizedSlot()`、按段 `getBlocks`/`getBlock`，把 `kind=block|skipped` 写入 `internal/event/block`。`cmd/sol-processor` 单写协程解析并一块一事务落库，发布循环再按 `sol_publish_cursor` 发送下游交易。  
**Tech Stack / 技术栈：** Go 1.26.0、`chain-api-go v0.1.0`、`solana-go/v2 v2.3.0`、GORM、PostgreSQL 14.14、`confluent-kafka-go/v2 v2.11.0`（PLAN-018 才加入）、`yaml.v3`、`log/slog`。

## 1. 文档目的 / Document Purpose

本文件冻结 v1.6.1 的技术决策、目录、依赖方向、全部 Plan、两级验证、风险、回滚和 40/5/55 预算。`spec.md` 是要什么；本文件是怎么做。任务字段不在这里展开。

This file freezes v1.6.1 decisions, layout, dependencies, every plan, two-level verification, risks, rollback, and the 40/5/55 budget. `spec.md` says what; this file says how. Task fields stay in `tasks.md`.

### 修订 v1.6.1 / Amendment v1.6.1

关闭 Auditor A-025-01：PLAN-025 增加对 `internal/scanner/loop.go`（及对应聚焦测试）的窄授权，使 `Run` 对游标冲突与投递游标路径数据库错误返回致命错误，从而 AC-069 在 Run 路径上可验收。不新开 PLAN-021.1。PLAN-021 接口句仅作与 FR-09/AC-069 一致的文字对齐；代码增量记在 PLAN-025 的 T128–T129。

### 修订 v1.6.2 / Amendment v1.6.2

关闭 Auditor AUD-026-01（F5）：同一 `package ioc` 同时含 `InitProvider`/`InitScanner` 与处理侧 InitX 时，`cmd/sol-processor` 无法既遵守 FR-17 又通过 AC-094 的 `go list -deps`。选定最小包边界：父包 `internal/ioc` 仅 `InitDB`/`CloseDB`；`internal/ioc/scanner` 与 `internal/ioc/processor` 分属两进程。PLAN-026 获授权迁现有 ioc 文件并改两 `cmd` 的 import；不新开 PLAN。PLAN-025 无新 Todo——扫块 `main` 的 import 调整作为本修订配套，记在 PLAN-026。

### 修订 v1.6.3 / Amendment v1.6.3

对齐已落地的写协程行为与 `design.md` §3.2 / FR-12 / AC-090 / AC-093：缺口轮 = 空 poll；乱序进缓冲不计轮；已覆盖丢弃须 Commit；`resumeOffset < 0` 时停扫只 Commit 不 Seek。不新开 Plan/Todo（实现与聚焦测试已在 `internal/processor`）；本修订只改规格/设计/PLAN-022 接口文字。共享等待辅助可落在 `internal/pkg/ctxwait`（三处生产调用方），不单独成 Plan。Human 于 2026-11-23 批准。

### 修订 v1.6.4 / Amendment v1.6.4

Human 于 2026-11-23 授权保留并正式批准下列路径为 PLAN-028 / T126 live 稳定性配套修复（retroactive；不新开 Plan/Todo）：`internal/event/block/consumer.go`（`ErrQueueNotReady`）、`internal/processor/loop.go`（不计缺口）、`internal/ioc/scanner/block_event.go` 与 `internal/ioc/processor/processor.go`（Kafka 客户端 32MiB）、`scripts/test_docker_compose.yml` 与 `Makefile`（`E2E_TIMEOUT`）。不声称服务已上线。Auditor Findings AUD-028-01..04 的修复同属本修订授权范围（A-02 独立 jsonParsed、A-04 严格 lag、文档落盘、32MiB `shortcut`）。

### 修订 v1.6.5 / Amendment v1.6.5

A-04 逻辑与环境分离（关闭 AUD-028-02 后的判定可测性缺口）。事实：现 `assertA04` 只在 `e2e` 文件中，判定分支无离线自动化；任何 `FinalizedSlot` 错误都被当成限速；旧 FA-04 `recorded` 证据来自 lag-only 记录，已被 AUD-028-02 判无效。决策：

1. 新增测试辅助包 `internal/test/lagcheck`（不带 `e2e` tag，先例 `internal/test/jsonparsed`）：`Observe`（注入 Source 与 Clock）、`IsRateLimit`（仅 HTTP 429 或 provider `TRANSIENT`）、`ClassifyThroughput`、`Judge`、`FAStatus`。判定优先级见 spec AC-076。
2. live harness 只做适配：`provider.Reader` + `ScanRepository.GetCheckpoint` 作为 Source；观察期后停止两个进程，执行 40 slot 吞吐探针得到 gate；`Judge` 为 fail 时 `t.Fatalf`；输出唯一判定行。
3. FA-04 状态语义：passed / failed / blocked / pending（见 §15）。当前 Infura 账号只允许 blocked 或 failed；passed 仅在 gate=sufficient 时出现。

不新开 Plan：属 PLAN-028 验收收紧，新增 T133、T134；T126 的 FA-02/FA-04 证据在 T134 后由 Controller 重跑。不改生产代码、表结构、消息契约、双进程边界；不声称已上线。Human 于 2026-11-23 批准本基线（限速信号收窄为 429/TRANSIENT；高吞吐以探针 `gate=sufficient` 定义；探针 40 slot / 1.0× tip）。

## 2. 输入依据 / Inputs and Governing Sources

1. Human 指令：按已确认 `design.md` 重做计划；不把现有单进程扫描当成 PLAN-008 完成。该指令当时要求新基线先保持「待批准」；Human 于 2026-11-22 批准 `PLAN-v1.6-SOL-SCANNER-v1`。
2. `design.md`（2026-11-22，架构修订已确认）：产品与数据契约最高权威。
3. 同目录 `spec.md`。验收使用该文件的 AC，尤其是 AC-078 至 AC-097。
4. `constitution.md`、`AGENTS.md`。二者与 design 冲突时，本基线以 design 和 spec 为目标；PLAN-027 再把治理文档改齐。
5. 2026-11-22 只读代码：`go.mod`、`config/config.example.yaml`、`internal/config`、`scripts/postgre/000001_initial_schema.up.sql`、`internal/rpc/provider`、`internal/parser`、`internal/repository`、`internal/scanner`、`cmd/migrate`、`Makefile`。
6. v1.5 计划里已经写明、尚未落地的 Kafka Compose：`apache/kafka:3.9.0`，宿主端口 `19092`。

## 3. 当前实现快照 / Current Implementation Snapshot

观察日 / Observed：2026-11-22。事实与目标分开。HEAD 由 Controller 在审批包记录；本 Planner 未改仓库。

| 对象 / Object | 事实 / Observed | 对 v1.6 的含义 / Meaning |
|---|---|---|
| `go.mod` | `go 1.26.0`；`require github.com/Stone-afk/chain-api-go v0.1.0`；`go.sum` 有该版本；无 `replace`；无 Kafka 依赖 | PLAN-001 已落地，不重做 |
| `internal/config` | 严格 YAML、`${VAR}`、迁移只读 database、错误不含 URL/DSN。字段仍是 `scanWindowSlots`、`fetchConcurrency`；Kafka 只有 `transactionTopic` | 机制已落地；键集合不是目标 |
| `config/config.example.yaml` | 旧键 `scanWindowSlots: 100`、`fetchConcurrency: 8`；无 `blockTopic` | PLAN-014 替换，不留别名 |
| `scripts/postgre/000001_*` | 七张表，无 `sol_scan_cursor` | 字节保持不变 |
| `internal/pkg/postgre` | advisory lock、事务 Up/Down、`requiredSchemaMigrations` 只有 `(1, initial_schema)` | PLAN-015 追加版本 2 |
| `cmd/migrate` | `up\|down\|status` 已可用 | 不改命令语法 |
| `internal/rpc/provider` | `Reader` 五方法、重试、错误分类、脱敏已有测试 | 语义保留，包迁到 `internal/scanner/provider` |
| `internal/parser` | `ParseBlock`、转账、Token-2022、夹具与 `ErrDecode` 已有测试 | 口径保留，包迁到 `internal/processor/parser` |
| `internal/test/jsonparsed` | 夹具路径指向 `internal/parser/testdata` | 随解析器迁址 |
| `internal/repository` + `postgre` | `ScanRepository` 一块一事务、条件 checkpoint、halt；postgre 不 import repository | 保留。尚无投递游标和发布 DAO |
| `internal/pkg/alert` | `Alerter` / `LogAlerter` 已存在 | 处理进程复用，不重写告警语义 |
| `internal/scanner` | `Run` 直接调用 parser 和 `SaveBlockBatch`；`checkContinuity` 在无 checkpoint 时直接判成功；配置仍是 window/concurrency | 不是目标架构，不是 PLAN-008 完成 |
| `cmd/sol-scanner`、`cmd/sol-processor`、`internal/ioc`、`internal/event`、`internal/pkg/mqx` | 不存在 | 新 Plan 交付 |
| `scripts/test_docker_compose.yml` | 只有 `postgres:14.14`，端口 `15442` | PLAN-018 追加 Kafka |
| `Makefile` `e2e` | 项目名 `sol-scanner-service-e2e`，退出时 `down -v`；已设 `CGO_ENABLED=1` | 命令保持，只追加 Kafka 环境变量 |

推断 / Inference：现有 `internal/scanner` 测试锁定的是旧窗口循环。PLAN-021 替换这些测试，不把它们修到通过废止的 AC-038–AC-051。

## 4. 实现目标与非目标 / Implementation Goals and Non-Goals

目标 / Goals：

- 两个进程按 spec FR-10 至 FR-17 工作。
- 已落地能力只迁路径或加表，不重写解析口径和 `000001`。
- AC-001–AC-037、AC-052–AC-077、AC-078–AC-097 都有归属 Plan。AC-038–AC-051 不执行。

非目标 / Non-Goals：

- 不重做 `chain-api-go`，不增加 `replace`。
- 不把旧 scanner 修成单进程达标实现。
- 不在每个 Plan 跑 A-01–A-04。
- 不新增 `confluent-kafka-go/v2 v2.11.0` 以外的直接依赖。
- 不做 Lark、分区、清理、多写实例、回滚 topic、历史回补。

## 5. 约束符合性与关键决策 / Constraint Compliance and Key Decisions

| # | 决策 / Decision | 理由 / Rationale | 后果 / Consequence | 拒绝 / Rejected |
|---|---|---|---|---|
| D-14 | 保持两个进程和 Kafka 区块 topic | design §1.2、§2 已确认 | 扫块失败不写链上表；下游故障不挡住落库 | 继续单进程：与已确认 design 冲突 |
| D-15 | `000002_scan_cursor` 只建 `sol_scan_cursor` | 宪法禁止改写已应用迁移 | `000001` 字节不变；gate 升到版本 2 | 把新表补进 `000001` |
| D-16 | provider 迁到 `internal/scanner/provider`，parser 迁到 `internal/processor/parser`，然后删除旧目录 | design §5.1 | 迁址 Plan 只改导入和路径 | 留在旧路径并改文档将就代码 |
| D-17 | 无游标行时下一 slot 是 `startSlot`；回退写成 `startSlot-1` | spec FR-09 | `handed_slot=0` 不表示“未开始” | 用 0 同时表示未开始和回退 |
| D-18 | 不要求 `stepSlots % slotsPerWorker == 0`；末段可短 | design 的 40 协程是示例；追平尖端要向上取整 | 校验只要求两个字段分别在 `1..500000` | 不能整除就拒绝配置 |
| D-19 | 处理进程的 raw 配置把 `rpc` 解成与扫块相同的固定 `rawRPCConfig`，但不展开其中的 `${}`，也不复制进 `ProcessorConfig` | AC-078、AC-007 | 共享示例文件仍可含 RPC 占位符；`rpc` 下未知字段两边都失败 | 处理进程用 `any` 放过未知键，或强制展开 `INFURA_API_KEY` |
| D-20 | 写协程单协程取消息并应用；关闭 Kafka 自动提交 | spec FR-12、FR-11 | 停扫时 seek 到最后成功区块之后的位点并提交该回退位点 | 第二个写库协程；自动提交 |
| D-21 | 重复区块只有在 checkpoint 已覆盖该 slot 时当成已应用 | spec FR-08 | 崩溃重放不退出；checkpoint 落后却主键冲突则退出码 1 | 任何主键冲突都停扫 |
| D-22 | 区块 JSON 使用 `encoding/json` 编码 `getBlock` result | spec FR-11 | PLAN-019 用夹具证明往返。失败则停，返回 F5 | 把 base64 原文或整段 JSON-RPC 信封写入消息 |
| D-23 | E2E Kafka 使用 v1.5 已写明的 `apache/kafka:3.9.0`、`127.0.0.1:19092` | 不另选镜像 | 容器不健康按 F4 换官方等价配置，不改契约 | 每个 Plan 自带一套 broker |
| D-24 | 小结构体值语义；多方法服务对象可保留指针 | 仓库指针规则 | 不批量改写已落地指针 | 为了规则把 GORM 和 SDK 改成值 |
| D-25 | 运行时路由按 Human 已有覆盖：Cursor 三角色 Auto/inherit | 路由表要求的 `claude-opus-5-5[effort=high]` 不在可用列表；不得换成 `claude-opus-5-5-medium` | 本基线不换模型 | 静默改用 medium |

## 6. 目标架构 / Target Architecture

```text
cmd/sol-scanner/          扫块进程：参数、信号、退出码
cmd/sol-processor/        处理进程：参数、信号、退出码
cmd/migrate/              已落地，保持
config/config.example.yaml
internal/config/          LoadForScanner、LoadForProcessor、LoadForMigration
internal/domain/          已落地值对象
internal/scanner/         分段、协程、扫块循环、扫块启动门禁
internal/scanner/provider/  由 internal/rpc/provider 迁入
internal/processor/       单写协程、连续性、停扫回退
internal/processor/parser/ 由 internal/parser 迁入
internal/repository/      ScanRepository、ScanCursorRepository、PublishRepository
internal/repository/postgre/  entity 与 DAO；不 import repository
internal/event/block/     区块队列消息、生产、消费
internal/event/transaction/  下游消息与发布循环
internal/pkg/mqx/         带 key 的批量生产者
internal/pkg/alert/       已落地 LogAlerter
internal/pkg/postgre/     连接、gate、迁移 runner
internal/ioc/             两个进程各自的 InitX
internal/test/jsonparsed/
internal/test/integration/  //go:build e2e
scripts/postgre/000001_*  不改
scripts/postgre/000002_scan_cursor.{up,down}.sql
scripts/test_docker_compose.yml
```

依赖方向 / Dependency direction：

```text
cmd/sol-scanner → ioc → scanner, scanner/provider, event/block, repository(ScanCursor, Halt 查询), alert, pkg/postgre
cmd/sol-processor → ioc → processor, processor/parser, event/block, event/transaction, repository, mqx, alert, pkg/postgre
scanner → scanner/provider, event/block, ScanCursorRepository, HasUnresolvedHalt, domain
processor → processor/parser, event/block, ScanRepository, ScanCursorRepository, alert, domain
event/transaction → PublishRepository, mqx, domain
parser → domain, solana-go types
repository → domain, postgre DAO
postgre → gorm entity；不依赖 repository
domain → stdlib
```

状态所有权 / State ownership：`handed_slot` 归扫块进程推进、处理进程在停扫时回退。checkpoint 与业务行只由写协程经 `SaveBlockBatch` 写入。应用位置和“最后成功区块后的队列位点”在处理进程内存中，重启从已提交位点恢复。`published_slot` 只由发布循环推进。

## 7. 模块职责 / Module or Package Responsibilities

| 模块 / Module | Responsibility | Must Not | Change |
|---|---|---|---|
| `internal/config` | 三种加载器和 v1.6 校验 | 连接 RPC、数据库、Kafka | PLAN-014 |
| `internal/scanner/provider` | chain-api-go、超时、重试、分类、脱敏 | 解析转账 | PLAN-016 迁址 |
| `internal/scanner` | 先 `FinalizedSlot()`，再分段投递 | 解析、写业务表、发交易 topic | PLAN-021 替换现有循环 |
| `internal/processor/parser` | `ParseBlock` 纯函数 | 网络、数据库、日志 | PLAN-017 迁址 |
| `internal/processor` | 重排、连续性、落库、停扫回退 | 调用 RPC 或 chain-api-go | PLAN-022 |
| `internal/event/block` | 区块消息、key、JSON 往返、生产与消费位点 | 解析业务、写 PostgreSQL | PLAN-019 |
| `internal/event/transaction` | 下游契约、`FromBlockBatch`、发布循环 | 访问 RPC；顶层 publisher 包 | PLAN-023、PLAN-024 |
| `internal/repository` | 三个领域接口和转换 | 泄露 `*gorm.DB` 或 SQL | PLAN-020、PLAN-024 |
| `internal/repository/postgre` | entity、DAO、SQL | import repository；实现领域接口 | 同上 |
| `internal/pkg/mqx` | key、批量回执、Flush 1000 | 业务结构 | PLAN-018 |
| `internal/pkg/alert` | 已落地日志告警 | 接 Lark | 不改语义 |
| `internal/ioc` 与两个 `cmd` | `InitX`、信号、退出码 | 业务规则 | PLAN-025、PLAN-026 |
| `scripts/postgre` | 唯一 SQL 权威 | 服务进程里建表 | PLAN-015 只加 000002 |

## 8. 调研结论与来源登记 / Research Findings and Source Register

| ID | 结论 / Claim | 来源 / Source | 日期 / Date | 状态 / Status | 可信度 / Confidence |
|---|---|---|---|---|---|
| RS-20 | 两个进程、先 `FinalizedSlot()`、每段一次 `getBlocks`、失败不推进游标 | `design.md` §3.1 | 2026-11-22 | fact，Human 已确认 | 高 / high |
| RS-21 | 无 checkpoint 时不比较父哈希，但第一块前每个 slot 必须是 skipped | `design.md` §3.2；spec AC-089 | 2026-11-22 | fact | 高 |
| RS-22 | 停扫后退到最后区块 slot，或 `startSlot-1` | `design.md` §3.2；spec AC-092 | 2026-11-22 | fact | 高 |
| RS-23 | 当前 scanner 直接写库，且空 checkpoint 直接判连续成功 | `internal/scanner/scanner.go`、`continuity.go` | 2026-11-22 | observed | 高 |
| RS-24 | 七表 SQL 已在 `000001`；没有 `sol_scan_cursor` | `scripts/postgre/000001_initial_schema.up.sql` | 2026-11-22 | observed | 高 |
| RS-25 | provider 与 parser 的目标语义已有实现和测试，路径仍是旧目录 | `internal/rpc/provider`、`internal/parser` | 2026-11-22 | observed | 高 |
| RS-26 | `chain-api-go v0.1.0` 已进入 `go.sum`，模块要求无 `replace` | `go.mod`、`go.sum` | 2026-11-22 | observed | 高 |
| RS-27 | 示例配置仍是旧扫描键 | `config/config.example.yaml` | 2026-11-22 | observed | 高 |
| RS-28 | 夹具 tx158 的消息 index 为 `"2"` | spec AC-026、AC-056；design §5.3 | 2026-11-22 | fact | 高 |
| RS-29 | Kafka 批量、key、Flush 毫秒 1000 是 design §5.2 的三处修改 | `design.md` §5.2 | 2026-11-22 | fact | 高 |
| RS-30 | E2E Compose 目前只有 PostgreSQL 14.14 | `scripts/test_docker_compose.yml`、`Makefile` | 2026-11-22 | observed | 高 |
| RS-13 | `Flush` 参数单位是毫秒；库需要 cgo | v1.5 对 confluent v2.11.0 的记录 | 2026-11-21 | verified，实施时核对 import | 高 |
| RS-18 | `apache/kafka:3.9.0` 单节点 KRaft 是选定测试形态 | v1.5 plan 的 Compose 片段 | 2026-11-21 | inference；PLAN-018 证实 | 中 / medium |

## 9. 方案对比 / Solution Comparison

| 议题 / Topic | 方案 / Option | 复杂度 | 性能 | 扩展 | 兼容 | 风险 | 维护 | 处置 / Disposition |
|---|---|---|---|---|---|---|---|---|
| 进程 | 两进程加区块 topic | 中 | 扫块与落库分离 | 中 | 符合 design | 位点回退复杂 | 中 | 选定 / selected |
| 进程 | 保留单进程 scanner | 低 | 少一次队列 | 低 | 与 design 冲突 | 高 | 低 | 拒绝 / rejected |
| 新表 | `000002` | 低 | — | 高 | 不改已应用 SQL | 低 | 低 | 选定 |
| 新表 | 修改 `000001` | 低 | — | 低 | 破坏迁移身份 | 高 | 低 | 拒绝 |
| 包路径 | 按 design 搬迁后删旧目录 | 低 | — | 高 | 导入一次性变化 | 低 | 低 | 选定 |
| 包路径 | 旧路径加别名 | 低 | — | 低 | 两套入口 | 中 | 高 | 拒绝 |
| 区块载荷 | `getBlock` result 的 JSON | 中 | 中 | 中 | 可复用 `ParseBlock` | 往返失败则停 | 低 | 选定 |
| 区块载荷 | 保存节点原始 HTTP 体 | 低 | 差 | 低 | 违反“原始 JSON 不进库”，消息也会绑住信封 | 中 | 中 | 拒绝 |
| 写库并发 | 一个写协程 | 低 | 足够本期 | 低 | 符合 design | 低 | 低 | 选定 |
| 写库并发 | 多个写实例 | 高 | 高 | 高 | 连续性无法保持 | 高 | 高 | 拒绝 |
| Kafka 客户端 | `confluent-kafka-go/v2 v2.11.0` | 中 | 高 | 中 | design 已指定 | cgo | 中 | 选定 |
| Kafka 客户端 | 纯 Go 客户端 | 中 | 中 | 中 | 未经 design 批准 | 中 | 中 | 拒绝 |

过度设计控制 / Over-design controls：不把超时、退避和批量大小做成配置。不建 provider 多节点。不建通用 publisher 框架。不让 GORM 自动迁移。

## 10. 可行性验证 / Feasibility Validation

| ID | 假设 / Hypothesis | 方法与阈值 / Method and threshold | 预算与清理 / Budget and cleanup | 决策影响 / Decision impact | 证据 / Evidence |
|---|---|---|---|---|---|
| FV-04 | 夹具区块的 `GetBlockResult` 经 `encoding/json` 往返后，`ParseBlock` 与直接解析一致 | PLAN-019 离线测试使用现有 gzip 夹具。字段、交易数、第 158 笔 transfer 必须相同 | 无额外主网请求 | 不一致则停止，返回 Planner，不改解析口径 | PLAN-019 |
| FV-05 | v1.5 写明的 Kafka 3.9.0 Compose 能在 `make e2e` 中变为 healthy | 启动专用项目，退出时 `down -v` | 容器和 volume 随测试清理 | 不健康按 F4 改官方等价环境变量，端口保持 `19092` | PLAN-018 |
| FV-06 | 现有 provider 测试搬到新包后不需改断言 | `go test` 新包，旧包目录不存在 | 无 | 断言必须改才通过时，停止并核对是否改变了 RPC 语义 | PLAN-016 |

不需要单独的生产代码试验。FV-04 失败不是让 Coder 改消息格式的授权。

## 11. 开发协作文档 / Development Collaboration Documents

接口契约在 spec FR-02、FR-09 至 FR-17 和 design §5.2、§5.3。跨团队约定：下游只消费 `kafka.transactionTopic`，按签名去重；Token-2022 入账用 `change`。区块 topic 是内部队列，不是下游契约。没有单独配套文件。`approved_companion_files` 为空。

## 12. 共享 Task Contract / Shared Task Contract

```yaml
runtime_routing:
  runtime: cursor
  profile_source: references/runtime-routing.zh-CN.md
  parent_selection: auto
  human_override: "三角色 Auto/inherit；不使用路由表中的 claude-opus-5-5[effort=high]，也不改用 claude-opus-5-5-medium"
  planner: {policy: inherit, access: 只读}
  coder: {policy: inherit_verified_auto, access: 仅授权路径}
  auditor: {policy: inherit, access: 只读加隔离审计测试路径}
  silent_substitution_forbidden: true
planning_baseline:
  version: PLAN-v1.6-SOL-SCANNER-v1
  date: 2026-11-22
  status: 已批准 / Approved
  all_plans_approved: true
  document_root: docs/superpowers/001-sol-scanner-v1
time_budget:
  planning_design_documentation_percent: 40
  coding_percent: 5
  verification_audit_acceptance_percent: 55
verification_layers:
  per_plan: "离线单测；证明该 Plan 接口所必需的确定性集成。真实 PG/Kafka 只在 internal/test/integration 且 //go:build e2e"
  final_acceptance: "A-01..A-04 与 AC-073..AC-076、AC-097，全部新 Plan 集成后执行一次"
failure_routing:
  F1_F4: Coder
  F5_F6: Planner，Human 裁决约束性歧义
```

公共不变量是 spec INV-001 至 INV-013。公共停止条件：规格冲突、要扩大路径、FV-04 失败、需要新依赖、要把 A-01–A-04 塞进子 Plan。公共回滚：回退该 Plan 自己的提交；不回改 `000001`。集成阶段不跑测试，只做 Git 结构检查。

历史编号 T001–T043 属于已落地工作，不重编、不重做。T044–T072 属于被替代的 PLAN-008–PLAN-013，不执行。新工作从 T073 起，完整字段留在 `tasks.md`。

## 13. 详细功能 Plan / Detailed Functional Plans

### PLAN-001–PLAN-007 已落地 / Landed, do not re-execute

| Plan | 已落地能力 / Landed capability | 仍有效验收 / Acceptance | 处置 / Disposition |
|---|---|---|---|
| PLAN-001 | `chain-api-go v0.1.0` 已在 `go.mod` | AC-001–AC-005 | 不重发、不重测 C-05 |
| PLAN-002 | 严格加载、环境变量、迁移加载、脱敏错误 | AC-006–AC-009 的机制；键集合改由 PLAN-014 满足 | 不重写加载器骨架 |
| PLAN-003 | `000001` 七表、migrate CLI、schema gate 版本 1、`make e2e` 的 PostgreSQL | AC-010–AC-012 的版本 1 部分；AC-013 的版本 2 归 PLAN-015 | 不改 `000001` |
| PLAN-004 | provider 语义和测试 | AC-014–AC-019 | 路径归 PLAN-016 |
| PLAN-005 | 领域模型、区块头和交易解析、夹具 | AC-020–AC-025 | 路径归 PLAN-017 |
| PLAN-006 | 转账、Token-2022、`index` `"2"` | AC-026–AC-033 | 路径归 PLAN-017 |
| PLAN-007 | `ScanRepository` 一块一事务、halt | AC-034–AC-037 | 保留；游标和发布接口是新 Plan |

这些 Plan 的生产增量不再计入 v1.6 预算。

### PLAN-008–PLAN-013 被 v1.6 替代 / Superseded, do not execute

| 旧 Plan / Old | 旧任务 / Old tasks | 替代者 / Replacement | 原因 / Reason |
|---|---|---|---|
| PLAN-008 单进程扫描 | T044–T049 | PLAN-021、PLAN-022 | 现有循环写库，且空 checkpoint 规则与 AC-089 相反 |
| PLAN-009 mqx | T050–T053 | PLAN-018 | 能力仍需要，但按新依赖顺序重开，不沿用旧任务 |
| PLAN-010 交易消息 | T054–T055 | PLAN-023 | 同上 |
| PLAN-011 发布 | T056–T061 | PLAN-024 | 发布循环在处理进程，不在单进程 scanner |
| PLAN-012 单进程装配 | T062–T068 | PLAN-025、PLAN-026 | 两个命令和两种启动门禁 |
| PLAN-013 总体验收 | T069–T072 | PLAN-028 | A-01–A-04 改为两个进程一起验收 |

不得把 T044–T072 改成新实现，也不得勾选它们。

### PLAN-014 配置契约改为双进程 / Retarget configuration

- 背景 / Background：加载机制已落地，目标键仍是旧扫描窗口。
- 目标 / Goal：示例文件和加载器使用 `stepSlots`、`slotsPerWorker`、`blockTopic`；处理进程不展开 RPC。
- 范围 / Scope：`LoadForScanner`、`LoadForProcessor`、校验、示例 YAML。保留 `LoadForMigration`。
- 非目标 / Non-goals：不连接网络；不保留旧键别名；不改迁移 CLI。
- 接口 / Interfaces：spec FR-02。处理进程 raw 结构中 `rpc` 与扫块相同，固定只认 `httpURL`，但不展开、不复制进 `ProcessorConfig`。扫块进程完整校验 `rpc.httpURL` 与 `blockTopic`。`transactionTopic` 对扫块进程可空，非空则校验字符集；处理进程要求非空。
- 策略 / Strategy：先改 `load_test.go` 与 `validate_test.go`，使旧键失败、AC-078 和 AC-079 失败；再改 types、load、validate 和示例文件。删除对外的旧 `LoadForService` 入口，调用点还没有生产进程。
- allowed_paths：`internal/config/**`、`config/config.example.yaml`。
- 依赖 / Dependencies：PLAN-002 已落地。无新 Plan 前置。
- 验收 / Acceptance：AC-006、AC-007、AC-008、AC-009、AC-078、AC-079。
- per-plan 命令 / Command：`go test ./internal/config/...`；`go vet ./internal/config/...`。
- final acceptance 贡献 / Contribution：提供两个进程可加载的配置。不跑主网。
- 风险 / Risks：共享示例若被处理进程按未知字段拒绝。缓解：两个加载器都认识全部目标键，差别只在是否展开和是否要求 RPC。
- 回滚 / Rollback：恢复 config 包和示例文件到本 Plan 前的树。
- 预算 / Budget：约 4 个文件；生产约 180 行；测试约 250 行。总投入 3.8h，规划 1.52h / 编码 0.19h / 验证 2.09h。任务 T073–T077。未超 400/1200 预警。
- 预期 / Result：`go test ./internal/config/...` 通过，示例文件不再含旧键。

### PLAN-015 投递游标迁移 / Handoff-cursor migration

- 背景 / Background：七表已在版本 1。投递位置还没有表。
- 目标 / Goal：新增 `000002_scan_cursor`，gate 认识 `(2, scan_cursor)`。
- 范围 / Scope：成对 SQL、`requiredSchemaMigrations`、发现测试和迁移集成测试。
- 非目标 / Non-goals：不改 `000001` 字节；不写仓储；不在服务启动时迁移。
- 接口 / Interfaces：Up 只创建 design §4 的 `sol_scan_cursor`。Down 只 `DROP TABLE sol_scan_cursor`。`CheckSchemaVersion(ctx, db, 2)` 要求版本 1 和 2 的版本号与名称都匹配。
- 策略 / Strategy：先让发现测试和 `TestMigrationsUpDownStatus` / `TestSchemaGate` 因缺少 000002 失败；再写 SQL；最后把 gate 切片追加一项。现有“恰好一对迁移”的断言改为“000001 不变且新增恰好一对 000002”。
- allowed_paths：`scripts/postgre/000002_scan_cursor.up.sql`、`scripts/postgre/000002_scan_cursor.down.sql`、`internal/pkg/postgre/database.go`、`internal/pkg/postgre/migrations_test.go`、`internal/test/integration/migrations_test.go`。
- 依赖 / Dependencies：PLAN-003 已落地。
- 验收 / Acceptance：AC-010、AC-011 保持；AC-012、AC-013、AC-080。
- per-plan 命令 / Command：`go test ./internal/pkg/postgre/...`；`make e2e E2E_RUN='^(TestMigrationsUpDownStatus|TestSchemaGate)$'`。
- final acceptance 贡献 / Contribution：两个进程启动前的 schema 身份。不跑主网。
- 风险 / Risks：Down 误删七张旧表。缓解：Down SQL 只点名 `sol_scan_cursor`，集成测试先建七表再执行 000002 Down。
- 回滚 / Rollback：对测试库执行 000002 Down，删除新 SQL 和 gate 项。已应用生产库不在本 Plan 操作。
- 预算 / Budget：约 5 个文件；生产 SQL 与 gate 约 40 行；测试约 180 行。总投入 2.8h，规划 1.12h / 编码 0.14h / 验证 1.54h。任务 T078–T081。
- 预期 / Result：版本 2 升级和回退只增减一张表。

### PLAN-016 provider 迁址 / Move provider

- 背景 / Background：语义已落地，目录仍是 `internal/rpc/provider`。
- 目标 / Goal：包位于 `internal/scanner/provider`，旧目录删除，断言不变。
- 范围 / Scope：移动 `reader.go`、`errors.go` 及两个测试；更新现有 scanner 的导入，使树在 PLAN-021 替换前仍能编译。
- 非目标 / Non-goals：不改重试次数、分类、getBlock 参数；不改 scanner 行为。
- 接口 / Interfaces：spec FR-04 的 `Reader`。`NewReader(endpoint string) (Reader, error)` 保持。
- 策略 / Strategy：先把测试文件放到新路径，确认旧导入路径编译失败；再移动实现并改 `internal/scanner/scanner.go` 与 `scanner_test.go` 的 import。删除空的 `internal/rpc/provider`。
- allowed_paths：`internal/scanner/provider/**`、`internal/rpc/provider/**`、`internal/scanner/scanner.go`、`internal/scanner/scanner_test.go`。
- 依赖 / Dependencies：PLAN-004 已落地。与 PLAN-017 都要改 `scanner.go`，因此 PLAN-017 排在本 Plan 之后。
- 验收 / Acceptance：AC-014–AC-019、AC-081。
- per-plan 命令 / Command：`go test ./internal/scanner/provider/...`；`go test ./internal/scanner/...`；`go build ./...`。
- final acceptance 贡献 / Contribution：扫块进程的链访问位置。不跑主网。
- 风险 / Risks：搬迁时顺手改变超时或错误文本。缓解：测试断言保持原样，FV-06 失败即停止。
- 回滚 / Rollback：把文件移回 `internal/rpc/provider` 并恢复导入。
- 预算 / Budget：移动约 4 个 Go 文件；净新增生产约 0 行；测试约 0 行。总投入 1.4h，规划 0.56h / 编码 0.07h / 验证 0.77h。任务 T082–T083。
- 预期 / Result：旧包路径不存在，provider 测试通过。

### PLAN-017 parser 迁址 / Move parser

- 背景 / Background：解析口径已落地，目录仍是 `internal/parser`。
- 目标 / Goal：包位于 `internal/processor/parser`，夹具和 jsonparsed 路径跟着走，旧目录删除。
- 范围 / Scope：移动解析源码、测试和 `testdata`；更新 `internal/test/jsonparsed` 的夹具路径；更新 scanner 导入。
- 非目标 / Non-goals：不改 P-01–P-10 的期望值；不改 kind 编码。
- 接口 / Interfaces：`ParseBlock(slot uint64, blk *rpc.GetBlockResult) (domain.BlockBatch, error)`，`ErrDecode` 保持。
- 策略 / Strategy：先移动测试和夹具，看到新包缺少实现而失败；再移动实现。`jsonparsed.FixturePath` 改为 `internal/processor/parser/testdata`。
- allowed_paths：`internal/processor/parser/**`、`internal/parser/**`、`internal/test/jsonparsed/**`、`internal/scanner/scanner.go`。
- 依赖 / Dependencies：PLAN-016，避免同时改 `scanner.go`。
- 验收 / Acceptance：AC-020–AC-033、AC-082。
- per-plan 命令 / Command：`go test ./internal/processor/parser/... ./internal/test/jsonparsed/... ./internal/domain/...`；`go build ./...`。
- final acceptance 贡献 / Contribution：处理进程的解析位置。A-02 仍延后。
- 风险 / Risks：夹具路径漏改导致测试读到空文件。缓解：AC-020 仍断言交易数 457。
- 回滚 / Rollback：移回 `internal/parser` 并恢复夹具路径。
- 预算 / Budget：移动现有文件；净新增生产约 0 行。总投入 1.8h，规划 0.72h / 编码 0.09h / 验证 0.99h。任务 T084–T085。
- 预期 / Result：旧解析包不存在，原解析测试在新路径通过。

### PLAN-018 Kafka 批量生产者 / Kafka batch producer

- 背景 / Background：下游和区块队列都需要带 key 的批量回执。仓库还没有 mqx，Compose 也没有 Kafka。
- 目标 / Goal：`ProduceBatch` 满足 AC-052–AC-055，Flush 使用 1000。
- 范围 / Scope：`internal/pkg/mqx`、`go.mod` 中的 confluent 依赖、Compose 的 Kafka 服务、Makefile 的 `SOL_SCANNER_E2E_KAFKA=127.0.0.1:19092`。
- 非目标 / Non-goals：不定义区块或交易消息；不创建业务 topic 名称常量以外的队列逻辑。
- 接口 / Interfaces：spec FR-14。`Keyed[T]` 用值。`Producer[T]` 由持有连接的对象实现，允许指针。
- 策略 / Strategy：先按 v1.5 已写明的环境变量追加 Kafka，并写离线失败测试和 `//go:build e2e` 测试；再实现生产者。`message.timeout.ms=1000`。不可达 broker 的测试不依赖容器。真实投递测试只在 integration。
- allowed_paths：`internal/pkg/mqx/**`、`internal/test/integration/mqx_test.go`、`internal/test/integration/helpers_test.go`、`scripts/test_docker_compose.yml`、`Makefile`、`go.mod`、`go.sum`。
- 依赖 / Dependencies：无功能前置。Compose 变更不得删除现有 PostgreSQL 服务。
- 验收 / Acceptance：AC-052、AC-053、AC-054、AC-055。
- per-plan 命令 / Command：`CGO_ENABLED=1 go test ./internal/pkg/mqx/...`；`make e2e E2E_RUN='^TestMqx'`。
- final acceptance 贡献 / Contribution：两个 topic 的传输能力。不在本 Plan 发送主网交易。
- 风险 / Risks：镜像配置不健康。缓解：FV-05，按 F4 调整，端口和清理方式不变。cgo 缺失时不得改成另一个客户端。
- 回滚 / Rollback：删除 mqx 包、Kafka 服务和该依赖。PostgreSQL E2E 保持可运行。
- 预算 / Budget：约 6 个文件；生产约 250 行；测试约 400 行。总投入 4.0h，规划 1.60h / 编码 0.20h / 验证 2.20h。任务 T086–T089。未超预警。
- 预期 / Result：离线取消测试和一次真实 Kafka 批量测试通过。

### PLAN-019 区块队列消息 / Block-queue messages

- 背景 / Background：扫块和处理之间还没有内部消息。
- 目标 / Goal：一条消息一个 slot，key 为十进制 slot；JSON 往返后能交给现有 `ParseBlock`。
- 范围 / Scope：`internal/event/block` 的类型、编码、生产者、消费者。消费者关闭自动提交，并提供 Commit 与 Seek。
- 非目标 / Non-goals：不决定何时提交位点；不解析转账；不把 JSON 写入数据库。
- 接口 / Interfaces：spec FR-11 与 design §5.3 `BlockMessage`。`Encode(msg BlockMessage) ([]byte, error)`、`Decode([]byte) (BlockMessage, error)`、`Key(slot uint64) string`。生产者接收 `[]BlockMessage` 并调用 mqx。消费者返回消息和该消息位点；`Commit(offset)` 与 `Seek(offset)` 分开。
- 策略 / Strategy：先用夹具写往返失败测试，覆盖 skipped 无 `block`、block 有 result JSON、key `102`。往返断言调用 `parser.ParseBlock`。通过后再实现生产者和消费者的离线假 mqx 测试。一个最小 e2e 只证明一条 block 和一条 skipped 能经真实 Kafka 读回，不跑扫块循环。
- allowed_paths：`internal/event/block/**`、`internal/test/integration/block_queue_test.go`。
- 依赖 / Dependencies：PLAN-017、PLAN-018。
- 验收 / Acceptance：AC-083。生产失败不推进游标由 PLAN-021 的 AC-095 使用本接口证明。
- per-plan 命令 / Command：`go test ./internal/event/block/...`；`make e2e E2E_RUN='^TestBlockQueueRoundTrip$'`。
- final acceptance 贡献 / Contribution：A-01 所需的 slot 消息形状。本 Plan 不执行 A-01。
- 风险 / Risks：FV-04 失败。缓解：停止并返回 Planner。不得改用原始 HTTP 体蒙混通过。
- 回滚 / Rollback：删除 `internal/event/block` 和对应 e2e 文件。
- 预算 / Budget：约 6 个文件；生产约 320 行；测试约 500 行。总投入 5.4h，规划 2.16h / 编码 0.27h / 验证 2.97h。任务 T090–T095。接近预警，仍是一个队列能力，不按行数拆开。
- 预期 / Result：夹具往返测试通过；e2e 能读回两种 kind。

### PLAN-020 投递游标仓储 / Handoff-cursor repository

- 背景 / Background：表将在 PLAN-015 存在，还没有领域接口。
- 目标 / Goal：条件前进和停扫回退都经过 `ScanCursorRepository`。
- 范围 / Scope：接口、DAO、entity、构造函数、e2e。
- 非目标 / Non-goals：不实现扫块循环；不修改 `ScanRepository` 的区块事务。
- 接口 / Interfaces：`GetHandedSlot(ctx) (uint64, bool, error)`；`AdvanceHandedSlot(ctx, prev, next uint64) error`；`RewindHandedSlot(ctx, slot uint64) error`。没有行时 `ok=false`。前进使用 `WHERE id=1 AND handed_slot=prev`；`prev=startSlot-1` 且没有行时插入。回退是显式 upsert 到给定 slot。冲突返回明确错误，不把行改掉。
- 策略 / Strategy：先写 e2e：无行、插入、条件前进、错误 prev、回退到 101、回退到 499。测试失败后再写 `internal/repository/scan_cursor.go` 与 `internal/repository/postgre` 的 entity/DAO。DAO 使用显式 SQL 或 GORM 条件更新，SQL 文本仍只描述表内操作，不把新 DDL 放进 Go。
- allowed_paths：`internal/repository/scan_cursor.go`、`internal/repository/postgre/scan_cursor.go`、`internal/repository/postgre/entity.go`、`internal/test/integration/scan_cursor_repository_test.go`。
- 依赖 / Dependencies：PLAN-015。
- 验收 / Acceptance：AC-088。AC-092 的回退值由 PLAN-022 调用本接口证明。
- per-plan 命令 / Command：`make e2e E2E_RUN='^TestScanCursorRepository$'`；`go test ./internal/repository/...`。
- final acceptance 贡献 / Contribution：A-01 的投递位置。不跑主网。
- 风险 / Risks：回退被写成只允许向前。缓解：测试覆盖 `handed_slot` 从 21000 改到 101。
- 回滚 / Rollback：删除新 Go 文件。表留在 000002，由 PLAN-015 的 Down 负责。
- 预算 / Budget：约 4 个文件；生产约 180 行；测试约 250 行。总投入 3.0h，规划 1.20h / 编码 0.15h / 验证 1.65h。任务 T096–T099。
- 预期 / Result：条件冲突不改行；回退可写到更小 slot。

### PLAN-021 扫块循环 / Scanner handoff loop

- 背景 / Background：现有 `internal/scanner` 直接解析并写库，不能当作目标循环。
- 目标 / Goal：替换为 spec FR-10 的分段投递，并完成扫块侧启动检查。
- 范围 / Scope：纯分段函数、每段一次 `BlockSlots`、block/skipped 投递、整步成功后 `AdvanceHandedSlot`、创世哈希、`startSlot`、最早区块、未处理停扫等待。删除旧 `continuity.go` 和旧循环测试。
- 非目标 / Non-goals：不调用 `ParseBlock`；不调用 `SaveBlockBatch`；不发交易 topic；不实现处理侧连续性。
- 接口 / Interfaces：`Segments(left, right, slotsPerWorker uint64) [][2]uint64`，闭区间，末段可短。`Run(ctx) error` 只依赖 `provider.Reader`、区块生产者、`ScanCursorRepository`、`HasUnresolvedHalt`；`Alerter` 不由扫块循环发送。扫块循环看到未处理停扫只等待。致命错误（`Run` 返回、不写 halt）：`UNSUPPORTED_VERSION`、`repository.ErrHandedSlotConflict`、以及 `GetHandedSlot` / `AdvanceHandedSlot` / `HasUnresolvedHalt` 的数据库错误。可重试（记录后等待 `pollInterval` 重做整步）：非 `UNSUPPORTED_VERSION` 的 provider/RPC 失败、区块队列 `Produce` 失败。v1.6.1 起，Run 路径致命分类的实现与聚焦测试授权在 PLAN-025（T128–T129），不单独重开本 Plan。
- 策略 / Strategy：用假 Reader、假生产者和假游标先写 S-01、S-02、S-03、S-07、S-07b、S-08、S-10–S-13、S-15、S-16 的失败测试，同时删除旧行为断言。再实现分段、worker 和循环。worker 失败取消同一步的其他 worker。已发出的消息允许重复。任一段失败不调用 `AdvanceHandedSlot`。
- allowed_paths：`internal/scanner/*.go`，不含 `internal/scanner/provider/**`。
- 依赖 / Dependencies：PLAN-014、PLAN-016、PLAN-019、PLAN-020。
- 验收 / Acceptance：AC-064、AC-065、AC-066、AC-067 的扫块半边、AC-084、AC-085、AC-086、AC-087、AC-094 的扫块半边、AC-095。
- per-plan 命令 / Command：`go test ./internal/scanner/...`；`go test -race ./internal/scanner/...`。
- final acceptance 贡献 / Contribution：A-01 的扫块侧规则。本 Plan 使用假 RPC，不访问主网。
- 风险 / Risks：生产约 480 行，超过约 400 行预警。这是一个不可再拆的投递能力：分段、失败原子性和启动门禁共用同一循环状态。保持完整，请 Human 在批准本基线时一并接受该预算。
- 回滚 / Rollback：恢复 PLAN-021 之前的 `internal/scanner` 文件。provider 迁址不回滚。
- 预算 / Budget：约 6 个文件；生产约 480 行；测试约 900 行。总投入 6.5h，规划 2.60h / 编码 0.33h / 验证 3.57h。任务 T100–T105。测试未超 1200 行预警。
- 预期 / Result：旧连续性函数不存在；新扫块测试通过；包测试不导入 parser 或 `SaveBlockBatch`。

### PLAN-022 处理写协程 / Processor apply loop

- 背景 / Background：连续性必须在落库前完成，且与旧 `checkContinuity` 不同。
- 目标 / Goal：单协程按 slot 应用 block 与 skipped，按 spec 停扫并回退游标和位点。
- 范围 / Scope：缓冲、10 次空 poll 缺口、五种停扫、skipped、无 checkpoint 前缀、重复已覆盖区块（丢弃+Commit）、halt 顺序、`resumeOffset < 0` 不 Seek、告警失败仍保留 halt。
- 非目标 / Non-goals：不访问 RPC；不发送下游交易；不实现进程 main。
- 接口 / Interfaces：`Apply(ctx) error` 依赖区块消费者、`parser.ParseBlock`、`ScanRepository`、`ScanCursorRepository`、`Alerter`、`startSlot`、`pollInterval`。内存保存 `applySlot` 和 `resumeOffset`。成功提交真实区块后更新 `resumeOffset`。skipped 成功或丢弃已覆盖后提交该消息位点。停扫时 `RewindHandedSlot`；若 `resumeOffset >= 0` 则 Seek 再 Commit，若 `< 0` 则只 Commit。
- 策略 / Strategy：假队列、假仓储、假告警上先写 AC-089、AC-090、AC-091、AC-092、AC-093 的失败测试（含 ≥10 条更晚 slot 再齐前缀、丢弃须 Commit、`resumeOffset=-1` 不 Seek）。断言顺序用告警替身记录调用次数，证明 halt 行先写。然后实现一个协程里的 poll 和 apply。空 poll 缺口轮使用注入的时钟，不睡眠真实 10 个 pollInterval。
- allowed_paths：`internal/processor/*.go`，不含 `internal/processor/parser/**`。
- 依赖 / Dependencies：PLAN-007 已落地、PLAN-014、PLAN-017、PLAN-019、PLAN-020。可与 PLAN-021 在双方依赖满足后并行，但不得同时改对方目录。
- 验收 / Acceptance：AC-067 的处理半边、AC-089、AC-090、AC-091、AC-092、AC-093、AC-094 的处理半边。
- per-plan 命令 / Command：`go test ./internal/processor/...`；`go test -race ./internal/processor/...`。
- final acceptance 贡献 / Contribution：A-01 的父子衔接规则。不跑主网。
- 风险 / Risks：生产约 420 行，超过约 400 行预警。写协程、停扫和位点回退是同一个状态机，拆开会做出无法独立验收的半套连续性。保持完整，请 Human 批准该预算。
- 回滚 / Rollback：删除 `internal/processor` 根目录 Go 文件，保留已迁入的 parser。
- 预算 / Budget：约 5 个文件；生产约 420 行；测试约 700 行。总投入 4.2h，规划 1.68h / 编码 0.21h / 验证 2.31h。任务 T106–T108。
- 预期 / Result：假队列测试覆盖 S-04、S-04b、S-05、S-06、S-09、S-09b、S-14、S-17、S-18、S-19、SX-01。

### PLAN-023 下游交易消息 / Downstream transaction message

- 背景 / Background：下游字段已在 design 冻结，仓库还没有事件包。
- 目标 / Goal：`FromBlockBatch` 只从领域行生成与 design §5.3 一致的 JSON。
- 范围 / Scope：`internal/event/transaction/types.go`、测试和 golden。golden 来自 spec 已引用的 design 示例，比较前去掉排版空白。
- 非目标 / Non-goals：不读数据库；不发 Kafka；不重新解析链上数据。
- 接口 / Interfaces：spec FR-15。`FromBlockBatch(chain string, batch domain.BlockBatch) []TransactionMessage`。金额用字符串。失败交易的数组是空切片，序列化为 `[]`。
- 策略 / Strategy：先放 golden 和失败测试，覆盖第 158 笔、失败交易、内层 index、省略 fee、decimals null；再写类型和映射。测试构造 `domain.BlockBatch`，不启动 parser 的网络路径。
- allowed_paths：`internal/event/transaction/types.go`、`internal/event/transaction/types_test.go`、`internal/event/transaction/testdata/k05_message.json`。
- 依赖 / Dependencies：领域模型已落地。不依赖 parser 迁址后的行为变化。为避免和 PLAN-024 同时创建目录，本 Plan 先于 PLAN-024。
- 验收 / Acceptance：AC-056、AC-057。
- per-plan 命令 / Command：`go test ./internal/event/transaction/ -run 'TestTransactionMessage|TestFailedTransactionJSON'`。
- final acceptance 贡献 / Contribution：A-03 的消息形状。不消费真实 topic。
- 风险 / Risks：空白比较不稳定。缓解：解码两边 JSON 再按规范编码后比较字节。
- 回滚 / Rollback：删除本 Plan 新增的三个文件。
- 预算 / Budget：3 个文件；生产约 220 行；测试约 250 行。总投入 1.8h，规划 0.72h / 编码 0.09h / 验证 0.99h。任务 T109–T110。
- 预期 / Result：golden 中 `index` 为 `"2"`，测试通过。

### PLAN-024 发布仓储与发布循环 / Publish repository and loop

- 背景 / Background：`sol_publish_cursor` 已在 `000001`，没有 DAO，也没有发送循环。
- 目标 / Goal：按 slot 读已提交区块，整块回执成功后才前进发布游标。
- 范围 / Scope：`PublishRepository`、postgre DAO、`event/transaction` 的 producer、离线退避测试、一个 Kafka e2e。
- 非目标 / Non-goals：不重解析区块队列；不在扫块进程启动；不做人工重发入口。
- 接口 / Interfaces：spec FR-16。`GetPublishCursor`、`ListBlockBatches(after, limit)`、`AdvancePublishCursor`。无游标时从最小 slot 开始，成功后插入。topic 不匹配返回错误且不发送。退避 1s、2s、4s，上限 30s，时钟可注入。
- 策略 / Strategy：先写仓储 e2e 和生产者单测，使缺少接口而失败；再写 DAO 和循环。K-01、K-02、K-04 的真实 Kafka 测试放在 integration。K-03 的“checkpoint 仍前进”需要写协程同时运行，留到 PLAN-026 的进程测试，不在这里启动 scanner。
- allowed_paths：`internal/repository/publish.go`、`internal/repository/postgre/publish.go`、`internal/event/transaction/producer.go`、`internal/event/transaction/producer_test.go`、`internal/test/integration/publish_repository_test.go`、`internal/test/integration/transaction_producer_kafka_test.go`。
- 依赖 / Dependencies：PLAN-007、PLAN-018、PLAN-023。
- 验收 / Acceptance：AC-058、AC-059、AC-060、AC-061、AC-062、AC-063。
- per-plan 命令 / Command：`go test ./internal/event/transaction/ -run TestPublish`；`make e2e E2E_RUN='^(TestPublishRepository|TestTransactionProducerKafka)$'`。
- final acceptance 贡献 / Contribution：A-03 的发送路径。本 Plan 用构造好的库行，不扫主网。
- 风险 / Risks：`ListBlockBatches` 把同一个 slot 拆成两次发送。缓解：limit 以区块为单位，一个区块的子行一次取全。
- 回滚 / Rollback：删除新 DAO、接口和 producer 文件。`000001` 的表保留。
- 预算 / Budget：约 6 个文件；生产约 350 行；测试约 550 行。总投入 5.6h，规划 2.24h / 编码 0.28h / 验证 3.08h。任务 T111–T115。
- 预期 / Result：失败重发字节相同；空区块只前进游标。

### PLAN-025 扫块进程装配 / Scanner process assembly

- 背景 / Background：循环已能在库里测试，还没有进程入口。v1.6.1：Auditor A-025-01 证明 AC-069 的游标冲突/Run 路径数据库错误在 `Run` 中被误重试，且原 `allowed_paths` 无法改 `loop.go`。
- 目标 / Goal：`sol-scanner --config` 完成扫块启动顺序、退出码和信号；并使 `Scanner.Run` 对 AC-069 致命错误返回，从而进程退出码 1。
- 范围 / Scope：`cmd/sol-scanner`、扫块用 `InitX`、main 测试；以及 v1.6.1 窄改 `internal/scanner/loop.go`（致命/可重试分类）与对应聚焦测试。
- 非目标 / Non-goals：不装配 parser、交易 producer 或写协程；不改分段算法、worker 投递或 preflight 规则；不为 PERMANENT RPC 新造退出语义；不改处理进程。
- 接口 / Interfaces：spec FR-17 的扫块半边。`run(args []string) int` 可在测试中调用。缺配置或多余参数返回 2。创世哈希、空 `startSlot`、最早区块、数据库（含 Init 与 Run 路径）、游标冲突和 `UNSUPPORTED_VERSION` 返回 1。未处理停扫时 `run` 在取消前保持运行。信号返回 0。`Run` 对非 `UNSUPPORTED_VERSION` 的 provider 失败与 `Produce` 失败保持重试。
- 策略 / Strategy：先写 `main_test.go`，用假依赖或 ioc 替换点证明退出码；生产 `main` 只解析参数、装真依赖、监听信号。v1.6.1：先写 `Run` 在 `ErrHandedSlotConflict` / 游标 DB 错误下返回的失败测试，再在 `loop.go` 做最小分类（可用包内 sentinel 包裹游标仓储错误，以区别于 `Produce` 失败）。测试不访问主网。
- allowed_paths：`cmd/sol-scanner/**`、`internal/ioc/scanner.go`、`internal/ioc/db.go`、`internal/ioc/provider.go`、`internal/ioc/block_event.go`、`internal/scanner/loop.go`、`internal/scanner/loop_fatal_test.go`（或等价地只改 `internal/scanner/scanner_test.go` 中与 Run 致命分类相关的用例；不得改 `provider/**`、`segment.go`、`worker.go`、`preflight.go`，除非 Human 另批）。
- 依赖 / Dependencies：PLAN-014、PLAN-015、PLAN-016、PLAN-020、PLAN-021。v1.6.1 补丁任务 T128–T129 依赖已落地的 T118。
- 验收 / Acceptance：AC-064、AC-065、AC-066、AC-067、AC-069、AC-070、AC-071、AC-096 的扫块命令；v1.6.1 起 AC-069 必须覆盖 Run 路径游标冲突与游标 DB 错误（不仅 Init 数据库错误与 `UNSUPPORTED_VERSION`）。
- per-plan 命令 / Command：`go test ./cmd/sol-scanner/... ./internal/ioc/...`；`go test ./internal/scanner/ -run 'TestRunFatal|TestStepUnsupported'`；`go test -race ./internal/scanner/...`；`go build -o /dev/null ./cmd/sol-scanner`。
- final acceptance 贡献 / Contribution：A-01 的扫块进程入口。不在本 Plan 启动主网。
- 风险 / Risks：main 测试误连真实 RPC。缓解：测试只走假 Reader；生产 Init 函数在单测中不调用。误把 `Produce` 失败当成致命。缓解：分类测试断言 Produce/RPC 仍重试。
- 回滚 / Rollback：删除 `cmd/sol-scanner` 和只被它使用的 ioc 文件；将 `loop.go` 的 Run 分类回退到仅 `UNSUPPORTED_VERSION` 致命（不推荐，除非整基线回退）。
- 预算 / Budget：约 8 个文件；生产约 250 行；测试约 320 行。总投入 3.4h，规划 1.36h / 编码 0.17h / 验证 1.87h。任务 T116–T118、T128–T129。
- 预期 / Result：`go build ./cmd/sol-scanner` 成功；退出码测试通过；`Run` 在游标冲突与游标 DB 错误时返回且不写 halt；Produce/RPC 非 unsupported 仍重试。

### PLAN-026 处理进程装配 / Processor process assembly

- 背景 / Background：写协程和发布循环还没有进程把它们放在一起。v1.6.2：AUD-026-01 证明共享 `package ioc` 使 FR-17 与 AC-094 互斥；Coder 叶子装配被正确判定为 FR-17 失败。
- 目标 / Goal：`sol-processor --config` 在不读取 RPC 的前提下经处理侧 InitX 启动；停扫时写协程停、发布继续、进程不退出；`go list -deps` 对两命令同时满足 AC-094。
- 范围 / Scope：`cmd/sol-processor`、处理侧 InitX、退出码测试、K-03 e2e；以及 v1.6.2 最小拆包（迁扫块 ioc 文件、改 `cmd/sol-scanner` import、父包仅保留 DB InitX）。
- 非目标 / Non-goals：不调用 `GenesisHash` 或 `FinalizedSlot`；不把两个进程合成一个 main；不改 loop/processor 业务语义；不用 build tags；不在 `cmd` 叶子装配。
- 接口 / Interfaces：spec FR-17（含 v1.6.2 包边界）的处理半边。有未处理 halt 时不调用写协程启动函数，仍启动发布循环。数据库错误返回 1。信号返回 0。`defaultBuildRuntime` 必须调用 `ioc.InitDB` / `ioc.CloseDB` 与 `internal/ioc/processor` 的 InitX，不得直接 `postgre.Open` / 手写 Kafka/仓储装配。
- 策略 / Strategy：先补/改失败测试：断言处理 `main` 生产路径依赖处理 InitX，且 `go list -deps` 对 `./cmd/sol-processor` 与 `./cmd/sol-scanner` 分别满足 AC-094。再拆包：父 `internal/ioc` 仅 `db.go`；扫块文件迁入 `internal/ioc/scanner`；处理文件迁入 `internal/ioc/processor`；两 `main` 改 import 并去掉叶子装配。K-03 e2e 仍用本地容器，不访问 Infura。
- allowed_paths：`cmd/sol-processor/**`、`cmd/sol-scanner/main.go`（仅 import / InitX 调用点）、`internal/ioc/db.go`、`internal/ioc/scanner/**`、`internal/ioc/processor/**`、`internal/test/integration/processor_runtime_test.go`；可删除已迁走的旧单包 ioc 文件。
- 依赖 / Dependencies：PLAN-014、PLAN-015、PLAN-022、PLAN-024；扫块侧 InitX 已由 PLAN-025 落地。本修订在 PLAN-025 AUDIT 之后执行。
- 验收 / Acceptance：AC-067、AC-068、AC-069、AC-070、AC-071、AC-072、AC-078 在进程边界上的结果、**AC-094（两命令）**、AC-096、FR-17。
- per-plan 命令 / Command：`go test ./cmd/sol-processor/...`；`go test ./cmd/sol-scanner/...`；`go build -o /dev/null ./cmd/sol-processor`；`go build -o /dev/null ./cmd/sol-scanner`；`go list -deps ./cmd/sol-processor`；`go list -deps ./cmd/sol-scanner`；`make e2e E2E_RUN='^TestProcessorRuntimeKafkaDown$'`。
- final acceptance 贡献 / Contribution：两个进程可被 PLAN-028 同时启动。本 Plan 不跑 A-01。
- 风险 / Risks：拆包后扫块 `main` 漏改 import；处理 `main` 再叶子装配。缓解：FR-17 测试 + 双命令 `go list -deps`。
- 回滚 / Rollback：恢复单包 `internal/ioc` 与叶子装配则同时破坏 FR-17 或 AC-094；回滚应整修订回退到 v1.6.1 形状并暂停处理进程交付。
- 预算 / Budget：约 8 个文件（含迁址）；生产约 260 行（多为迁址）；测试约 320 行。总投入约 3.2h。任务 T119–T121、T130–T132。
- 预期 / Result：两命令构建成功；处理 `main` 经 InitX 装配；两命令 `go list -deps` 满足 AC-094；下游不通时发布游标不动。

### PLAN-027 治理文档对齐 / Align governance docs

- 背景 / Background：`AGENTS.md` 与 `constitution.md` 仍描述旧目录和旧连续性。
- 目标 / Goal：两份文档指向 design §5.1 和 spec 的双进程规则，并写明服务尚未因此就已上线。
- 范围 / Scope：只改 `AGENTS.md`、`constitution.md`。README 仍是两行且没有错误架构声明，不改。
- 非目标 / Non-goals：不新增运行手册；不把未实现命令写成当前可用。
- 接口 / Interfaces：无新 Go 接口。文档中的目录、依赖方向、启动顺序和连续性必须与 spec FR-10、FR-12、FR-17 一致。
- 策略 / Strategy：在两个进程命令已经存在后修改。文档命令只保留当时真实存在的 `go test`、`go vet`、`go build`、`gofmt -l .`、`make e2e`、`go run ./cmd/migrate`，以及落地后的两个 `go run` 入口。明确写“需 `CGO_ENABLED=1`”。
- allowed_paths：`AGENTS.md`、`constitution.md`。
- 依赖 / Dependencies：PLAN-025、PLAN-026。
- 验收 / Acceptance：AC-077 的文档部分：无密钥、无 `replace` 建议、无旧的 `internal/publisher/`、`internal/rpc/provider`、`internal/parser` 目标路径。
- per-plan 命令 / Command：`gofmt -l .` 不适用文档。人工核对后执行 `go test ./...` 与 `go vet ./...`，证明文档修改没有夹带代码。
- final acceptance 贡献 / Contribution：后续实现不再被旧宪法带回单进程。不跑主网。
- 风险 / Risks：文档把计划写成已上线。缓解：保留“未完成总体验收前不得声称可上线”。
- 回滚 / Rollback：恢复两个 Markdown 文件。
- 预算 / Budget：2 个文件；生产代码 0 行；文档约 200 行。总投入 1.6h，规划 0.64h / 编码 0.08h / 验证 0.88h。任务 T122–T123。
- 预期 / Result：治理文档与 v1.6 目录一致，测试命令仍通过。

### PLAN-028 最终总体验收 / Final acceptance

- 背景 / Background：子 Plan 只证明各自接口。主网只读验收必须等两个进程都在。
- 目标 / Goal：一次性执行 spec 的 A-01–A-04，并先做离线集成门禁。
- 范围 / Scope：`internal/test/integration/live_acceptance_test.go`。未设置 `SOL_SCANNER_LIVE_E2E=1` 时 `t.Skip`。DSN 主机不是 `127.0.0.1` 或 `localhost` 时失败。使用 gitignore 的 `config/config.local.yaml`。
- 非目标 / Non-goals：不把该测试放进 PLAN-014–PLAN-027 的完成条件；有可验证限速信号时不把 A-04 记成失败，但不把单独的 `last_lag>150` 当成限速；不打印密钥。
- 接口 / Interfaces：harness 启动 `sol-scanner` 与 `sol-processor`，读取本机 PostgreSQL 和 Kafka，只对 Infura 做 design §6.6 的只读调用。
- 策略 / Strategy：先提交 Skip 和本机 DSN 拒绝测试，`make e2e` 不得因此访问主网。全部功能 Plan 集成后，由 Controller 在一次性环境执行 live 命令。证据记录 slot 区间、脱敏结果和 A-04 的实测落后值。v1.6.5：A-04 判定为 `internal/test/lagcheck` 纯函数并离线覆盖全部分支（T133）；live harness 仅适配 Source 与吞吐探针门禁，输出 `verdict/gate/fa04` 判定行（T134）。
- allowed_paths：`internal/test/integration/live_acceptance_test.go`；v1.6.4 追加 `internal/test/jsonparsed/**`、`internal/event/block/consumer.go`、`internal/processor/loop.go`、`internal/ioc/scanner/block_event.go`、`internal/ioc/processor/processor.go`、`scripts/test_docker_compose.yml`、`Makefile`（仅 T126 live 稳定性与 AUD-028 修复；不扩产品语义）；v1.6.5 追加 `internal/test/lagcheck/**`（T133）与 `internal/test/integration/live_acceptance_test.go` 的 A-04 段（T134）。
- 依赖 / Dependencies：PLAN-014–PLAN-027。
- 验收 / Acceptance：AC-073、AC-074、AC-075、AC-076、AC-077、AC-097。
- per-plan 命令 / Command：实现 harness 时只跑 `go test -tags=e2e ./internal/test/integration/ -run TestLiveAcceptance`，预期 Skip。总体验收命令不在子 Plan 重复：

```sh
go test ./...
go vet ./...
gofmt -l .
make e2e
SOL_SCANNER_LIVE_E2E=1 go test -tags=e2e ./internal/test/integration/ -run TestLiveAcceptance -count=1
```

- final acceptance 贡献 / Contribution：本 Plan 就是去重后的 `final_acceptance_checks`。
- 风险 / Risks：live 配置指向共享库。缓解：AC-097 拒绝非本机 DSN。主网不可用时记录阻塞，不改验收标准。
- 回滚 / Rollback：删除 live 测试文件。不回滚已集成的功能 Plan。
- 预算 / Budget：3 个文件；生产 0 行；测试约 350 + 330 行（v1.6.5 lagcheck 约 130 + 测试约 150 + harness 净改约 50）。总投入 7.8h，规划 3.12h / 编码 0.39h / 验证 4.29h。任务 T124–T127、T133–T134。验证占比含一次性主网观察。
- 预期 / Result：默认 `make e2e` 不访问主网；授权运行后得到 A-01–A-04 的脱敏证据。

## 14. 实施顺序与依赖 / Execution Order and Dependencies

```text
已落地 PLAN-001..007
不执行 PLAN-008..013

PLAN-014 配置 ─┬─▶ PLAN-021 扫块循环 ─▶ PLAN-025 扫块进程 ─┐
PLAN-015 000002 ┼─▶ PLAN-020 游标仓储 ─┘                    │
PLAN-016 provider ┘                                         ├─▶ PLAN-027 文档 ─▶ PLAN-028 总体验收
PLAN-017 parser ─▶ PLAN-022 写协程 ─▶ PLAN-026 处理进程 ─┤
PLAN-018 mqx ─▶ PLAN-019 区块消息 ─┘                         │
PLAN-018 ─▶ PLAN-023 交易消息 ─▶ PLAN-024 发布循环 ────────┘
```

可并行 / Parallel after dependencies：PLAN-014、PLAN-015、PLAN-016、PLAN-018 可并行。PLAN-017 在 PLAN-016 之后。PLAN-021 与 PLAN-022 在依赖满足后可并行。PLAN-025 与 PLAN-026 可并行。未获 Human 明确授权时不派多个 Coder。

## 15. 测试与验证策略 / Testing and Verification Strategy

总原则 / Principle：行为变更先有失败测试。迁址先移动测试并看到旧路径失败。真实 PostgreSQL 和 Kafka 只在 `internal/test/integration` 使用 `//go:build e2e`。子 Plan 不跑 A-01–A-04。

`per_plan_checks`：各 Plan 正文中的命令。离线包用 `go test`。涉及新 SQL 或真实 Kafka 的，只用对应 `-run` 的 `make e2e`。

`final_acceptance_checks`：

| ID | 内容 / Check | 依赖 / Depends | 环境 / Environment | 状态 / Status |
|---|---|---|---|---|
| FA-00 | `go test ./...`、`go vet ./...`、`gofmt -l .`、一次完整 `make e2e` | PLAN-014–PLAN-027 | 本地 Go；专用 Compose | passed (2026-11-23；完整 `make e2e` exit 0) |
| FA-01 | A-01 / AC-073 | PLAN-028 | 本机 PG/Kafka，Infura 只读，`SOL_SCANNER_LIVE_E2E=1` | passed (2026-11-23；v1.6.5 T126 重跑；slots `[472892306,472892505]`) |
| FA-02 | A-02 / AC-074 | FA-01 的同一段 200 个 slot | 同上，另取 20 块独立 jsonParsed 对照 DB | passed (2026-11-23；v1.6.5 T126 重跑；独立 jsonParsed) |
| FA-03 | A-03 / AC-075 | 同一段 | 下游 topic | passed (2026-11-23；v1.6.5 T126；deduped=50466) |
| FA-04 | A-04 / AC-076 | T133 离线分支测试通过；T134；两个进程连续 10 分钟 + 吞吐探针 | 同上；判定与门禁见 AC-076 | blocked (2026-11-23；`verdict=recorded gate=insufficient fa04=blocked last_lag=1708 tip_rate=4.62 probe_rate=1.31`；Infura；非 passed) |

FA-04 状态语义 / FA-04 status semantics：`passed` = `Judge=pass` 且 gate=`sufficient`（仅高吞吐 RPC 可达）；`blocked` = `Judge=recorded`，或 `Judge=pass` 但 gate 为 `insufficient`/`unknown`（当前 Infura 预期）；`failed` = `Judge=fail`；`pending` = 尚无 T134 之后的判定行。状态只按证据中的判定行记录，不按退出码。T133 的离线测试证明判定分支，属 per-plan 检查，不是 FA-04 本身。

证据绑定命令、退出码、代码快照和配置是否只含占位符。相关代码变化只使受影响证据失效。集成提交阶段不跑这些命令。

## 16. 风险与权衡 / Risks and Trade-Offs

| 风险 / Risk | 触发 / Trigger | 影响 / Impact | 缓解 / Mitigation | 停止或回滚 / Stop or rollback |
|---|---|---|---|---|
| JSON 往返丢失字段 | FV-04 失败 | 处理侧解析与直连不一致 | 用现有夹具做门禁 | 停止，返回 Planner |
| Kafka 镜像不健康 | Compose 超时 | E2E 不能证明投递 | F4 调官方等价配置 | 不改消息契约；仍失败则阻塞 |
| 扫块或写协程超过 400 行 | PLAN-021、PLAN-022 | 审核面变大 | 已按单一状态机保留 | 不按行数拆功能；Human 批准预算后执行 |
| 旧 scanner 测试与新规则冲突 | PLAN-021 开始 | 无法同时满足废止 AC | 替换旧测试，不弱化新断言 | 不得恢复空 checkpoint 直接入库 |
| 处理进程误连 RPC | import provider | 破坏 AC-094 | `go list -deps` | 删除该导入后才能通过 |
| 主网限速 | 429/TRANSIENT 或吞吐探针不足 | 不能判断容量 | 记录实测、gate 和瓶颈；FA-04=blocked | 不把单独的 lag>150 当限速；不改 AC-076 判定优先级 |
| 测试库不是本机 | live DSN 主机异常 | 误写共享库 | AC-097 直接失败 | 不继续验收 |

## 17. 方案验收标准 / Plan Acceptance Criteria

本基线落地后：两个命令可构建；扫块进程只投递；处理进程单写协程落库并发布；`000001` 未改；`sol_scan_cursor` 由 `000002` 创建；旧 provider 和 parser 目录不存在；无 `internal/publisher/`；无 `replace`。AC-038–AC-051 没有实现任务。AC-073–AC-076 只在 PLAN-028 有证据后才能称为通过。在此之前不得声称服务已上线。

## 18. 推荐执行命令 / Recommended Commands

当前已经存在 / Existing now：

```sh
go test ./...
go vet ./...
go build ./...
gofmt -l .
make e2e
make e2e E2E_RUN='^(TestMigrationsUpDownStatus|TestSchemaGate)$'
go run ./cmd/migrate status --config ./config/config.local.yaml
```

对应 Plan 落地后才使用 / Valid only after the named plan：

```sh
go test ./internal/config/...
go test ./internal/scanner/provider/...
go test ./internal/processor/parser/...
CGO_ENABLED=1 go test ./internal/pkg/mqx/...
go test ./internal/event/block/...
go test ./internal/event/transaction/...
go test ./internal/scanner/...
go test ./internal/processor/...
go test ./cmd/sol-scanner/...
go test ./cmd/sol-processor/...
make e2e E2E_RUN='^TestMqx'
make e2e E2E_RUN='^TestBlockQueueRoundTrip$'
make e2e E2E_RUN='^TestScanCursorRepository$'
make e2e E2E_RUN='^(TestPublishRepository|TestTransactionProducerKafka)$'
make e2e E2E_RUN='^TestProcessorRuntimeKafkaDown$'
```

`CGO_ENABLED=1 go run ./cmd/sol-scanner` 与 `CGO_ENABLED=1 go run ./cmd/sol-processor` 只在 PLAN-025、PLAN-026 完成后成为入口。现在不存在这两个命令。没有 `make lint` 或 `serve`。

## 19. 资源分配与变更预警 / Effort Allocation and Change Warnings

整个 v1.6 新工作合计约 52.6h：规划 21.04h（40%）、编码 2.63h（5%）、验证 28.93h（55%）。已落地 PLAN-001–PLAN-007 不计入。编码只计生产实现；测试设计和测试代码计入验证。

| Plan | 生产行预警 / Prod | 测试行预警 / Test | 总 h | 40/5/55 h |
|---|---|---|---|---|
| PLAN-014 | 180 | 250 | 3.8 | 1.52 / 0.19 / 2.09 |
| PLAN-015 | 40 | 180 | 2.8 | 1.12 / 0.14 / 1.54 |
| PLAN-016 | 0 净增 | 0 净增 | 1.4 | 0.56 / 0.07 / 0.77 |
| PLAN-017 | 0 净增 | 0 净增 | 1.8 | 0.72 / 0.09 / 0.99 |
| PLAN-018 | 250 | 400 | 4.0 | 1.60 / 0.20 / 2.20 |
| PLAN-019 | 320 | 500 | 5.4 | 2.16 / 0.27 / 2.97 |
| PLAN-020 | 180 | 250 | 3.0 | 1.20 / 0.15 / 1.65 |
| PLAN-021 | 480，超过约 400 | 900 | 6.5 | 2.60 / 0.33 / 3.57 |
| PLAN-022 | 420，超过约 400 | 700 | 4.2 | 1.68 / 0.21 / 2.31 |
| PLAN-023 | 220 | 250 | 1.8 | 0.72 / 0.09 / 0.99 |
| PLAN-024 | 350 | 550 | 5.6 | 2.24 / 0.28 / 3.08 |
| PLAN-025 | 220 | 250 | 2.8 | 1.12 / 0.14 / 1.54 |
| PLAN-026 | 240 | 300 | 2.8 | 1.12 / 0.14 / 1.54 |
| PLAN-027 | 0 | 0 | 1.6 | 0.64 / 0.08 / 0.88 |
| PLAN-028 | 0 | 680 | 7.8 | 3.12 / 0.39 / 4.29 |

约 400 行生产和约 1200 行测试是预警，不是拆分依据。只有 PLAN-021 与 PLAN-022 超过生产预警，二者都是单一状态机，本基线请求 Human 按完整能力批准。没有 Plan 把测试写成 1200 行额度。

各 Plan 内每个功能任务沿用该 Plan 的 40/5/55，不另改比例。实际偏离时先查范围漂移或重复工作，不按时间把一个能力切开。

## 20. 正向与反向追踪 / Forward and Reverse Traceability

| 验收 / Acceptance | Plan |
|---|---|
| AC-001–AC-005 | PLAN-001 已落地 |
| AC-006–AC-009、AC-078、AC-079 | PLAN-014 |
| AC-010–AC-012 版本 1 | PLAN-003 已落地 |
| AC-012 版本 2、AC-013、AC-080 | PLAN-015 |
| AC-014–AC-019、AC-081 | PLAN-016 |
| AC-020–AC-033、AC-082 | PLAN-017 |
| AC-034–AC-037 | PLAN-007 已落地 |
| AC-038–AC-051 | 废止，无 Plan |
| AC-052–AC-055 | PLAN-018 |
| AC-056、AC-057 | PLAN-023 |
| AC-058–AC-063 | PLAN-024 |
| AC-064–AC-066、AC-084–AC-087、AC-095 | PLAN-021、PLAN-025 |
| AC-067、AC-068、AC-072、AC-096 | PLAN-025、PLAN-026 |
| AC-088 | PLAN-020 |
| AC-089–AC-094 | PLAN-022；AC-094 的进程依赖由 PLAN-026 再查 |
| AC-073–AC-077、AC-097 | PLAN-028 |

反向 / Reverse：`sol_scan_cursor` 来自 design §4 与 AC-080。`stepSlots`、`slotsPerWorker`、`blockTopic` 来自 design §5.4 与 AC-078、AC-079。`internal/event/block` 来自 design §5.1 与 AC-083。confluent 依赖来自 design §5.2 与 AC-052。两个命令来自 design §5.1 与 AC-096。

## 21. 最终决策摘要 / Final Decision Summary

- 选定两个进程、区块 topic、`000002`、包迁址、单写协程、条件投递游标和停扫回退。
- 不执行 PLAN-008–PLAN-013，不修复旧单进程循环来满足废止验收。
- 不改 `000001`、解析口径、下游 `index` `"2"`、`LogAlerter` 和 `chain-api-go v0.1.0`。
- 主网 A-01–A-04 只属于 PLAN-028。
- Human 于 2026-11-22 批准本文件。`PLAN-v1.6-SOL-SCANNER-v1` 现在是实现基线。
