# Solana 扫链服务 v1 任务清单 / Solana Scanner Service v1 Tasks

**规格文档 / Spec**: [`docs/superpowers/001-sol-scanner-v1/spec.md`](spec.md)  
**技术方案 / Plan**: [`docs/superpowers/001-sol-scanner-v1/plan.md`](plan.md)  
**任务清单 / Tasks**: [`docs/superpowers/001-sol-scanner-v1/tasks.md`](tasks.md)  
**基线版本 / Baseline Version**: `PLAN-v1.6.5-SOL-SCANNER-v1`  
**日期 / Date**: 2026-11-23  
**状态 / Status**: 已批准 / Approved

本清单对齐同目录 `spec.md` 的 AC-078–AC-097 与 `plan.md`。Human 于 2026-11-23 批准 `PLAN-v1.6.5`（A-04 逻辑与环境分离；新增 T133–T134；v1.6.4 路径扩权与 v1.6.3 写协程澄清仍有效）。一次只执行一个 Plan。

This checklist matches AC-078–AC-097 in `spec.md` and `plan.md`. Human approved `PLAN-v1.6.5` on 2026-11-23 (A-04 logic/environment split; adds T133–T134; v1.6.4 path expansion and v1.6.3 apply-loop clarifications still apply). Execute one Plan at a time.

## 1. 任务格式 / Task Format

`- [ ] T### [P] 中文动作 / English action`

- `[P]` 只表示依赖满足后可与同阶段任务并行，不授权多 Agent。/ `[P]` means parallel-capable after dependencies, not a multi-agent grant.
- 行为实现任务必须依赖更早的失败测试。新 API 未定义导致的编译失败算 RED；环境、网络、Docker 故障不算 RED。
- Behavior implementation depends on an earlier failing test. Compile failure from a missing API counts as RED. Environment, network, or Docker failures do not.
- Budget 格式：`文件数；生产行；测试行；总 h（规划/编码/验证 = 40/5/55）`。
- T001–T129 编号稳定，不重编、不复用。v1.6.2 新增 T130–T132；v1.6.5 新增 T133–T134。/ IDs T001–T129 stay stable; v1.6.2 adds T130–T132; v1.6.5 adds T133–T134.

## 2. 依赖总览 / Dependency Overview

```text
已落地 PLAN-001..007 = T001..T043，不重做
禁止执行 PLAN-008..013 = T044..T072

T073..T077 PLAN-014 ─┬─▶ T100..T105 PLAN-021 ─▶ T116..T118 PLAN-025 ─┐
T078..T081 PLAN-015 ─┼─▶ T096..T099 PLAN-020 ─┘                      │
T082..T083 PLAN-016 ─┘                                                ├─▶ T122..T123 PLAN-027 ─▶ T124..T127 PLAN-028
T084..T085 PLAN-017 ─▶ T106..T108 PLAN-022 ─▶ T119..T121 PLAN-026 ─┤
T086..T089 PLAN-018 ─▶ T090..T095 PLAN-019 ─┘                        │
T086..T089 PLAN-018 ─▶ T109..T110 PLAN-023 ─▶ T111..T115 PLAN-024 ──┘

v1.6.5：T126(已执行) ─▶ T133 ─▶ T134 ─▶ Controller 重跑 T126 命令 ─▶ T127
```

中文：PLAN-014、015、016、018 的首个任务无相互依赖。PLAN-017 在 PLAN-016 之后。PLAN-021 与 PLAN-022 在各自前置完成后才可并行。PLAN-025 与 PLAN-026 同理。未获 Human 授权不派多个 Coder。T106–T127 的完整条目在本段之后继续，编号已在此保留。

English: The first tasks of PLAN-014, 015, 016, and 018 do not depend on each other. PLAN-017 follows PLAN-016. PLAN-021 and PLAN-022 may run in parallel only after their prerequisites. PLAN-025 and PLAN-026 likewise. Full entries for T106–T127 continue after this part; their IDs are reserved here.

## 3. 已落地编号 / Landed IDs

状态均为「已落地，不重做」/ Status for every row: Landed, do not re-execute. 不计入 v1.6 增量预算。

| ID | Plan | 能力 / Capability |
|---|---|---|
| T001 | PLAN-001 | 为 `GetBlocks`、`GetFirstAvailableBlock` 写失败测试 / Failing tests for the two passthroughs |
| T002 | PLAN-001 | 实现两个透传方法 / Implement both passthrough methods |
| T003 | PLAN-001 | 场景目录计数的失败测试 / Failing scenario-catalog count test |
| T004 | PLAN-001 | 补齐场景目录与 Solana runner / Sync catalog and Solana runner |
| T005 | PLAN-001 | 发布前门禁与 C-05 / Release gate and C-05 |
| T006 | PLAN-001 | 打附注标签 `v0.1.0` / Annotated tag `v0.1.0` |
| T007 | PLAN-001 | 确认 `v0.1.0` 可解析 / Confirm `v0.1.0` resolves |
| T008 | PLAN-002 | `go.mod` 与配置加载失败测试 / Module and failing load tests |
| T009 | PLAN-002 | 配置校验失败测试 / Failing validation tests |
| T010 | PLAN-002 | 严格加载与类型 / Strict loader and types |
| T011 | PLAN-002 | 字段校验 / Field validation |
| T012 | PLAN-002 | 示例配置与 gitignore / Example config and gitignore |
| T013 | PLAN-002 | 初版 `AGENTS.md` / Initial `AGENTS.md` |
| T014 | PLAN-003 | 迁移发现失败测试 / Failing migration-discovery tests |
| T015 | PLAN-003 | migrate CLI 失败测试 / Failing migrate CLI tests |
| T016 | PLAN-003 | PostgreSQL E2E Compose 与 Makefile / PostgreSQL e2e Compose and Makefile |
| T017 | PLAN-003 | 七表往返与 schema gate 集成测试 / Seven-table round-trip and schema-gate tests |
| T018 | PLAN-003 | `000001` 成对 SQL / Paired `000001` SQL |
| T019 | PLAN-003 | 迁移 runner / Migration runner |
| T020 | PLAN-003 | 连接、Ping 与 gate / Open, ping, and gate |
| T021 | PLAN-003 | `cmd/migrate` / Migrate command |
| T022 | PLAN-003 | AGENTS 中的迁移与 e2e 命令 / Migration and e2e commands in AGENTS |
| T023 | PLAN-004 | Reader 失败测试 / Failing Reader tests |
| T024 | PLAN-004 | 错误分类失败测试 / Failing error-classification tests |
| T025 | PLAN-004 | provider 错误类型 / Provider error types |
| T026 | PLAN-004 | Reader 实现 / Reader implementation |
| T027 | PLAN-005 | 录制 slot 471991643 夹具 / Record slot 471991643 fixture |
| T028 | PLAN-005 | 领域编码失败测试 / Failing domain-code tests |
| T029 | PLAN-005 | 领域类型与常量 / Domain types and constants |
| T030 | PLAN-005 | jsonparsed 加载与账户提取 / jsonparsed load and account extraction |
| T031 | PLAN-005 | 区块头与交易失败测试 / Failing block-header and transaction tests |
| T032 | PLAN-005 | 区块头与交易解析 / Block header and transaction parsing |
| T033 | PLAN-006 | jsonparsed 转账提取 / jsonparsed transfer extraction |
| T034 | PLAN-006 | 转账失败测试 / Failing transfer tests |
| T035 | PLAN-006 | 余额变化失败测试 / Failing balance-change tests |
| T036 | PLAN-006 | 指令解码器 / Instruction decoder |
| T037 | PLAN-006 | 转账解析并挂接 `ParseBlock` / Transfer parsing hooked to `ParseBlock` |
| T038 | PLAN-006 | Token-2022 余额解析并挂接 / Token-2022 balance parsing hooked in |
| T039 | PLAN-007 | entity 转换失败测试 / Failing entity-conversion tests |
| T040 | PLAN-007 | 一块一事务集成测试 / One-block transaction integration test |
| T041 | PLAN-007 | `ScanRepository` 接口 / `ScanRepository` interface |
| T042 | PLAN-007 | entity 与领域转换 / Entity and domain conversion |
| T043 | PLAN-007 | 扫描 DAO / Scan DAO |

## 4. 被替代编号 / Superseded IDs

状态均为「被 v1.6 替代，禁止执行」/ Status for every row: Superseded by v1.6, do not execute.

| ID | Plan | 能力 / Capability |
|---|---|---|
| T044 | PLAN-008 | 旧告警失败测试 / Old alert failing test |
| T045 | PLAN-008 | 旧 `LogAlerter` 任务 / Old `LogAlerter` task |
| T046 | PLAN-008 | 旧连续性失败测试 / Old continuity failing test |
| T047 | PLAN-008 | 旧单进程扫描失败测试 / Old single-process scan failing test |
| T048 | PLAN-008 | 旧 `checkContinuity` / Old `checkContinuity` |
| T049 | PLAN-008 | 旧写库扫描循环 / Old scan loop that writes the database |
| T050 | PLAN-009 | 旧 E2E Kafka 脚手架任务 / Old e2e Kafka scaffold task |
| T051 | PLAN-009 | 旧 mqx 离线失败测试 / Old mqx offline failing test |
| T052 | PLAN-009 | 旧 mqx 集成失败测试 / Old mqx integration failing test |
| T053 | PLAN-009 | 旧 mqx 实现 / Old mqx implementation |
| T054 | PLAN-010 | 旧交易消息失败测试 / Old transaction-message failing test |
| T055 | PLAN-010 | 旧交易消息类型 / Old transaction message types |
| T056 | PLAN-011 | 旧发布仓储集成测试 / Old publish-repository integration test |
| T057 | PLAN-011 | 旧发布循环单测 / Old publish-loop unit test |
| T058 | PLAN-011 | 旧发布 Kafka 集成测试 / Old publish Kafka integration test |
| T059 | PLAN-011 | 旧 `PublishRepository` 接口 / Old `PublishRepository` interface |
| T060 | PLAN-011 | 旧发布 DAO / Old publish DAO |
| T061 | PLAN-011 | 旧单进程发布循环 / Old single-process publish loop |
| T062 | PLAN-012 | 旧启动门禁失败测试 / Old preflight failing test |
| T063 | PLAN-012 | 旧单进程 main 测试 / Old single-process main test |
| T064 | PLAN-012 | 旧 K-03 运行测试 / Old K-03 runtime test |
| T065 | PLAN-012 | 旧 `preflight.go` / Old `preflight.go` |
| T066 | PLAN-012 | 旧单进程 ioc / Old single-process ioc |
| T067 | PLAN-012 | 旧 `cmd/sol-scanner` 单进程 / Old single-process `cmd/sol-scanner` |
| T068 | PLAN-012 | 旧运行说明 / Old runbook notes |
| T069 | PLAN-013 | 旧主网 harness / Old mainnet harness |
| T070 | PLAN-013 | 旧 FA-00 / Old FA-00 |
| T071 | PLAN-013 | 旧 A-01..A-03 / Old A-01..A-03 |
| T072 | PLAN-013 | 旧 A-04 / Old A-04 |

已存在的 `internal/pkg/alert` 与 `internal/scanner` 不是这些任务的完成证据。新行为由 T100 之后的任务替换。

The existing `internal/pkg/alert` and `internal/scanner` packages are not completion evidence for these tasks. New behavior replaces them from T100 onward.

## 5. 新任务 / New Tasks

本段到 T105 为止。T106–T127 在下一段，不在这里写半条。

