# sol-scanner-service 项目上下文入口

本文是维护本仓库的 Agent 和工程师的操作入口。非简单任务开始前，主动读取下面的上下文文件；路径列表本身不会自动展开。架构与质量底线见 `constitution.md`。用户当前的明确要求优先于普通仓库约定，系统和平台规则仍优先。

## 核心上下文与项目阶段

- `constitution.md`：分层、迁移、连续性/发布、测试先行与协作原则。
- `docs/superpowers/001-sol-scanner-v1/design.md`：产品与数据契约（最高产品权威）。
- `docs/superpowers/001-sol-scanner-v1/spec.md`：需求与验收权威。
- `docs/superpowers/001-sol-scanner-v1/plan.md`、`tasks.md`：技术方案、Plan/Task 边界与顺序。
- `README.md`：对外项目介绍（能力以真实装配为准，勿夸大）。
- `Makefile`、`scripts/test_docker_compose.yml`：标准 E2E 入口和专用 PostgreSQL（及 Kafka）环境。
- `config/config.example.yaml`、`config/config.local.yaml`、`internal/config/`：配置来源、严格解析与本地密钥文件。
- `scripts/postgre/`、`internal/pkg/postgre/`：成对 SQL、迁移执行、连接生命周期和 schema gate。
- 目标包的接口、实现与相邻测试；跨层修改还要读 `cmd/sol-scanner/`、`cmd/sol-processor/`、`cmd/migrate/` 或 `internal/ioc/`（含 `ioc/scanner`、`ioc/processor`）中对应的装配代码。

本仓库实现**只读** Solana **finalized** 扫链服务，同一仓库两个进程：`cmd/sol-scanner` 只扫块并投递区块队列；`cmd/sol-processor` 消费、解析、一块一事务落库，再按发布游标发送下游交易 Kafka 消息（至少一次，key=签名）。不做回滚 topic、不回补历史、不判断充值/提现业务语义。

当前处于分阶段规格开发阶段：严格配置、PostgreSQL migration、双进程主循环、解析、provider、Kafka 与装配按 Plan 逐步落地；**PLAN-028 总体验收完成前不得声称已上线或可生产运行**。不要根据空目录、schema 或规划文档推断功能已经实现。

## 技术栈与标准命令

| 范围 | 当前选择 |
| --- | --- |
| Go | `go.mod` 声明 Go 1.26.0；使用标准库 `context`、`log/slog` |
| 链访问 | `github.com/Stone-afk/chain-api-go` **已发布版本**；禁止 `replace`；经 `internal/scanner/provider` 访问 |
| 数据库 | PostgreSQL、GORM；E2E Compose 使用 PostgreSQL 14.14 |
| 配置 | `gopkg.in/yaml.v3`，严格 KnownFields 与显式环境变量展开 |
| Kafka | `confluent-kafka-go`（需 `CGO_ENABLED=1`） |
| 装配 | 命令入口逐依赖调用 `InitX`：共享 `internal/ioc`（DB）+ `internal/ioc/scanner` / `internal/ioc/processor`；没有 Wire、容器对象或 `init` 注册 |

```sh
go test ./...                  # 不访问真实数据库、Kafka 或主网；Kafka 相关包需 CGO_ENABLED=1
go vet ./...
go build ./...
gofmt -l .
make e2e                       # 专用 Docker PostgreSQL（及 Kafka），退出时清理容器与 volume
make e2e E2E_RUN='^(TestMigrationsUpDownStatus|TestSchemaGate)$'
go run ./cmd/migrate up --config ./config/config.local.yaml
go run ./cmd/migrate status --config ./config/config.local.yaml
go run ./cmd/migrate down --config ./config/config.local.yaml
# 两个进程入口（需 CGO；PLAN-028 总体验收完成前不要当成已上线）：
CGO_ENABLED=1 go run ./cmd/sol-scanner --config ./config/config.local.yaml
CGO_ENABLED=1 go run ./cmd/sol-processor --config ./config/config.local.yaml
```

`make e2e` 会创建并清理名为 `sol-scanner-service-e2e` 的专用 Compose 项目。先确认 Docker 与 `docker compose` 可用，再运行 E2E。没有 `make lint`、通用 `serve` 子命令；不要在文档或交付说明中虚构这些入口。

## 分层与目录责任

