# sol-scanner-service

只读 Solana **finalized** 扫链服务：同一仓库两个进程——扫块投递区块队列，处理侧解析、一块一事务落库，再按发布游标发下游交易（至少一次，key=签名）。

不做回滚 topic、不回补 `startSlot` 之前的历史、不判断充值/提现业务语义。产品与数据契约、规格三件套在本地 `docs/superpowers/001-sol-scanner-v1/`（不随仓库公开；当前基线 `PLAN-v1.6.5`）。处理侧消费吞吐优化见 `docs/superpowers/002-processor-consume-throughput/`（链尖 L=300，与 001 A-04=150 并存）。

> 总体验收（PLAN-028）已通过；当前 Infura 环境下 A-04 吞吐门禁为 **blocked**，**不声称已上线或可生产运行**。高吞吐 RPC 下才可将 FA-04 标为 passed。

## 架构

```mermaid
flowchart TB
    RPC["Solana RPC<br/>HTTP · commitment=finalized"]
    subgraph SCAN["cmd/sol-scanner"]
        PROV["provider · chain-api-go"]
        STEP["主循环：FinalizedSlot()<br/>划一步 → 切成 n 段"]
        subgraph WORKERS["并行扫块协程 · n = stepSlots / slotsPerWorker"]
            W1["协程-1<br/>getBlocks → getBlock / skipped"]
            W2["协程-2"]
            WN["协程-n"]
        end
        JOIN["整步回执齐全后<br/>才推进投递游标"]
    end
    Q["Kafka 区块 topic<br/>key = slot · kind=block|skipped"]
    subgraph PROC["cmd/sol-processor"]
        WR["单写协程<br/>按 slot 重排 · 连续性 · 一块一事务"]
        PARSE["parser · 纯函数"]
        PUB["发布循环<br/>按发布游标读库"]
    end
    DB[("PostgreSQL")]
    TX["Kafka 下游交易 topic<br/>key = 签名"]
    DOWN["下游业务"]

    RPC --> PROV --> STEP
    STEP --> W1 & W2 & WN
    W1 & W2 & WN --> RPC
    W1 & W2 & WN --> JOIN --> Q
    Q --> WR
    WR --> PARSE
    WR --> DB
    DB --> PUB --> TX --> DOWN
```

| 进程 | 并发 | 做什么 | 不做什么 |
| --- | --- | --- | --- |
| `sol-scanner` | **多协程**：每步按 `slotsPerWorker` 扇出；示例 `stepSlots=20000` / `500` → 40 协程 | HTTP 拉 finalized 并投递 | 不解析、不写业务表、不发下游 |
| `sol-processor` | **单写协程**落库；发布循环另跑 | 消费、解析、落库、按游标发下游 | 不访问 RPC |

并行只在扫块侧。整步任一片段失败则游标不前进。区块 topic 是内部队列；交易 topic 给下游。

## 数据流

### 扫块一步

先 `FinalizedSlot()`，再划本步并 **扇出多协程**；整步成功才推进投递游标。

```mermaid
flowchart TD
    A["读投递游标；无则用 startSlot"] --> H{"未处理停扫?"}
    H -- 是 --> Z["等待，不开下一步"] --> H
    H -- 否 --> B["FinalizedSlot()"]
    B --> C{"游标之后还有 slot?"}
    C -- 否 --> Wait["等待 pollInterval"] --> B
    C -- 是 --> D["划一步左右端"]
    D --> E["切成 n 段<br/>n = ceil(步长 / slotsPerWorker)"]
    E --> F1["协程-1: getBlocks/getBlock"]
    E --> F2["协程-2: …"]
    E --> Fn["协程-n: …"]
    F1 & F2 & Fn --> G{"整步回执齐全?"}
    G -- 否 --> R["各段重投，游标不动"] --> F1
    G -- 是 --> U["投递游标推进到右端"] --> B
```

### 处理落库

单写协程按 slot 应用；skipped 只推进应用位置，不改 checkpoint。

```mermaid
flowchart TD
    A["读 checkpoint"] --> B["Poll 区块队列"]
    B --> P{"有新消息?"}
    P -- 空 poll --> L{"连续空 poll 且缺应用位置?"}
    L -- 是 --> X["停扫 GAP_UNRESOLVED"]
    L -- 否 --> W["等待"] --> B
    P -- slot 已覆盖 --> Dc["丢弃并 Commit"] --> B
    P -- 否则 --> Buf["入缓冲"] --> C{"缓冲有应用位置?"}
    C -- 否 --> B
    C -- 是 --> D{"skipped?"}
    D -- 是 --> E["应用位置+1 · Commit"] --> C
    D -- 否 --> F["解析"] --> I{"与 checkpoint 接得上?"}
    I -- 是 --> J["一块一事务落库 · Commit"] --> C
    I -- 否 --> X2["停扫，不提交"]
```

停扫后：不再落新块；已入库数据继续发布；扫块不开下一步；两进程不因停扫自行退出。

## 目录（摘要）

```text
cmd/sol-scanner|sol-processor|migrate
internal/
  scanner/ (+ provider)     # 扫块
  processor/ (+ parser)     # 落库与解析
  event/block|transaction   # 区块队列 / 下游消息
  repository/ (+ postgre)   # 仓储
  ioc/ (+ scanner|processor)# 装配 InitX
  config/ domain/ pkg/
scripts/postgre/            # 迁移 SQL 权威
```

依赖方向：`cmd → ioc → {scanner|processor|event|repository|…} → domain`。处理进程不得依赖 `provider` / `chain-api-go`。

## 快速开始

1. 复制 [`config/config.example.yaml`](config/config.example.yaml) 为 gitignore 的 `config/config.local.yaml`，填入 `${INFURA_API_KEY}` / `${POSTGRES_DSN}` 等（密钥勿提交）。
2. 迁移与进程（Kafka 相关需 `CGO_ENABLED=1`）：

```sh
go run ./cmd/migrate up --config ./config/config.local.yaml
CGO_ENABLED=1 go run ./cmd/sol-scanner --config ./config/config.local.yaml
CGO_ENABLED=1 go run ./cmd/sol-processor --config ./config/config.local.yaml
```

3. 测试：

```sh
go test ./...          # 离线；Kafka 包需 CGO_ENABLED=1
go vet ./...
go build ./...
gofmt -l .
make e2e               # 专用 Compose PostgreSQL + Kafka，退出清理
```

主网只读验收另需 `SOL_SCANNER_LIVE_E2E=1` 且 DSN 主机为本机；证据不得含完整 URL / DSN / API key。无 `make lint`、无通用 `serve`。

## 技术栈

| 项 | 选择 |
| --- | --- |
| Go | `go.mod` 声明版本 |
| 链访问 | 已发布 `chain-api-go`（禁止 `replace`） |
| DB | PostgreSQL + GORM；E2E 用 Compose PostgreSQL 14.14 |
| 配置 | `yaml.v3`，KnownFields + `${VAR}` 展开 |
| Kafka | `confluent-kafka-go`（CGO） |

更细的表结构、消息 JSON、停扫原因与验收口径见本地规格文档（`docs/`，已 gitignore）。