This part stops at T105. T106–T127 follow in the next part; no partial task is written here.

### PLAN-014 配置契约 / Configuration contract

#### Tests First / 测试先行

- [ ] T073 重写 `internal/config/load_test.go`，使 v1.6 加载器在缺失时失败。
  - Rewrite `internal/config/load_test.go` so the missing v1.6 loaders fail.
  - Plan: PLAN-014.
  - Scope: `internal/config/load_test.go`。
  - Dependencies: none。
  - Acceptance: AC-006、AC-008、AC-078；RED 原因为 `LoadForScanner` / `LoadForProcessor` 未定义，或现有加载器仍接受 `scanWindowSlots`。用例覆盖 CFG-01、CFG-02、CFG-04。
  - Acceptance: AC-006, AC-008, AC-078; RED because `LoadForScanner` / `LoadForProcessor` are undefined, or the current loader still accepts `scanWindowSlots`. Cases CFG-01, CFG-02, CFG-04.
  - Verification: `go test ./internal/config/... -run 'TestLoadForScanner|TestLoadForProcessor|TestLoadForMigration'`。
  - Budget: 1 文件；生产 0；测试约 180；1.0h（0.40/0.05/0.55）。
  - Done when: 命令因目标 API 或旧键拒绝缺失而失败，且失败不是缺包、网络或测试写错。

- [ ] T074 [P] 重写 `internal/config/validate_test.go`，覆盖步长与旧键拒绝。
  - Rewrite `internal/config/validate_test.go` for step size and rejection of the old keys.
  - Plan: PLAN-014.
  - Scope: `internal/config/validate_test.go`。
  - Dependencies: none。
  - Acceptance: AC-007、AC-079；RED 原因为校验仍要求 `scanWindowSlots` / `fetchConcurrency`，或尚未拒绝 0 与大于 500000 的步长。错误断言含字段路径且不含 URL/DSN。
  - Acceptance: AC-007, AC-079; RED because validation still requires the old keys or does not reject 0 and values above 500000. Errors include the field path and exclude URL/DSN.
  - Verification: `go test ./internal/config/... -run TestValidate`。
  - Budget: 1 文件；生产 0；测试约 80；0.8h（0.32/0.04/0.44）。
  - Done when: `TestValidate` 因 v1.6 规则未实现而失败。

#### Implementation / 实现

- [ ] T075 实现 `LoadForScanner` 与 `LoadForProcessor`。
  - Implement `LoadForScanner` and `LoadForProcessor`.
  - Plan: PLAN-014.
  - Scope: `internal/config/load.go`；另 `internal/config/types.go`（同一加载契约，不可只改其一）。
  - Dependencies: T073.
  - Acceptance: AC-006、AC-008、AC-078；T073 中除示例文件尚未更新外的用例转绿。处理进程不展开 `rpc.httpURL`。
  - Acceptance: AC-006, AC-008, AC-078; T073 is green except the example-file case. The processor loader does not expand `rpc.httpURL`.
  - Verification: `go test ./internal/config/... -run 'TestLoadForScanner|TestLoadForProcessor|TestLoadForMigration'`。
  - Budget: 2 文件；生产约 140；测试 0；1.0h（0.40/0.05/0.55）。
  - Done when: 指定命令通过，且处理进程用例在未设置 `INFURA_API_KEY` 时成功。

- [ ] T076 把校验改为 `stepSlots`、`slotsPerWorker` 与 topic 规则。
  - Change validation to `stepSlots`, `slotsPerWorker`, and the topic rules.
  - Plan: PLAN-014.
  - Scope: `internal/config/validate.go`。
  - Dependencies: T074, T075.
  - Acceptance: AC-007、AC-079；T074 转绿。不要求 `stepSlots` 能被 `slotsPerWorker` 整除。
  - Acceptance: AC-007, AC-079; T074 is green. Divisibility is not required.
  - Verification: `go test ./internal/config/... -run TestValidate`。
  - Budget: 1 文件；生产约 40；测试 0；0.6h（0.24/0.03/0.33）。
  - Done when: `TestValidate` 全部通过。

- [ ] T077 把 `config/config.example.yaml` 换成 v1.6 键。
  - Replace `config/config.example.yaml` with the v1.6 keys.
  - Plan: PLAN-014.
  - Scope: `config/config.example.yaml`。
  - Dependencies: T075.
  - Acceptance: AC-006、AC-009；示例含 `stepSlots: 20000`、`slotsPerWorker: 500`、`blockTopic: sol-scanner.blocks`、`transactionTopic: sol-scanner.transactions` 与 `${INFURA_API_KEY}`。文件不含 `scanWindowSlots` 或 `fetchConcurrency`。T073 的示例用例转绿。
  - Acceptance: AC-006, AC-009; the example has those four values and `${INFURA_API_KEY}`, and omits the old scanner keys. The T073 example case is green.
  - Verification: `go test ./internal/config/...`。
  - Budget: 1 文件；生产约 20 行 YAML；测试 0；0.4h（0.16/0.02/0.22）。
  - Done when: `go test ./internal/config/...` 通过，且示例文件检索不到两个旧键名。

### PLAN-015 投递游标迁移 / Handoff-cursor migration

#### Tests First / 测试先行

- [ ] T078 改 `internal/pkg/postgre/migrations_test.go`，要求存在未实现的 `000002`。
  - Change `internal/pkg/postgre/migrations_test.go` to require the missing `000002`.
  - Plan: PLAN-015.
  - Scope: `internal/pkg/postgre/migrations_test.go`。
  - Dependencies: none。
  - Acceptance: AC-080 的离线发现部分；RED 原因为迁移集合仍只有 `000001`，或 `000001` 的名称被改动。断言 `000001` 仍是 `initial_schema`，并期望恰好再有一对 `000002` / `scan_cursor`。
  - Acceptance: AC-080 offline discovery; RED because the set is still only `000001`. The test keeps `000001` named `initial_schema` and expects one `000002` / `scan_cursor` pair.
  - Verification: `go test ./internal/pkg/postgre/...`。
  - Budget: 1 文件；生产 0；测试约 40；0.6h（0.24/0.03/0.33）。
  - Done when: 失败原因是缺少 `000002` 文件，不是测试无法编译或 Docker 失败。

- [ ] T079 [P] 扩展迁移集成测试，要求第八张表和版本 2 gate。
  - Extend the migration integration test for the eighth table and version-2 gate.
  - Plan: PLAN-015.
  - Scope: `internal/test/integration/migrations_test.go`。
  - Dependencies: none。
  - Acceptance: AC-012、AC-013、AC-080；RED 原因为 Up 后没有 `sol_scan_cursor`，或 Down 误删七张旧表的断言尚无实现可满足。用例包含只删除游标表、`status` 显示版本 2 pending/applied。
  - Acceptance: AC-012, AC-013, AC-080; RED because Up does not create `sol_scan_cursor`. The case requires Down to remove only that table and status to show version 2.
  - Verification: `make e2e E2E_RUN='^(TestMigrationsUpDownStatus|TestSchemaGate)$'`。
  - Budget: 1 文件；生产 0；测试约 140；1.0h（0.40/0.05/0.55）。
  - Done when: 失败原因是缺少 `000002` 或 gate 不认识版本 2，且 Compose 已清理。

#### Implementation / 实现

- [ ] T080 新增 `000002_scan_cursor` 成对 SQL。
  - Add the paired `000002_scan_cursor` SQL.
  - Plan: PLAN-015.
  - Scope: `scripts/postgre/000002_scan_cursor.up.sql`；另 `.down.sql`（成对迁移不可分割）。
  - Dependencies: T078, T079.
  - Acceptance: AC-080；Up 只创建 `id SMALLINT PRIMARY KEY CHECK (id = 1)`、`handed_slot BIGINT NOT NULL`、`updated_at BIGINT NOT NULL`。Down 只删除 `sol_scan_cursor`。`000001` 两个文件字节不变。
  - Acceptance: AC-080; Up creates only that table and Down drops only it. Both `000001` files stay byte-identical.
  - Verification: `git diff -- scripts/postgre/000001_initial_schema.up.sql scripts/postgre/000001_initial_schema.down.sql` 无输出；`go test ./internal/pkg/postgre/...`。
  - Budget: 2 文件；生产约 20 行 SQL；测试 0；0.6h（0.24/0.03/0.33）。
  - Done when: 发现测试通过，且 `000001` diff 为空。

- [ ] T081 让 schema gate 要求 `(2, scan_cursor)`。
  - Make the schema gate require `(2, scan_cursor)`.
  - Plan: PLAN-015.
  - Scope: `internal/pkg/postgre/database.go`。
  - Dependencies: T080.
  - Acceptance: AC-012、AC-013；`requiredSchemaMigrations` 追加版本 2 且不改版本 1。T079 转绿。
  - Acceptance: AC-012, AC-013; version 2 is appended and version 1 is unchanged. T079 is green.
  - Verification: `make e2e E2E_RUN='^(TestMigrationsUpDownStatus|TestSchemaGate)$'`。
  - Budget: 1 文件；生产约 20；测试 0；0.6h（0.24/0.03/0.33）。
  - Done when: 该 `make e2e` 退出码 0，且结束后无 `sol-scanner-service-e2e` 残留容器。

### PLAN-016 provider 迁址 / Move provider

#### Tests First / 测试先行

- [ ] T082 把 provider 测试移到 `internal/scanner/provider`，先看到新包缺失。
  - Move the provider tests to `internal/scanner/provider` and observe the missing package.
  - Plan: PLAN-016.
  - Scope: `internal/scanner/provider/reader_test.go`；另 `internal/scanner/provider/errors_test.go`（同一 Reader 契约的两份既有测试，不可只移一份）。
  - Dependencies: none。
  - Acceptance: AC-081、AC-014–AC-019；RED 原因为新包没有 `Reader` 实现。断言文本保持搬迁前的原样。
  - Acceptance: AC-081, AC-014–AC-019; RED because the new package has no `Reader` implementation. Assertions stay unchanged.
  - Verification: `go test ./internal/scanner/provider/...`。
  - Budget: 2 文件；生产 0；测试 0 净增；0.6h（0.24/0.03/0.33）。
  - Done when: 命令因新包缺少实现或无法编译而失败，不是断言被放宽。

#### Implementation / 实现

- [ ] T083 把 provider 实现迁到新包并删除旧目录。
  - Move the provider implementation into the new package and delete the old directory.
  - Plan: PLAN-016.
  - Scope: `internal/scanner/provider/reader.go`；另 `errors.go`（实现与错误类型同一契约），以及 `internal/scanner/scanner.go`、`internal/scanner/scanner_test.go` 的导入路径。删除 `internal/rpc/provider/**`。
  - Dependencies: T082.
  - Acceptance: AC-014–AC-019、AC-081；T082 转绿。重试、分类和 getBlock 参数不变。旧 scanner 行为保持可编译，不在本任务改写。
  - Acceptance: AC-014–AC-019, AC-081; T082 is green. Retry, classification, and getBlock parameters stay unchanged. The old scanner remains compilable.
  - Verification: `go test ./internal/scanner/provider/...`；`go test ./internal/scanner/...`；`go build ./...`。
  - Budget: 4 个移动/删除路径；生产 0 净增；测试 0；0.8h（0.32/0.04/0.44）。
  - Done when: 三条命令通过，且仓库中不存在 `internal/rpc/provider`。

### PLAN-017 parser 迁址 / Move parser

#### Tests First / 测试先行