- `cmd/sol-scanner/`：扫块进程入口；读取扫块配置，调用 `ioc.InitDB` 与 `internal/ioc/scanner` 的 `InitX`，持有资源至信号并显式关闭。只扫块并投递区块队列，不解析、不写业务表、不发下游交易 topic。
- `cmd/sol-processor/`：处理进程入口；读取处理配置，调用 `ioc.InitDB` 与 `internal/ioc/processor` 的 `InitX`。消费区块队列、解析、落库，再按发布游标发下游交易；不访问 RPC。
- `cmd/migrate/`：独立可执行入口；解析 `up|down|status`、只读取迁移所需数据库配置、运行 migration 并关闭连接。不把迁移塞进服务启动路径。
- `internal/config/`：严格 YAML、环境变量展开和字段校验。扫块/处理服务配置与迁移配置有不同边界；迁移不依赖 RPC/Kafka 变量；处理配置加载不打开 RPC。
- `internal/domain/`：纯 Go 领域模型与编码常量，不引入 GORM/配置/网络依赖。
- `internal/scanner/`：扫块主循环——先 `FinalizedSlot()`，再按 `stepSlots`/`slotsPerWorker` 分段 `getBlocks`/`getBlock` 并投递；不解析、不写业务表、不发下游交易 topic。
- `internal/scanner/provider/`：封装 chain-api-go；超时、重试、错误分类与脱敏；不解析转账业务。
- `internal/processor/`：写协程按 slot 重排、连续性校验、一块一事务落库与停扫回退；不访问 RPC。
- `internal/processor/parser/`：区块 → `BlockBatch` 纯函数；不访问网络和数据库。
- `internal/event/block/`：区块队列消息契约（`kind=block|skipped`）；扫块生产、处理消费。
- `internal/event/transaction/`：下游交易 Kafka 消息契约、`FromBlockBatch` 映射，以及按发布游标读库发送的 `Producer`；不访问 RPC。对齐 `notification-platform/internal/event/<领域>/`。
- `internal/repository/`：面向领域的仓储接口与 entity → domain 转换，隐藏 GORM 和 SQL。
- `internal/repository/postgre/`：GORM entity、DAO 接口与 SQL 实现；不实现领域 ScanRepository，不向上泄露 `*gorm.DB`。
- `internal/pkg/postgre/`：服务与迁移生产入口复用的 PostgreSQL 连接、schema gate 和 migration runner；不要放单一业务规则。
- `internal/pkg/mqx/`、`internal/pkg/alert/`：Kafka 批量生产者与运维告警；告警与业务消息无关。
- `internal/ioc/`：共享 DB 的 `InitDB`/`CloseDB`；不得 import scanner/provider/processor/parser。
- `internal/ioc/scanner/`：扫块进程 `InitX`（可依赖 provider）。
- `internal/ioc/processor/`：处理进程 `InitX`（不得依赖 provider / chain-api-go）。
- `internal/test/integration/`：所有真实 PostgreSQL、Kafka、主网或其他外部组件测试的归属地，通过 `e2e` build tag 与 `make e2e`（及显式 live 开关）运行。
- `scripts/`：统一存放工程脚本和测试基础设施，按职责组织子目录。
- `scripts/postgre/`：schema 的唯一权威；六位版本号的 `.up.sql`/`.down.sql` 成对出现。GORM entity 是映射，不是迁移来源。

依赖方向保持：

```text
cmd/sol-scanner   → ioc + ioc/scanner → {scanner, scanner/provider, event/block, repository, mqx, alert, pkg/postgre} → domain
cmd/sol-processor → ioc + ioc/processor → {processor, processor/parser, event/block, event/transaction, repository, mqx, alert, pkg/postgre} → domain
scanner → provider, event/block, ScanCursor/Halt 仓储, alert, domain
processor → parser, event/block, ScanRepository, alert, domain
event/transaction → PublishRepository, mqx, domain
parser → domain
repository → domain, postgre(DAO)
repository/postgre → entities, gorm（不依赖 repository 接口；不承载 domain 转换）
domain → (stdlib only)
```

`internal/pkg` 不是兜底目录：新公共能力必须至少有两个真实生产调用方、职责稳定，并证明不属于现有单一模块。

## 修改前后的检查清单