- [ ] T084 把解析测试和夹具移到 `internal/processor/parser`。
  - Move parser tests and fixtures to `internal/processor/parser`.
  - Plan: PLAN-017.
  - Scope: `internal/processor/parser/*_test.go` 与 `internal/processor/parser/testdata/**`；另 `internal/test/jsonparsed/jsonparsed.go` 的夹具根路径（测试无法独立于夹具位置）。
  - Dependencies: T083.
  - Acceptance: AC-082、AC-020–AC-033；RED 原因为新包没有 `ParseBlock`。期望值保持 P-01–P-10，含 `ix_index=1`。
  - Acceptance: AC-082, AC-020–AC-033; RED because the new package has no `ParseBlock`. Expected P-01–P-10 values stay, including `ix_index=1`.
  - Verification: `go test ./internal/processor/parser/...`。
  - Budget: 测试与夹具移动；生产 0；测试 0 净增；0.8h（0.32/0.04/0.44）。
  - Done when: 命令因新包缺少解析实现而失败，且夹具路径已指向新目录。

#### Implementation / 实现

- [ ] T085 把解析实现迁入新包并删除 `internal/parser`。
  - Move the parser implementation into the new package and delete `internal/parser`.
  - Plan: PLAN-017.
  - Scope: `internal/processor/parser/*.go`（不含测试）；另 `internal/scanner/scanner.go` 的 parser 导入。删除 `internal/parser/**`。
  - Dependencies: T084.
  - Acceptance: AC-020–AC-033、AC-082；T084 与 `go test ./internal/test/jsonparsed/... ./internal/domain/...` 转绿。
  - Acceptance: AC-020–AC-033, AC-082; T084 and the jsonparsed/domain tests are green.
  - Verification: `go test ./internal/processor/parser/... ./internal/test/jsonparsed/... ./internal/domain/...`；`go build ./...`。
  - Budget: 移动现有实现；生产 0 净增；测试 0；1.0h（0.40/0.05/0.55）。
  - Done when: 两条命令通过，且 `internal/parser` 不存在。

### PLAN-018 Kafka 批量生产者 / Kafka batch producer

#### Tests First / 测试先行

- [ ] T086 在专用 Compose 中追加 Kafka，并让 Makefile 传入 broker。
  - Add Kafka to the dedicated Compose project and pass the broker through the Makefile.
  - Plan: PLAN-018.
  - Scope: `scripts/test_docker_compose.yml`；另 `Makefile` 的 `E2E_ENV`（同一 E2E 入口，不可只改一边）。
  - Dependencies: none。
  - Acceptance: AC-052 的环境前提。保留现有 `postgres:14.14` 与端口 `15442`。追加 `apache/kafka:3.9.0`，宿主端口 `19092`，`SOL_SCANNER_E2E_KAFKA=127.0.0.1:19092`。这是脚手架，不要求业务 RED。
  - Acceptance: AC-052 environment prerequisite. Keep PostgreSQL 14.14 on port 15442. Add `apache/kafka:3.9.0` on host port 19092 and the Kafka env var. Scaffolding does not need a behavioral RED.
  - Verification: `make e2e E2E_RUN='^(TestMigrationsUpDownStatus)$'`。
  - Budget: 2 文件；生产约 30 行 YAML；测试 0；0.8h（0.32/0.04/0.44）。
  - Done when: 该命令退出码 0，PostgreSQL 用例仍通过，退出后 Compose 项目被 `down -v` 清理。镜像不健康时只按 F4 调整官方等价配置，端口保持 19092。

- [ ] T087 [P] 编写 `internal/pkg/mqx/producer_test.go` 的离线失败测试。
  - Write failing offline tests in `internal/pkg/mqx/producer_test.go`.
  - Plan: PLAN-018.
  - Scope: `internal/pkg/mqx/producer_test.go`。
  - Dependencies: none。
  - Acceptance: AC-053、AC-054、AC-055；RED 原因为 `ProduceBatch` 未定义。覆盖不可达 broker、2 秒内取消、空批次、`Close` 6 秒内返回，以及 Flush 超时参数为 1000。
  - Acceptance: AC-053, AC-054, AC-055; RED because `ProduceBatch` is undefined. Cover an unreachable broker, cancel within 2 seconds, an empty batch, `Close` within 6 seconds, and Flush timeout 1000.
  - Verification: `CGO_ENABLED=1 go test ./internal/pkg/mqx/...`。
  - Budget: 1 文件；生产 0；测试约 220；1.0h（0.40/0.05/0.55）。
  - Done when: 失败原因是 API 未定义或行为缺失，不是 cgo 工具链缺失。cgo 缺失按 F4 停止并报告，不改客户端。

- [ ] T088 编写真实 Kafka 的 mqx 失败集成测试。
  - Write the failing real-Kafka mqx integration test.
  - Plan: PLAN-018.
  - Scope: `internal/test/integration/mqx_test.go`；另 `internal/test/integration/helpers_test.go` 的 broker 地址读取（测试与地址入口同一 E2E 契约）。
  - Dependencies: T086.
  - Acceptance: AC-052；RED 原因为生产者未定义。三条带 key 消息全部投递后才返回 nil，消费端 key 与 JSON 一致。文件首行是 `//go:build e2e`。
  - Acceptance: AC-052; RED because the producer is undefined. Three keyed messages return nil only after all are delivered, and the consumer sees the same key and JSON. The file starts with `//go:build e2e`.
  - Verification: `make e2e E2E_RUN='^TestMqx'`。
  - Budget: 2 文件；生产 0；测试约 180；1.0h（0.40/0.05/0.55）。
  - Done when: 失败原因是 mqx API 缺失，且容器已清理。

#### Implementation / 实现

- [ ] T089 实现 `internal/pkg/mqx` 并加入唯一的新依赖。
  - Implement `internal/pkg/mqx` and add the one new dependency.
  - Plan: PLAN-018.
  - Scope: `internal/pkg/mqx/producer.go`；另 `internal/pkg/mqx/types.go`、`go.mod`、`go.sum`（批量生产者与 `Keyed[T]` 同一契约；依赖必须同时锁定）。
  - Dependencies: T087, T088.
  - Acceptance: AC-052–AC-055；T087 与 T088 转绿。直接依赖只新增 `github.com/confluentinc/confluent-kafka-go/v2 v2.11.0`。`go.mod` 仍无 `replace`。
  - Acceptance: AC-052–AC-055; T087 and T088 are green. The only new direct dependency is `confluent-kafka-go/v2 v2.11.0`. `go.mod` still has no `replace`.
  - Verification: `CGO_ENABLED=1 go test ./internal/pkg/mqx/...`；`make e2e E2E_RUN='^TestMqx'`。
  - Budget: 4 文件；生产约 220；测试 0；1.2h（0.48/0.06/0.66）。
  - Done when: 两条命令通过，且 `go.mod` 的 require 差集只有该 Kafka 模块。

### PLAN-019 区块队列消息 / Block-queue messages

#### Tests First / 测试先行

- [ ] T090 [P] 编写区块消息编码失败测试，含夹具往返。
  - Write failing block-message encoding tests, including the fixture round trip.
  - Plan: PLAN-019.
  - Scope: `internal/event/block/message_test.go`。
  - Dependencies: T085, T089.
  - Acceptance: AC-083；RED 原因为 `BlockMessage`、`Key` 或 JSON 编解码未定义。skipped 的 key 为 `102` 且 JSON 无 `block`。夹具 `GetBlockResult` 往返后 `ParseBlock` 与直接解析一致；不一致则停止并返回 Planner，不改解析口径。
  - Acceptance: AC-083; RED because the message type, `Key`, or JSON codec is undefined. Skipped slot 102 has key `102` and no `block`. The fixture round trip must match direct `ParseBlock`; a mismatch stops and returns to Planner.
  - Verification: `go test ./internal/event/block/ -run TestBlockMessage`。
  - Budget: 1 文件；生产 0；测试约 180；1.2h（0.48/0.06/0.66）。
  - Done when: 命令因类型或往返行为缺失而失败。若失败已证明往返不等，任务停止并附夹具差异，不继续 T093。

- [ ] T091 [P] 编写区块生产者的假 mqx 失败测试。
  - Write failing block-producer tests against a fake mqx.
  - Plan: PLAN-019.
  - Scope: `internal/event/block/producer_test.go`。
  - Dependencies: T085, T089.
  - Acceptance: AC-083；RED 原因为区块生产者未定义。假 mqx 收到的 key 是无前导零的十进制 slot；假 mqx 返回错误时生产者返回该错误。
  - Acceptance: AC-083; RED because the block producer is undefined. The fake mqx receives a decimal slot key without leading zeros, and a fake error is returned unchanged.
  - Verification: `go test ./internal/event/block/ -run TestBlockProducer`。
  - Budget: 1 文件；生产 0；测试约 120；0.8h（0.32/0.04/0.44）。
  - Done when: 失败原因是生产者 API 或 key/错误行为缺失。

- [ ] T092 [P] 编写消费者与真实 Kafka 往返的失败测试。
  - Write failing consumer tests and the real-Kafka round trip.
  - Plan: PLAN-019.
  - Scope: `internal/event/block/consumer_test.go`；另 `internal/test/integration/block_queue_test.go`（离线位点语义与一条真实往返不可拆成两个交付）。
  - Dependencies: T085, T089.
  - Acceptance: AC-083；RED 原因为消费者未定义。离线测试要求自动提交关闭，并存在分开的 Commit 与 Seek。集成测试要求一条 block 与一条 skipped 经 Kafka 读回。集成文件使用 `//go:build e2e`。
  - Acceptance: AC-083; RED because the consumer is undefined. Offline tests require auto-commit off and separate Commit and Seek. The integration test reads back one block and one skipped message. The integration file uses `//go:build e2e`.
  - Verification: `go test ./internal/event/block/ -run TestBlockConsumer`；`make e2e E2E_RUN='^TestBlockQueueRoundTrip$'`。
  - Budget: 2 文件；生产 0；测试约 200；1.0h（0.40/0.05/0.55）。
  - Done when: 两条命令都因消费者缺失而失败，且 e2e 容器已清理。

#### Implementation / 实现

- [ ] T093 实现区块消息类型与 JSON 编解码。
  - Implement the block message type and JSON codec.
  - Plan: PLAN-019.
  - Scope: `internal/event/block/message.go`。
  - Dependencies: T090.
  - Acceptance: AC-083；T090 转绿。`version=1`，`commitment=finalized`，`kind` 只有 `block` 与 `skipped`。
  - Acceptance: AC-083; T090 is green. `version=1`, `commitment=finalized`, and `kind` is only `block` or `skipped`.
  - Verification: `go test ./internal/event/block/ -run TestBlockMessage`。
  - Budget: 1 文件；生产约 120；测试 0；0.6h（0.24/0.03/0.33）。
  - Done when: `TestBlockMessage` 通过。若 T090 已因往返不等停止，本任务不开始。

- [ ] T094 实现区块生产者。
  - Implement the block producer.
  - Plan: PLAN-019.
  - Scope: `internal/event/block/producer.go`。
  - Dependencies: T091, T093.
  - Acceptance: AC-083；T091 转绿。生产者只调用 mqx，不推进游标，不写数据库。
  - Acceptance: AC-083; T091 is green. The producer calls mqx only and does not advance a cursor or write the database.
  - Verification: `go test ./internal/event/block/ -run TestBlockProducer`。
  - Budget: 1 文件；生产约 80；测试 0；0.8h（0.32/0.04/0.44）。
  - Done when: `TestBlockProducer` 通过。

- [ ] T095 实现关闭自动提交的区块消费者。
  - Implement the block consumer with auto-commit disabled.
  - Plan: PLAN-019.
  - Scope: `internal/event/block/consumer.go`。
  - Dependencies: T092, T093.
  - Acceptance: AC-083；T092 的离线测试与 `TestBlockQueueRoundTrip` 转绿。Commit 与 Seek 由调用方分开触发。
  - Acceptance: AC-083; T092's offline test and `TestBlockQueueRoundTrip` are green. The caller triggers Commit and Seek separately.
  - Verification: `go test ./internal/event/block/...`；`make e2e E2E_RUN='^TestBlockQueueRoundTrip$'`。
  - Budget: 1 文件；生产约 120；测试 0；1.0h（0.40/0.05/0.55）。
  - Done when: 两条命令通过。

### PLAN-020 投递游标仓储 / Handoff-cursor repository

#### Tests First / 测试先行

- [ ] T096 编写投递游标仓储的失败集成测试。
  - Write the failing handoff-cursor repository integration test.
  - Plan: PLAN-020.
  - Scope: `internal/test/integration/scan_cursor_repository_test.go`。
  - Dependencies: T081.
  - Acceptance: AC-088；RED 原因为 `ScanCursorRepository` 未定义。覆盖无行、`prev=startSlot-1` 时插入、条件前进、错误 prev 不改行、从 21000 回退到 101、回退到 499。文件使用 `//go:build e2e`。
  - Acceptance: AC-088; RED because `ScanCursorRepository` is undefined. Cover no row, insert when `prev=startSlot-1`, conditional advance, a wrong prev leaving the row unchanged, rewind from 21000 to 101, and rewind to 499. The file uses `//go:build e2e`.
  - Verification: `make e2e E2E_RUN='^TestScanCursorRepository$'`。
  - Budget: 1 文件；生产 0；测试约 250；1.2h（0.48/0.06/0.66）。
  - Done when: 失败原因是接口或 DAO 缺失，容器已清理。

#### Implementation / 实现

- [ ] T097 定义 `ScanCursorRepository`。
  - Define `ScanCursorRepository`.
  - Plan: PLAN-020.
  - Scope: `internal/repository/scan_cursor.go`。
  - Dependencies: T096.
  - Acceptance: AC-088 的接口形状：`GetHandedSlot`、`AdvanceHandedSlot`、`RewindHandedSlot` 使用值参数。本任务完成后 T096 仍因缺少 DAO 而失败。
  - Acceptance: AC-088 interface shape: `GetHandedSlot`, `AdvanceHandedSlot`, and `RewindHandedSlot` take value arguments. T096 still fails for the missing DAO.
  - Verification: `go test ./internal/repository/...`。
  - Budget: 1 文件；生产约 30；测试 0；0.4h（0.16/0.02/0.22）。
  - Done when: 包测试通过，且 `postgre` 尚未 import `repository`。

- [ ] T098 增加 `sol_scan_cursor` 的 GORM entity。
  - Add the GORM entity for `sol_scan_cursor`.
  - Plan: PLAN-020.
  - Scope: `internal/repository/postgre/entity.go`。
  - Dependencies: T097.
  - Acceptance: AC-088 的映射；列名是 `id`、`handed_slot`、`updated_at`，表名是 `sol_scan_cursor`。不在 Go 文件里写 DDL。
  - Acceptance: AC-088 mapping; columns are `id`, `handed_slot`, and `updated_at`, and the table name is `sol_scan_cursor`. No DDL is placed in Go.
  - Verification: `go test ./internal/repository/postgre/...`；`go vet ./internal/repository/postgre/...`。
  - Budget: 1 文件；生产约 30；测试 0；0.6h（0.24/0.03/0.33）。
  - Done when: vet 通过，且 entity 文件不含 `CREATE TABLE`。

- [ ] T099 实现投递游标 DAO 与仓储构造。
  - Implement the handoff-cursor DAO and repository constructor.
  - Plan: PLAN-020.
  - Scope: `internal/repository/postgre/scan_cursor.go`；另 `internal/repository/scan_cursor.go` 中的构造函数（接口与 `NewScanCursorRepository(dao)` 同一装配点）。
  - Dependencies: T098.
  - Acceptance: AC-088；T096 转绿。条件更新影响行数不为 1 时返回冲突且行不变。回退可把 `handed_slot` 改小。`postgre` 不 import `internal/repository`。
  - Acceptance: AC-088; T096 is green. A conditional update that does not affect one row returns a conflict and leaves the row unchanged. Rewind may lower `handed_slot`. `postgre` does not import `internal/repository`.
  - Verification: `make e2e E2E_RUN='^TestScanCursorRepository$'`；`go test ./internal/repository/...`。
  - Budget: 2 文件；生产约 120；测试 0；0.8h（0.32/0.04/0.44）。
  - Done when: 两条命令通过，且 `postgre` 的 import 没有 `internal/repository`。

### PLAN-021 扫块循环 / Scanner handoff loop

#### Tests First / 测试先行

- [ ] T100 [P] 编写分段纯函数的失败测试。
  - Write failing tests for the pure segment function.
  - Plan: PLAN-021.
  - Scope: `internal/scanner/segment_test.go`。
  - Dependencies: T077, T083, T095, T099.
  - Acceptance: AC-084、AC-085 的区间部分；RED 原因为 `Segments` 未定义。`Segments(1001,21000,500)` 得到 40 段，首段 `[1001,1500]`，末段 `[20501,21000]`。`Segments(1001,1800,500)` 只有 `[1001,1500]` 与 `[1501,1800]`。
  - Acceptance: AC-084, AC-085 range portion; RED because `Segments` is undefined. The 40-segment and 1800 cases match those bounds.
  - Verification: `go test ./internal/scanner/ -run TestSegments`。
  - Budget: 1 文件；生产 0；测试约 120；0.8h（0.32/0.04/0.44）。
  - Done when: 失败原因是 `Segments` 未定义。

- [ ] T101 [P] 用假依赖重写扫块循环测试，并移除旧窗口断言。
  - Rewrite the scan-loop tests with fakes and remove the old window assertions.
  - Plan: PLAN-021.
  - Scope: `internal/scanner/scanner_test.go`。
  - Dependencies: T077, T083, T095, T099.
  - Acceptance: AC-084、AC-085、AC-086、AC-087、AC-095；RED 原因为新循环 API 未定义。覆盖 40 段一次推进到 21000、乱序回执、101/103 与 skipped 102、空列表、`getBlocks` 失败不发 skipped、`getBlock` 失败不推进、finalized 1800 不产生 1801、已追平不调用 `getBlocks`、生产失败不推进。不得再断言旧的 `scanWindowSlots` 行为。
  - Acceptance: AC-084, AC-085, AC-086, AC-087, AC-095; RED because the new loop API is undefined. Cover those cases. Do not keep assertions for the old window behavior.
  - Verification: `go test ./internal/scanner/ -run 'TestStep|TestHandoff'`。
  - Budget: 1 文件；生产 0；测试约 500；1.2h（0.48/0.06/0.66）。
  - Done when: 命令因新循环缺失而失败，且文件不再引用旧窗口用例名。

- [ ] T102 [P] 编写扫块启动门禁的失败测试。
  - Write failing scanner preflight tests.
  - Plan: PLAN-021.
  - Scope: `internal/scanner/preflight_test.go`。
  - Dependencies: T077, T083, T095, T099.
  - Acceptance: AC-064、AC-065、AC-066、AC-067 的扫块半边；RED 原因为门禁函数未定义。无游标且 `startSlot` 为 0、创世哈希不同、`startSlot` 早于最早区块均拒绝启动。未处理停扫时不调用 `getBlocks` 且不返回退出进程的 halt 错误。
  - Acceptance: AC-064, AC-065, AC-066, and the scanner half of AC-067; RED because preflight is undefined. Reject unset start, genesis mismatch, and too-early start. An unresolved halt does not call `getBlocks` and does not return a process-exiting halt error.
  - Verification: `go test ./internal/scanner/ -run TestPreflight`。
  - Budget: 1 文件；生产 0；测试约 180；0.8h（0.32/0.04/0.44）。
  - Done when: 失败原因是门禁 API 缺失。

#### Implementation / 实现

- [ ] T103 实现闭区间分段函数。
  - Implement the inclusive segment function.
  - Plan: PLAN-021.
  - Scope: `internal/scanner/segment.go`。
  - Dependencies: T100.
  - Acceptance: AC-084、AC-085 的区间部分；T100 转绿。除末段外每段长度等于 `slotsPerWorker`，末段可短。
  - Acceptance: AC-084, AC-085 range portion; T100 is green. Every segment except the last has length `slotsPerWorker`; the last may be shorter.
  - Verification: `go test ./internal/scanner/ -run TestSegments`。
  - Budget: 1 文件；生产约 60；测试 0；0.8h（0.32/0.04/0.44）。
  - Done when: `TestSegments` 通过。

- [ ] T104 实现按段投递的扫块循环，并删除旧连续性实现。
  - Implement the per-segment handoff loop and delete the old continuity implementation.
  - Plan: PLAN-021.
  - Scope: `internal/scanner/loop.go`；另 `internal/scanner/worker.go` 与删除 `internal/scanner/continuity.go`、`internal/scanner/scanner.go`（同一步状态机及其旧入口不可留并存实现）。
  - Dependencies: T101, T103.
  - Acceptance: AC-084、AC-085、AC-086、AC-087、AC-095；T101 转绿。循环先 `FinalizedSlot()`，每段只调用一次 `BlockSlots`。任一段失败不调用 `AdvanceHandedSlot`。生产代码不引用 `ParseBlock`、`SaveBlockBatch` 或交易 topic。
  - Acceptance: AC-084, AC-085, AC-086, AC-087, AC-095; T101 is green. The loop calls `FinalizedSlot()` first and `BlockSlots` once per segment. Any segment failure skips `AdvanceHandedSlot`. Production code does not reference `ParseBlock`, `SaveBlockBatch`, or the transaction topic.
  - Verification: `go test ./internal/scanner/ -run 'TestStep|TestHandoff'`；`go test -race ./internal/scanner/...`。
  - Budget: 4 个路径；生产约 320；测试 0；1.7h（0.68/0.09/0.93）。
  - Done when: 两条命令通过，`continuity.go` 与旧 `scanner.go` 不存在，且 `internal/scanner` 非测试 Go 文件不含 `SaveBlockBatch`。

- [ ] T105 实现扫块启动门禁与未处理停扫等待。
  - Implement scanner preflight and the unresolved-halt wait.
  - Plan: PLAN-021.
  - Scope: `internal/scanner/preflight.go`。
  - Dependencies: T102, T104.
  - Acceptance: AC-064、AC-065、AC-066、AC-067 的扫块半边；T102 转绿。`UNSUPPORTED_VERSION` 返回致命错误且不写 halt。
  - Acceptance: AC-064, AC-065, AC-066, and the scanner half of AC-067; T102 is green. `UNSUPPORTED_VERSION` is fatal and writes no halt.
  - Verification: `go test ./internal/scanner/...`；`go test -race ./internal/scanner/...`。
  - Budget: 1 文件；生产约 100；测试 0；1.2h（0.48/0.06/0.66）。
  - Done when: 两条命令通过，且 `TestPreflight` 不再调用 `getBlocks` 的停扫用例通过。

### PLAN-022 处理写协程 / Processor apply loop

#### Tests First / 测试先行