| 改动 | 先看 | 必须同步检查 |
| --- | --- | --- |
| 配置 | `internal/config`、示例配置、`.gitignore` | 字段路径校验、`${VAR}` 只在引用时必需、扫块/处理/迁移加载边界、错误不含 URL/DSN/密钥 |
| 服务启动或 IoC | `cmd/sol-scanner`、`cmd/sol-processor`、`internal/ioc`、`ioc/scanner`、`ioc/processor`、扫块/处理 Preflight | 创世哈希、`startSlot`、未处理 halt、schema gate、信号退出、停扫后两进程均不退出；处理 deps 无 provider |
| migration | `scripts/postgre`、`internal/pkg/postgre` | 成对文件、版本与名称、advisory lock、事务 Up/Down、gate 身份 |
| entity/DAO | SQL、`internal/repository`（转换）、`internal/repository/postgre`（entity/DAO） | 列类型/NULL、主键与 UNIQUE、一块一事务、checkpoint 条件更新；postgre 不实现 ScanRepository |
| 解析 | `internal/processor/parser`、夹具、jsonParsed 对照 | 范围内指令、内层指令、失败交易无 transfer、Token-2022 余额变化 |
| 扫块投递 | `internal/scanner`、design §3.1 | 先 `FinalizedSlot()`，再分段 `getBlocks`；整步失败不推进投递游标；大于 finalized 的 slot 不发 skipped |
| 落库连续性 | `internal/processor`、design §3.2 / FR-12 | 处理侧校验连续性；skipped 只推进应用位置、不改 checkpoint；无 checkpoint 时第一块前必须已是 skipped；停扫回退投递游标与队列位点 |
| 发布/Kafka | `event/block`、`event/transaction`、`mqx` | 区块队列至少一次；下游整块回执后才推进发布游标、重发逐字节相同、key=签名、Flush 毫秒超时 |
| 告警 | `internal/pkg/alert` | 先写 halt 再 `SendAlert`；与 Kafka 业务消息无关 |
| 新依赖 / chain-api-go | `go.mod`、plan 批准项 | 禁止 `replace`；只用已发布版本；新依赖须批准 |
| 新的共享包 | 现有调用方和模块边界 | 至少两个真实生产调用方；不建空壳 |

Go 指针硬约束：新增或修改的小字段、小结构体及其字段、参数、返回值、构造传参使用值语义；只有方法很多的实现对象或依赖很多、负责复杂生命周期的 app/server 类对象允许使用指针。不要为此批量改写现有指针；未来触及现有实现时必须保留数据库 `NULL` 语义和第三方 API 契约，若与硬约束冲突则说明事实并交由用户决定。

协议常量：链上指令判别符、固定线格式宽度，以及生产代码里用来分支的协议码，必须是所属包的命名常量；测试构造受支持输入时复用它们。细则见 `constitution.md` 第八条。

禁止傻瓜式防御性 nil guard：业务方法里不得写 `if p == nil || p.dep == nil { return }` 这类早退。依赖有效性由构造/装配保证；构造参数校验（如 `NewX(nil)`）可以保留。

## 测试先行与验证证据

新功能或 Bug 修复遵守 `constitution.md` 的 Red → Green → Refactor：先写能因目标行为缺失而失败的测试，再写最小实现，最后整理。单元和集成测试有并列输入时优先表驱动；涉及状态、迁移和生命周期可用更清晰的专门用例。不要通过改弱断言、跳过测试或环境失败伪造 Red/Green。

普通测试必须离线可运行；真实组件测试只放 `internal/test/integration` 并使用 `//go:build e2e`。E2E 使用专用 Compose 环境，成功或失败都清理项目容器和 volume；不得重置共享/生产数据库。主网只读验收使用 gitignore 的 `config/config.local.yaml`（可含 `${INFURA_API_KEY}`），不得把密钥写入受控文件或证据。测试应断言 slot、签名、transfer、余额变化、checkpoint/游标、迁移历史与错误语义，而不只看命令退出码。无法运行的检查需说明具体原因和剩余范围。

## 密钥与本地配置

- 仓库只提交 `config/config.example.yaml`，RPC/DSN 只用 `${INFURA_API_KEY}`、`${POSTGRES_DSN}` 等占位符。
- 真实配置写在 `config/config.local.yaml`；该路径已被 `.gitignore` 忽略，不得提交。
- 日志、错误、测试输出不得包含完整 URL、DSN 密码、API key、Kafka 凭据或节点原始响应体。

## 协作与交付

- 保留用户已有修改；当前工作树可能有未提交改动，先看 `git status`，不要重置、清理或覆盖无关文件。
- 文档默认中文；用户要求中英双语时双语交付。不要把参考仓库的 EVM 重组/WS、支付钱包通知、pgx 共存或 FlushDB 规则机械移植到本项目。
- Git 提交、推送、PR 以用户当前授权为准；只纳入本任务文件，提交信息优先用 `<type>(<scope>): <subject>`。
- 收尾写明实际修改、执行的验证与未验证范围。修改 `constitution.md` 时同步核对本文件、README、已批准规格和真实实现。