- [ ] T106 编写处理写协程的应用失败测试。
  - Write failing processor apply tests.
  - Plan: PLAN-022.
  - Scope: `internal/processor/apply_test.go`。
  - Dependencies: T077, T085, T095, T099.
  - Acceptance: AC-089、AC-090、AC-091；RED 原因为写协程 API 未定义。覆盖无 checkpoint 时 500 与 501 skipped 后提交 502 且不比较父哈希、500 缺失时不提交 502、连续 10 次空 poll 缺 102 为 `GAP_UNRESOLVED`、先 ≥10 条更晚 slot 再齐前缀不得假停扫、父哈希不同为 `HASH_MISMATCH`、父 slot 更小为 `PARENT_BEHIND`、101 已提交且 102 skipped 而 103 的父 slot 为 102 时立即 `GAP_UNRESOLVED`、skipped 只把应用位置加一且 checkpoint 不变。空 poll 缺口轮使用注入时钟。
  - Acceptance: AC-089, AC-090, AC-091; RED because the apply API is undefined. Cover the prefix, ten empty-poll gap, no false gap after ≥10 later slots, hash, parent slot, immediate gap after a skipped slot, and skipped leaving checkpoint unchanged. Empty-poll rounds use an injected clock.
  - Verification: `go test ./internal/processor/ -run TestApply`。
  - Budget: 1 文件；生产 0；测试约 400；1.5h（0.60/0.075/0.825）。
  - Done when: 命令因写协程 API 或上述行为缺失而失败，不是时钟真的等待了 10 个间隔。

- [ ] T107 [P] 编写停扫回退与队列位点的失败测试。
  - Write failing tests for halt rewind and queue offsets.
  - Plan: PLAN-022.
  - Scope: `internal/processor/halt_test.go`。
  - Dependencies: T077, T085, T095, T099.
  - Acceptance: AC-092、AC-093、AC-094 的处理侧；RED 原因为停扫回退 API 未定义。停扫行先于告警；告警失败后停扫行仍在；有 checkpoint 101 时 `handed_slot` 变为 101，无 checkpoint 且 `startSlot=500` 时变为 499；已入库行不变。位点在落库、skipped 成功或丢弃已覆盖后提交；停扫后 `resumeOffset >= 0` 时 Seek+Commit，`< 0` 时只 Commit。重复的已覆盖区块不使调用失败且必须 Commit。测试用的生产文件约束是 `internal/processor` 的非测试代码不引用 provider 或 `chain-api-go`。
  - Acceptance: AC-092, AC-093, and the processor half of AC-094; RED because rewind is undefined. The halt row precedes the alert, a failed alert leaves the row, rewind targets are 101 or 499, stored rows stay unchanged, offsets commit after save, skipped, or covered discard; halt Seek+Commit when `resumeOffset >= 0` else Commit only; a duplicate covered block does not fail and must Commit. Non-test processor code must not reference provider or `chain-api-go`.
  - Verification: `go test ./internal/processor/ -run TestHalt`。
  - Budget: 1 文件；生产 0；测试约 300；1.2h（0.48/0.06/0.66）。
  - Done when: 命令因回退或位点行为缺失而失败。

#### Implementation / 实现

- [ ] T108 实现单协程应用、连续性与停扫回退。
  - Implement the single-goroutine apply, continuity, and halt rewind.
  - Plan: PLAN-022.
  - Scope: `internal/processor/loop.go`。
  - Dependencies: T106, T107.
  - Acceptance: AC-089、AC-090、AC-091、AC-092、AC-093、AC-094 的处理侧；T106 与 T107 转绿。只有一个写协程。解码失败写 `DECODE_ERROR` 且不提交该块。停扫调用 `RewindHandedSlot` 后：`resumeOffset >= 0` 则 Seek+Commit，否则只 Commit。
  - Acceptance: AC-089, AC-090, AC-091, AC-092, AC-093, and the processor half of AC-094; T106 and T107 are green. There is one apply goroutine. Decode failure writes `DECODE_ERROR` and does not commit the block. Halt calls `RewindHandedSlot`, then Seek+Commit when `resumeOffset >= 0`, else Commit only.
  - Verification: `go test ./internal/processor/...`；`go test -race ./internal/processor/...`。
  - Budget: 1 文件；生产约 420；测试 0；1.5h（0.60/0.075/0.825）。
  - Done when: 两条命令通过，且 `internal/processor/loop.go` 的 import 不含 `internal/scanner/provider` 或 `chain-api-go`。生产行数超过约 400 时照实记录，不拆开这个状态机。

### PLAN-023 下游交易消息 / Downstream transaction message

#### Tests First / 测试先行

- [ ] T109 编写交易消息 golden 与失败交易 JSON 的失败测试。
  - Write failing golden and failed-transaction JSON tests.
  - Plan: PLAN-023.
  - Scope: `internal/event/transaction/types_test.go`；另 `internal/event/transaction/testdata/k05_message.json`（golden 与断言不可分开）。
  - Dependencies: T089.
  - Acceptance: AC-056、AC-057；RED 原因为 `FromBlockBatch` 未定义。golden 去掉排版空白后与 design §5.3 示例一致，`transfers[0].index` 为 `"2"`。失败交易两个数组编码为 `[]`。内层 index、省略 fee、decimals 为 null、负数 change 带 `-` 且正数不带 `+` 均有断言。输入是 `domain.BlockBatch`，不访问网络或数据库。
  - Acceptance: AC-056, AC-057; RED because `FromBlockBatch` is undefined. The golden matches design §5.3 after whitespace removal, with `transfers[0].index` equal to `"2"`. Failed transactions encode both arrays as `[]`. Inner index, omitted fee, null decimals, and signed change are asserted. Input is a `domain.BlockBatch`, with no network or database.
  - Verification: `go test ./internal/event/transaction/ -run 'TestTransactionMessage|TestFailedTransactionJSON'`。
  - Budget: 2 文件；生产 0；测试约 180；1.0h（0.40/0.05/0.55）。
  - Done when: 命令因映射函数未定义而失败，golden 文件已含 `"index": "2"`。

#### Implementation / 实现

- [ ] T110 实现 `FromBlockBatch` 与交易消息类型。
  - Implement `FromBlockBatch` and the transaction message types.
  - Plan: PLAN-023.
  - Scope: `internal/event/transaction/types.go`。
  - Dependencies: T109.
  - Acceptance: AC-056、AC-057；T109 转绿。相同领域行再次编码的字节相同。
  - Acceptance: AC-056, AC-057; T109 is green. Encoding the same domain rows again yields the same bytes.
  - Verification: `go test ./internal/event/transaction/ -run 'TestTransactionMessage|TestFailedTransactionJSON'`。
  - Budget: 1 文件；生产约 220；测试 0；0.8h（0.32/0.04/0.44）。
  - Done when: 该命令通过，且包内没有 Kafka producer。

### PLAN-024 发布仓储与发布循环 / Publish repository and loop

#### Tests First / 测试先行

- [ ] T111 编写发布仓储的失败集成测试。
  - Write the failing publish-repository integration test.
  - Plan: PLAN-024.
  - Scope: `internal/test/integration/publish_repository_test.go`。
  - Dependencies: T110.
  - Acceptance: AC-061、AC-062；RED 原因为 `PublishRepository` 未定义。无游标时从最早区块开始；游标不后退；topic 不符返回错误。`ListBlockBatches` 按 slot 升序，同一区块的子行一次取全且顺序稳定。文件使用 `//go:build e2e`。
  - Acceptance: AC-061, AC-062; RED because `PublishRepository` is undefined. No cursor starts at the earliest block, the cursor does not move backward, and a topic mismatch returns an error. `ListBlockBatches` is slot-ascending and returns one block's child rows together in a stable order. The file uses `//go:build e2e`.
  - Verification: `make e2e E2E_RUN='^TestPublishRepository$'`。
  - Budget: 1 文件；生产 0；测试约 220；1.2h（0.48/0.06/0.66）。
  - Done when: 失败原因是仓储 API 缺失，容器已清理。

- [ ] T112 [P] 编写发布循环的离线失败测试。
  - Write failing offline publish-loop tests.
  - Plan: PLAN-024.
  - Scope: `internal/event/transaction/producer_test.go`。
  - Dependencies: T110.
  - Acceptance: AC-059、AC-060、AC-063；RED 原因为发布循环未定义。批中失败时游标不动，重发 JSON 逐字节相同。无交易区块不发消息但仍推进游标。退避序列为 1s、2s、4s，上限 30s；退避中取消立即返回。使用注入时钟和假 producer。
  - Acceptance: AC-059, AC-060, AC-063; RED because the publish loop is undefined. A mid-batch failure leaves the cursor unchanged and the retry JSON is byte-identical. An empty block sends nothing and still advances the cursor. Backoff is 1s, 2s, 4s, capped at 30s, and cancel during backoff returns immediately. Use an injected clock and a fake producer.
  - Verification: `go test ./internal/event/transaction/ -run TestPublish`。
  - Budget: 1 文件；生产 0；测试约 180；1.0h（0.40/0.05/0.55）。
  - Done when: 命令因发布循环缺失而失败。

- [ ] T113 [P] 编写真实 Kafka 发布失败集成测试。
  - Write the failing real-Kafka publish integration test.
  - Plan: PLAN-024.
  - Scope: `internal/test/integration/transaction_producer_kafka_test.go`。
  - Dependencies: T110.
  - Acceptance: AC-058、AC-059；RED 原因为发布循环未接入 mqx。一个含成功两笔 transfer、失败、成功无 transfer 的区块发出 3 条消息，key 为签名，失败条为 `FAILED` 且两个数组为空，游标前进。中途失败时游标不动且重发字节相同。文件使用 `//go:build e2e`。
  - Acceptance: AC-058, AC-059; RED because the loop is not wired to mqx. The three-transaction block emits 3 messages keyed by signature, the failed message is `FAILED` with both arrays empty, and the cursor advances. A mid-batch failure leaves the cursor unchanged and the retry bytes match. The file uses `//go:build e2e`.
  - Verification: `make e2e E2E_RUN='^TestTransactionProducerKafka$'`。
  - Budget: 1 文件；生产 0；测试约 150；1.2h（0.48/0.06/0.66）。
  - Done when: 失败原因是发布实现缺失，容器已清理。

#### Implementation / 实现

- [ ] T114 实现 `PublishRepository` 与发布 DAO。
  - Implement `PublishRepository` and the publish DAO.
  - Plan: PLAN-024.
  - Scope: `internal/repository/publish.go`；另 `internal/repository/postgre/publish.go`（接口与 DAO 同一仓储契约）。
  - Dependencies: T111.
  - Acceptance: AC-061、AC-062；T111 转绿。`postgre` 不 import `internal/repository`。不在 Go 中写 DDL。
  - Acceptance: AC-061, AC-062; T111 is green. `postgre` does not import `internal/repository`. No DDL is written in Go.
  - Verification: `make e2e E2E_RUN='^TestPublishRepository$'`；`go test ./internal/repository/...`。
  - Budget: 2 文件；生产约 180；测试 0；1.0h（0.40/0.05/0.55）。
  - Done when: 两条命令通过，且 `publish.go` 的 DAO 文件 import 不含 `internal/repository`。

- [ ] T115 实现按发布游标整块发送的循环。
  - Implement the publish loop that sends one stored block per batch.
  - Plan: PLAN-024.
  - Scope: `internal/event/transaction/producer.go`。
  - Dependencies: T112, T113, T114.
  - Acceptance: AC-058、AC-059、AC-060、AC-061、AC-062、AC-063；T112 与 T113 转绿。循环不访问 RPC，不读取区块队列。全部回执成功后才前进 `published_slot`。
  - Acceptance: AC-058, AC-059, AC-060, AC-061, AC-062, AC-063; T112 and T113 are green. The loop does not touch RPC or the block queue. `published_slot` advances only after every ack succeeds.
  - Verification: `go test ./internal/event/transaction/ -run TestPublish`；`make e2e E2E_RUN='^(TestPublishRepository|TestTransactionProducerKafka)$'`。
  - Budget: 1 文件；生产约 170；测试 0；1.2h（0.48/0.06/0.66）。
  - Done when: 两条命令通过，且 `producer.go` 不 import provider 或 parser。

### PLAN-025 扫块进程装配 / Scanner process assembly

#### Tests First / 测试先行

- [ ] T116 编写 `sol-scanner` 进程退出码的失败测试。
  - Write failing `sol-scanner` process exit-code tests.
  - Plan: PLAN-025.
  - Scope: `cmd/sol-scanner/main_test.go`。
  - Dependencies: T081, T083, T105.
  - Acceptance: AC-064、AC-065、AC-066、AC-067、AC-068、AC-069、AC-070、AC-071、AC-096 的扫块命令；RED 原因为 `sol-scanner` 的 `run` 未定义。缺 `--config` 或多余位置参数退出码 2。无游标且 `startSlot` 为 0、创世哈希不同、`startSlot` 早于最早区块、数据库错误或 `UNSUPPORTED_VERSION` 退出码 1，且不写停扫。未处理停扫时进程保持运行，直到信号才退出码 0。测试使用假 Reader，不访问主网。
  - Acceptance: AC-064, AC-065, AC-066, AC-067, AC-068, AC-069, AC-070, AC-071, and the scanner command in AC-096; RED because `sol-scanner` `run` is undefined. Missing `--config` or extra args exit 2. Unset start, genesis mismatch, too-early start, database error, or `UNSUPPORTED_VERSION` exit 1 and write no halt. An unresolved halt stays running until a signal exits 0. Tests use a fake Reader and do not touch mainnet.
  - Verification: `go test ./cmd/sol-scanner/...`。
  - Budget: 1 文件；生产 0；测试约 250；1.0h（0.40/0.05/0.55）。
  - Done when: 命令因命令包或 `run` 缺失而失败。

#### Implementation / 实现

- [ ] T117 实现扫块进程的 `InitX`。
  - Implement the scanner process `InitX` functions.
  - Plan: PLAN-025.
  - Scope: `internal/ioc/scanner.go`；另 `internal/ioc/db.go`、`internal/ioc/provider.go`、`internal/ioc/block_event.go`（一个进程的数据库、provider 与区块生产者必须一起装配）。
  - Dependencies: T116.
  - Acceptance: AC-096 的装配边界。Init 函数连接 PostgreSQL、schema gate 版本 2、provider 与区块生产者。不装配 parser、`SaveBlockBatch` 或交易 producer。本任务后 T116 仍因 `main` 未接线而失败。
  - Acceptance: AC-096 wiring boundary. Init connects PostgreSQL, schema gate version 2, the provider, and the block producer. It does not wire the parser, `SaveBlockBatch`, or the transaction producer. T116 still fails because `main` is not wired.
  - Verification: `go test ./internal/ioc/...`；`go build ./internal/ioc/`。
  - Budget: 4 文件；生产约 140；测试 0；0.8h（0.32/0.04/0.44）。
  - Done when: 两条命令通过，且这些文件不 import `internal/processor/parser`。

- [ ] T118 实现 `cmd/sol-scanner` 的参数、信号与退出码。
  - Implement `cmd/sol-scanner` arguments, signals, and exit codes.
  - Plan: PLAN-025.
  - Scope: `cmd/sol-scanner/main.go`。
  - Dependencies: T117.
  - Acceptance: AC-064、AC-065、AC-066、AC-067、AC-068、AC-069、AC-070、AC-071、AC-096；T116 转绿。命令名是 `sol-scanner --config <path>`。`SIGINT` / `SIGTERM` 关闭 producer 与数据库后退出码 0。
  - Acceptance: AC-064, AC-065, AC-066, AC-067, AC-068, AC-069, AC-070, AC-071, AC-096; T116 is green. The command is `sol-scanner --config <path>`. `SIGINT` / `SIGTERM` closes the producer and database, then exits 0.
  - Verification: `go test ./cmd/sol-scanner/...`；`go build -o /dev/null ./cmd/sol-scanner`。
  - Budget: 1 文件；生产约 80；测试 0；1.0h（0.40/0.05/0.55）。
  - Done when: 两条命令通过，且 `main.go` 不 import parser 或 `event/transaction`。

#### v1.6.1 Amendment / A-025-01（在 T118 之后，PLAN-025 重审之前）

- [ ] T128 编写 `Run` 对游标冲突与游标 DB 错误致命返回的失败测试。
  - Write failing tests that `Run` returns on handed-slot conflict and cursor DB errors.
  - Plan: PLAN-025.
  - Scope: `internal/scanner/loop_fatal_test.go`（或 `internal/scanner/scanner_test.go` 中仅新增 Run 致命分类用例）。
  - Dependencies: T118.
  - Acceptance: AC-069、FR-09；RED 原因为 `Run` 对 `repository.ErrHandedSlotConflict` 与 `GetHandedSlot` 数据库错误仍休眠重试。断言：冲突/游标 DB 错误时 `Run` 在短超时内返回该错误；不写停扫；游标值不变。对照：`Produce` 失败或非 `UNSUPPORTED_VERSION` 的 provider 错误时 `Run` 不因单次 Step 失败而返回（可取消后结束）。
  - Acceptance: AC-069, FR-09; RED because `Run` still sleep-retries on `ErrHandedSlotConflict` and GetHandedSlot DB errors. Assert conflict/DB causes `Run` to return that error quickly with cursor unchanged and no halt write. Control: Produce or non-unsupported provider failure must not make `Run` return from a single Step failure (end via cancel).
  - Verification: `go test ./internal/scanner/ -run 'TestRunFatal'`.
  - Budget: 1 文件；生产 0；测试约 120；0.4h（0.16/0.02/0.22）。
  - Done when: 命令因 `Run` 仍重试冲突/DB 而失败（或等价 RED），且对照用例编译。

- [ ] T129 实现 `Run` 致命/可重试分类，使 AC-069 Run 路径转绿。
  - Implement `Run` fatal vs retriable classification so AC-069 Run-path turns green.
  - Plan: PLAN-025.
  - Scope: `internal/scanner/loop.go`；测试文件仅限 T128 范围。
  - Dependencies: T128.
  - Acceptance: AC-069、FR-09、AC-095；T128 转绿。致命：`UNSUPPORTED_VERSION`、`ErrHandedSlotConflict`、游标/halt DB 错误 → `Run` 返回。可重试：非 unsupported 的 provider 失败、`Produce` 失败 → 记录并等待后重试。不写停扫。可用包内 sentinel 包裹游标仓储错误以区别 `Produce`；不得改 `worker.go` 投递语义，不得改 preflight。
  - Acceptance: AC-069, FR-09, AC-095; T128 green. Fatal: unsupported, conflict, cursor/halt DB → `Run` returns. Retriable: other provider failures and Produce → log, wait, retry. No halt write. Local sentinel wrap for cursor-store errors is allowed; do not change worker handoff semantics or preflight.
  - Verification: `go test ./internal/scanner/ -run 'TestRunFatal|TestStepUnsupported|TestHandoffProduce'`；`go test -race ./internal/scanner/...`；`go test ./cmd/sol-scanner/...`。
  - Budget: 1 生产文件；生产约 40；测试 0；0.3h（0.12/0.02/0.16）。
  - Done when: 上述命令通过；`loop.go` 以外的 PLAN-025 原有路径无无关改动。

### PLAN-026 处理进程装配 / Processor process assembly

#### Tests First / 测试先行

- [ ] T119 编写 `sol-processor` 退出码与下游不通的失败测试。
  - Write failing `sol-processor` exit-code and downstream-outage tests.
  - Plan: PLAN-026.
  - Scope: `cmd/sol-processor/main_test.go`；另 `internal/test/integration/processor_runtime_test.go`（进程退出码与 K-03 是同一进程行为的两面，测试必须同时落地）。
  - Dependencies: T081, T108, T115.
  - Acceptance: AC-067、AC-068、AC-072、AC-094、AC-096 的处理命令；RED 原因为 `sol-processor` 的 `run` 未定义。未处理停扫时不启动写协程，发布循环仍启动，进程不退出。下游 topic 不通时 checkpoint 可前进且发布游标不动。`go list -deps ./cmd/sol-processor` 不含 `internal/scanner/provider` 或 `chain-api-go`。缺配置退出码 2，数据库错误退出码 1，信号退出码 0。集成文件使用 `//go:build e2e`，不访问 Infura。
  - Acceptance: AC-067, AC-068, AC-072, AC-094, and the processor command in AC-096; RED because `sol-processor` `run` is undefined. An unresolved halt does not start the apply loop, the publish loop still starts, and the process stays up. With the downstream topic unavailable, checkpoint may advance while the publish cursor does not. `go list -deps ./cmd/sol-processor` contains neither `internal/scanner/provider` nor `chain-api-go`. Missing config exits 2, a database error exits 1, and a signal exits 0. The integration file uses `//go:build e2e` and does not contact Infura.
  - Verification: `go test ./cmd/sol-processor/...`；`make e2e E2E_RUN='^TestProcessorRuntimeKafkaDown$'`。
  - Budget: 2 文件；生产 0；测试约 300；1.0h（0.40/0.05/0.55）。
  - Done when: 两条命令都因处理进程缺失而失败，e2e 容器已清理。

#### Implementation / 实现

- [ ] T120 实现处理进程的 `InitX`。
  - Implement the processor process `InitX` functions.
  - Plan: PLAN-026.
  - Scope: `internal/ioc/processor.go`；另 `internal/ioc/transaction_event.go`（写协程与发布循环同一进程装配）。不修改 `internal/ioc/scanner.go`、`provider.go` 或 `block_event.go`。
  - Dependencies: T119.
  - Acceptance: AC-094、AC-096。Init 连接数据库、schema gate 版本 2、区块消费者、写协程和交易发布。不调用 `LoadForScanner`，不构造 provider。T119 仍因 `main` 未接线而失败。
  - Acceptance: AC-094, AC-096. Init connects the database, schema gate version 2, block consumer, apply loop, and transaction publisher. It does not call `LoadForScanner` or construct a provider. T119 still fails because `main` is not wired.
  - Verification: `go test ./internal/ioc/...`；`go build ./internal/ioc/`。
  - Budget: 2 文件；生产约 150；测试 0；0.8h（0.32/0.04/0.44）。
  - Done when: 两条命令通过，且这两个新文件不 import `internal/scanner/provider`。

- [ ] T121 实现 `cmd/sol-processor` 的参数、信号与退出码。
  - Implement `cmd/sol-processor` arguments, signals, and exit codes.
  - Plan: PLAN-026.
  - Scope: `cmd/sol-processor/main.go`。
  - Dependencies: T120.
  - Acceptance: AC-067、AC-068、AC-072、AC-094、AC-096；T119 转绿。命令是 `sol-processor --config <path>`。停扫不使进程退出。
  - Acceptance: AC-067, AC-068, AC-072, AC-094, AC-096; T119 is green. The command is `sol-processor --config <path>`. Halt does not exit the process.
  - Verification: `go test ./cmd/sol-processor/...`；`go build -o /dev/null ./cmd/sol-processor`；`make e2e E2E_RUN='^TestProcessorRuntimeKafkaDown$'`；`go list -deps ./cmd/sol-processor`。
  - Budget: 1 文件；生产约 90；测试 0；1.0h（0.40/0.05/0.55）。
  - Done when: 前三条命令通过，且 `go list -deps` 输出不含 `internal/scanner/provider` 与 `github.com/Stone-afk/chain-api-go`。

#### v1.6.2 Amendment / A-026-01（AUD-026-01；在既有 T119–T121 之上补齐包边界）

- [ ] T130 编写/收紧 FR-17 与双命令 AC-094 的失败测试。
  - Write or tighten failing tests for FR-17 InitX wiring and AC-094 on both commands.
  - Plan: PLAN-026.
  - Scope: `cmd/sol-processor/main_test.go`；必要时 `cmd/sol-scanner/main_test.go` 仅增加 deps 断言。
  - Dependencies: T119.
  - Acceptance: FR-17、AC-094、AC-096；RED 原因为处理 `main` 未经 `internal/ioc` / `internal/ioc/processor` InitX 装配，或 `go list -deps ./cmd/sol-processor` 在共享 `package ioc` 下仍含 `internal/scanner/provider` / `chain-api-go`，或扫块命令 deps 违反 AC-094 扫块半边。
  - Acceptance: FR-17, AC-094, AC-096; RED because processor `main` leaf-wires instead of InitX, or processor deps still pull provider/chain-api-go via shared `ioc`, or scanner deps violate the scanner half of AC-094.
  - Verification: `go test ./cmd/sol-processor/...`；`go list -deps ./cmd/sol-processor`；`go list -deps ./cmd/sol-scanner`。
  - Budget: 1–2 文件；生产 0；测试约 80；0.5h。
  - Done when: 至少一项因 FR-17 叶子装配或包边界失败而 RED；deps 断言已编译。

- [ ] T131 将 `internal/ioc` 拆为父包 DB + `ioc/scanner` + `ioc/processor`。
  - Split `internal/ioc` into parent DB package plus `ioc/scanner` and `ioc/processor`.
  - Plan: PLAN-026.
  - Scope: `internal/ioc/db.go`；`internal/ioc/scanner/**`；`internal/ioc/processor/**`；删除已迁走的旧单包文件。
  - Dependencies: T130.
  - Acceptance: AC-094、FR-17 包边界。父包 `internal/ioc` 的非测试生产文件不得 import `internal/scanner`、`internal/scanner/provider`、`chain-api-go`、`internal/processor` 或 `internal/processor/parser`。扫块子包可依赖 provider；处理子包不得依赖 provider / chain-api-go。
  - Acceptance: AC-094, FR-17 package boundary. Parent `internal/ioc` production code must not import scanner/provider/chain-api-go/processor/parser. Scanner subpackage may depend on provider; processor subpackage must not.
  - Verification: `go build ./internal/ioc/...`；`go list -deps ./internal/ioc`；`go list -deps ./internal/ioc/processor`；`go list -deps ./internal/ioc/scanner`。
  - Budget: 约 6 文件（迁址为主）；生产约 20 行净改；测试 0；0.6h。
  - Done when: 三包可分别 build；父包与 processor 子包 deps 无 provider/chain-api-go。

- [ ] T132 两 `main` 经 InitX 接线并去掉处理侧叶子装配。
  - Wire both mains through InitX and remove processor leaf wiring.
  - Plan: PLAN-026.
  - Scope: `cmd/sol-processor/main.go`；`cmd/sol-scanner/main.go`（仅 import/InitX）；`internal/test/integration/processor_runtime_test.go` import 对齐。
  - Dependencies: T131, T120, T121.
  - Acceptance: AC-067、AC-068、AC-072、AC-094、AC-096、FR-17；T130 转绿。处理 `defaultBuildRuntime` 调用 `ioc.InitDB` 与 `ioc/processor` InitX；扫块 `main` 调用 `ioc.InitDB` 与 `ioc/scanner` InitX。停扫不使处理进程退出。
  - Acceptance: AC-067, AC-068, AC-072, AC-094, AC-096, FR-17; T130 green. Processor runtime uses shared DB InitX plus processor InitX; scanner main uses shared DB InitX plus scanner InitX. Halt does not exit the processor.
  - Verification: `go test ./cmd/sol-processor/...`；`go test ./cmd/sol-scanner/...`；`go build -o /dev/null ./cmd/sol-processor`；`go build -o /dev/null ./cmd/sol-scanner`；`go list -deps ./cmd/sol-processor`；`go list -deps ./cmd/sol-scanner`；`make e2e E2E_RUN='^TestProcessorRuntimeKafkaDown$'`。
  - Budget: 3 文件；生产约 100；测试 0；0.8h。
  - Done when: 全部所列命令通过；处理 deps 无 provider 与 chain-api-go；扫块 deps 无 parser 与 event/transaction；`cmd/sol-processor/main.go` 无直接 `postgre.Open` / 手写 consumer 装配替代 InitX。

### PLAN-027 治理文档对齐 / Align governance docs

#### Implementation / 实现

- [ ] T122 把 `AGENTS.md` 改成双进程目录与真实命令。
  - Update `AGENTS.md` to the two-process layout and the real commands.
  - Plan: PLAN-027.
  - Scope: `AGENTS.md`。
  - Dependencies: T118, T121.
  - Acceptance: AC-077 的文档约束。目标路径是 `internal/scanner/provider`、`internal/processor/parser`、`internal/event/block`、`internal/event/transaction`。文档不含把 `internal/rpc/provider`、`internal/parser` 或 `internal/publisher/` 写成目标路径的句子。命令只保留 `go test ./...`、`go vet ./...`、`go build ./...`、`gofmt -l .`、`make e2e`、`go run ./cmd/migrate`，以及 `CGO_ENABLED=1 go run ./cmd/sol-scanner` 与 `CGO_ENABLED=1 go run ./cmd/sol-processor`。写明总体验收完成前不得声称已上线。无密钥和真实 DSN。
  - Acceptance: AC-077 documentation constraint. Target paths are `internal/scanner/provider`, `internal/processor/parser`, `internal/event/block`, and `internal/event/transaction`. The doc does not present `internal/rpc/provider`, `internal/parser`, or `internal/publisher/` as target paths. Commands are only the listed Go, make, migrate, and two `CGO_ENABLED=1 go run` entries. It states the service must not be called live before final acceptance. No secrets or real DSNs.
  - Verification: `go test ./...`；`go vet ./...`。
  - Budget: 1 文件；生产 0；测试 0；文档约 120 行；0.8h（0.32/0.04/0.44）。
  - Done when: 两条命令通过，且 `git diff --stat` 只有 `AGENTS.md`。

- [ ] T123 把 `constitution.md` 的分层与连续性改成 v1.6。
  - Update `constitution.md` layering and continuity to v1.6.
  - Plan: PLAN-027.
  - Scope: `constitution.md`。
  - Dependencies: T122.
  - Acceptance: AC-077 的文档约束。宪法写明两个进程、先 `FinalizedSlot()` 再分段 `getBlocks`、skipped 不改 checkpoint、无 checkpoint 时第一块前必须已是 skipped、停扫后退游标。删除“无 checkpoint 时第一块不校验”和单进程 `cmd/sol-scanner` 独占启动的目标表述。不改 README。
  - Acceptance: AC-077 documentation constraint. The constitution states two processes, `FinalizedSlot()` before segmented `getBlocks`, skipped leaving checkpoint unchanged, the pre-first-block skipped prefix, and halt rewind. It removes the old “first block skips continuity when checkpoint is absent” rule and the single-process startup target. README stays unchanged.
  - Verification: `go test ./...`；`go vet ./...`；`git diff --name-only`。
  - Budget: 1 文件；生产 0；测试 0；文档约 80 行；0.8h（0.32/0.04/0.44）。
  - Done when: 测试与 vet 通过，diff 文件名只有 `constitution.md`，且 README 不在 diff 中。

### PLAN-028 最终总体验收 / Final acceptance

#### Tests First / 测试先行

- [x] T124 编写默认 Skip 的主网只读验收测试。
  - Write the live read-only acceptance test that skips by default.
  - Plan: PLAN-028.
  - Scope: `internal/test/integration/live_acceptance_test.go`。
  - Dependencies: T123.
  - Acceptance: AC-073、AC-074、AC-075、AC-076、AC-097。未设置 `SOL_SCANNER_LIVE_E2E=1` 时 `t.Skip`。该变量已设置且 DSN 主机不是 `127.0.0.1` 或 `localhost` 时，在拨号前失败。文件使用 `//go:build e2e`。本任务不连接 Infura。
  - Acceptance: AC-073, AC-074, AC-075, AC-076, AC-097. `t.Skip` when `SOL_SCANNER_LIVE_E2E` is unset. When it is set and the DSN host is not `127.0.0.1` or `localhost`, fail before dialing. The file uses `//go:build e2e`. This task does not contact Infura.
  - Verification: `go test -tags=e2e ./internal/test/integration/ -run TestLiveAcceptance -count=1`。
  - Budget: 1 文件；生产 0；测试约 350；1.5h（0.60/0.075/0.825）。
  - Done when: 未设置 live 变量时命令以 Skip 结束且退出码 0；进程内把 DSN 主机改成非本机时断言失败且没有对外连接。

#### Implementation / 实现

- [x] T125 执行一次离线集成门禁，不打开主网开关。
  - Run one offline integration gate without the live switch.
  - Plan: PLAN-028.
  - Scope: 不修改仓库文件。只执行已存在的命令。
  - Dependencies: T124.
  - Acceptance: AC-077。`go test ./...`、`go vet ./...`、`gofmt -l .` 与一次完整 `make e2e` 通过。环境里不得设置 `SOL_SCANNER_LIVE_E2E`。`gofmt -l .` 无输出。
  - Acceptance: AC-077. `go test ./...`, `go vet ./...`, `gofmt -l .`, and one full `make e2e` pass. `SOL_SCANNER_LIVE_E2E` must be unset. `gofmt -l .` prints nothing.
  - Verification: `go test ./...`；`go vet ./...`；`gofmt -l .`；`make e2e`。
  - Budget: 0 文件；生产 0；测试 0；1.0h（0.40/0.05/0.55）。
  - Done when: 四条命令的退出码都是 0，`make e2e` 已清理 Compose 项目，且 live 测试在该次 `make e2e` 中是 Skip。

- [x] T126 在授权的本机环境执行 A-01 至 A-04。
  - Run A-01 through A-04 in the authorized local environment.
  - Plan: PLAN-028.
  - Scope: 不修改仓库文件。使用 gitignore 的 `config/config.local.yaml`。
  - Dependencies: T125.
  - Acceptance: AC-073、AC-074、AC-075、AC-076、AC-097。两个进程连续处理 200 个 slot：入库集合与该范围 `getBlocks` 一致，相邻父 slot 和父哈希接上；其中 20 个区块与 jsonParsed 的交易数、签名、成败、手续费、transfer 和 Token-2022 余额一致；下游消息按签名去重后条数等于交易行数。A-04 按 AC-076 优先级判定，FA-04 按 plan §15 状态语义记录（当前 Infura 仅可 blocked 或 failed）。证据不含完整 URL、DSN 或 API key。
  - Acceptance: AC-073, AC-074, AC-075, AC-076, AC-097. Two processes handle 200 slots: stored blocks match `getBlocks`, and parent slot plus parent hash connect; 20 blocks match jsonParsed on count, signature, status, fee, transfers, and Token-2022 balances; deduped downstream messages equal stored transaction rows. A-04 is judged by the AC-076 priority order and FA-04 is recorded per plan §15 semantics (current Infura: blocked or failed only). Evidence excludes full URLs, DSNs, and API keys.
  - Verification: `SOL_SCANNER_LIVE_E2E=1 go test -tags=e2e ./internal/test/integration/ -run TestLiveAcceptance -count=1`。
  - Budget: 0 文件；生产 0；测试 0；2.0h（0.80/0.10/1.10）。
  - Done when: 命令有真实退出码和脱敏摘要。非本机 DSN、主网不可用或未获 Human 授权时，命令不启动，任务记为阻塞而不是通过。
  - Evidence (2026-11-23, pre-v1.6.5): `SOL_SCANNER_LIVE_E2E=1 make e2e E2E_RUN='^TestLiveAcceptance$' E2E_TIMEOUT=60m` → exit 0（约 13min）。A-04 lag-only「仅记录」已按 AUD-028-02 / v1.6.5 作废。
  - Evidence (2026-11-23, v1.6.5): Human 授权后重跑。① `make e2e` 同命令 exit 0（~787s，无 `-v`）。② 等价 verbose：`SOL_SCANNER_LIVE_E2E=1` + Compose e2e env + `go test -tags=e2e -v … -run '^TestLiveAcceptance$'` → `PASS` / `ok`（~783s）。slot 窗 `[472892306,472892505]`；A-01 200 blocks；A-02 20 blocks 独立 jsonParsed；A-03 deduped=50466。判定行：`A-04 verdict=recorded gate=insufficient fa04=blocked samples=258 last_lag=1708 max_lag=1719 limit_signals=0 tip_rate=4.62 probe_rate=1.31`。FA-04=blocked（Infura 探针不足）；不得声称已上线。

- [ ] T127 汇总总体验收证据，不把服务写成已上线。
  - Collect the final-acceptance evidence and leave the baseline pending.
  - Plan: PLAN-028.
  - Scope: 不修改 `spec.md`、`plan.md`、`tasks.md` 或生产代码。证据交给 Controller 的审核包。
  - Dependencies: T126, T134.
  - Acceptance: AC-073、AC-074、AC-075、AC-076、AC-077、AC-097。审核包列出每条命令、退出码、slot 区间、A-04 实测值，以及 FA-00 与 live 的区分。A-04 项须引用判定行的 verdict、gate、fa04、last_lag、max_lag、限速信号数、tip 与探针速率。本任务不把服务写成已上线，也不改写已批准的基线版本。
  - Acceptance: AC-073, AC-074, AC-075, AC-076, AC-077, AC-097. The review packet lists each command, exit code, slot range, A-04 measurement, and the split between FA-00 and live. This task does not mark the service live and does not rewrite the approved baseline.
  - Verification: 审核包中的命令文本与 T125、T126 的实际输出一致。
  - Budget: 0 文件；生产 0；测试 0；0.6h（0.24/0.03/0.33）。
  - Done when: 审核包含上述字段且无密钥；工作区相对 T126 结束时没有新的文件改动。

#### v1.6.5 Amendment / A-04 逻辑与环境分离（AUD-028-02 后续）

- [x] T133 编写 A-04 判定辅助包与离线分支测试。
  - Write the A-04 judgment helper package and offline branch tests.
  - Plan: PLAN-028.
  - Scope: `internal/test/lagcheck/lagcheck.go`、`internal/test/lagcheck/lagcheck_test.go`（均不带 `e2e` tag）。不改 `internal/test/integration/**`、生产代码或 `go.mod`。
  - Dependencies: T126.
  - Design: `Source{FinalizedSlot(ctx)(uint64,error); Checkpoint(ctx)(uint64,bool,error)}`；`Clock{Now func() time.Time; Sleep func(context.Context, time.Duration) error}`；`Observe(ctx, Source, window, interval, Clock) Observation`，遇非限速错误停止并记 `Err`；`IsRateLimit(err)` = `errors.As(provider.Error)` 且 `HTTPStatus==429 || Kind==ErrorKindTransient`；`ClassifyThroughput(Probe{Slots, Elapsed, LimitSignals}, Observation) Gate`；`Judge(Observation, Gate) Verdict`；`FAStatus(Verdict, Gate) string`。阈值常量 150 在本包，live 引用。值语义，无指针。
  - Acceptance: AC-076、NFR-05、NFR-06。表驱动离线用例全部通过且不访问网络：① tip 每步 +1、checkpoint=tip−10、gate sufficient → pass/passed；② tip 每步 +50、checkpoint 每步 +5、无错误、gate sufficient → fail/failed；②′ 同②但 gate unknown → fail（守护 AUD-028-02：lag 单独不得 recorded）；③ 同②但部分 tick 返回 `provider.Error{Kind:TRANSIENT,HTTPStatus:429}` → recorded/blocked；④ 返回 `provider.Error{Kind:PERMANENT}` → fail；⑤ checkpoint 错误 → fail；⑥ 无样本且无限速 → fail；无样本但有 429 → recorded；⑦ lag 合格但 gate insufficient → recorded/blocked；⑧ lag 合格、gate unknown → pass/blocked。`ClassifyThroughput`：探针有 429 → insufficient；探针速率 < tip 速率 → insufficient；≥ → sufficient；tip 样本 < 2 或 Elapsed=0 → unknown。
  - Acceptance: AC-076, NFR-05, NFR-06. Offline table tests cover every branch above without network; lag > 150 with no verifiable limit signal is always `fail`, never `recorded`.
  - Verification: `go test ./internal/test/lagcheck/... -count=1`；`go vet ./internal/test/lagcheck/...`；`gofmt -l internal/test/lagcheck`；`go test ./...`。
  - Budget: 2 文件；生产 0；测试约 280（辅助约 130 + 测试约 150）；1.2h（0.48/0.06/0.66）。
  - Done when: 先提交测试并观察到因 `lagcheck` 缺失而 RED（编译失败），再补辅助包转绿；以上命令退出码 0；`gofmt -l` 无输出；包内无 `//go:build` 且无网络调用。

- [x] T134 live harness 改用 lagcheck 并加吞吐探针环境门禁。
  - Switch the live harness to lagcheck and add the throughput-probe environment gate.
  - Plan: PLAN-028.
  - Scope: `internal/test/integration/live_acceptance_test.go`（仅 `assertA04` 及其适配 / 探针辅助；不改 A-01–A-03 判定）。
  - Dependencies: T133.
  - Design: Source 适配 `h.reader.FinalizedSlot` 与 `h.scanRepo.GetCheckpoint`（`cp.Slot`）；Clock 用 `time.Now` 与可取消 sleep。观察 `liveLagObserveWindow` 后调用 `h.stopBinaries()`（须幂等，Cleanup 仍会再调）；探针取 `[finalized−39, finalized]`，worker 数 = `ceil(stepSlots/slotsPerWorker)`，每个 worker 顺序 `BlockSlots`+`Block`，`IsRateLimit` 计数，非限速错误 `t.Fatalf`。`Judge` = fail → `t.Fatalf`；其余 `t.Logf` 一行 `A-04 verdict=%s gate=%s fa04=%s samples=%d last_lag=%d max_lag=%d limit_signals=%d tip_rate=%.2f probe_rate=%.2f`，不含 URL/DSN/key 与原始响应。删除旧的「任意错误即 rateLimited」逻辑。
  - Acceptance: AC-076、AC-097、NFR-01。未设 `SOL_SCANNER_LIVE_E2E` 时仍 Skip 且不拨号；fail 必 Fatal；recorded 不 Fatal 且判定行 `fa04=blocked`；不存在仅凭 `last_lag>150` 返回非 Fatal 的路径。
  - Acceptance: AC-076, AC-097, NFR-01. Still skips without the live switch; `fail` always fatals; `recorded` logs `fa04=blocked`; no path returns non-fatal on `last_lag>150` alone.
  - Verification: `go vet -tags=e2e ./internal/test/integration/`；`go test -tags=e2e ./internal/test/integration/ -run TestLiveAcceptance -count=1`（预期 Skip）；`go test ./...`；`gofmt -l .`；`make e2e E2E_RUN='^TestLiveAcceptance$'`（预期 Skip）。live 重跑不在本任务内：Human 授权后由 Controller 执行 `SOL_SCANNER_LIVE_E2E=1 make e2e E2E_RUN='^TestLiveAcceptance$' E2E_TIMEOUT=60m`，记入 T126 证据。
  - Budget: 1 文件；生产 0；测试净改约 50；1.0h（0.40/0.05/0.55）。
  - Done when: 上述离线 / Skip 命令退出码 0；`rg 'rateLimited' internal/test/integration/live_acceptance_test.go` 无匹配；`assertA04` 的判定只经 `lagcheck.Judge` 与 `FAStatus`。

## 6. Final Validation Checklist / 最终验证清单

- [ ] 测试先于实现：T073、T074、T078、T079、T082、T084、T087、T088、T090、T091、T092、T096、T100、T101、T102、T106、T107、T109、T111、T112、T113、T116、T119、T124 都早于各自的实现任务。
- [ ] Tests precede implementation: the test IDs above are dependencies of their implementation tasks.
- [ ] 依赖与 plan 第 14 节以及已输出的 T100–T105 一致。T106 依赖 T077、T085、T095、T099。T116 依赖 T081、T083、T105。T119 依赖 T081、T108、T115。T122 依赖 T118、T121。T124 依赖 T123。
- [ ] Dependencies match plan section 14 and the published T100–T105 graph, including the five edges named above.
- [ ] T001–T043 只保留登记，不重做。T044–T072 禁止执行。AC-038–AC-051 没有新任务。
- [ ] T001–T043 stay registered only. T044–T072 are forbidden. AC-038–AC-051 have no new task.
- [ ] 范围未漂移：`000001` 不在任何新任务的 Scope 中。新依赖只有 T089 的 `confluent-kafka-go/v2 v2.11.0`。没有 `internal/publisher/`。
- [ ] Scope stays put: `000001` is outside every new Scope. The only new dependency is T089's `confluent-kafka-go/v2 v2.11.0`. There is no `internal/publisher/`.
- [ ] 关键约束仍在：先 `FinalizedSlot()`、整步失败不推进游标、skipped 不改 checkpoint、停扫回退、下游 `index` 为 `"2"`、主网默认 Skip。
- [ ] Key constraints remain: `FinalizedSlot()` first, a failed step does not advance the cursor, skipped leaves checkpoint unchanged, halt rewinds, downstream `index` is `"2"`, and mainnet skips by default.
- [ ] 聚焦验证写在每个 T073–T134 的 Verification 中。live A-01–A-04 只出现在 T126（含 v1.6.5 后的重跑），不出现在更早任务的完成条件里；T133 只离线证明 A-04 判定分支，不访问主网。
- [ ] Focused verification is on every T073–T134. Live A-01–A-04 appear only in T126 (including the post-v1.6.5 re-run); T133 proves A-04 branches offline without mainnet.
- [ ] 一次去重后的总体验收是 T124 的 Skip harness、T125 的 FA-00、T126 的 live 运行和 T127 的证据包。
- [ ] The deduplicated final acceptance is T124's skip harness, T125's FA-00, T126's live run, and T127's evidence packet.
