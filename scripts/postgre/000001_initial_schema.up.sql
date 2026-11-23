-- 区块：每天约 43 万行
CREATE TABLE sol_blocks (
    slot               BIGINT PRIMARY KEY,
    blockhash          VARCHAR(44) NOT NULL,
    parent_slot        BIGINT NOT NULL,
    previous_blockhash VARCHAR(44) NOT NULL,
    block_height       BIGINT,
    block_time         BIGINT,
    transaction_count  INTEGER NOT NULL,
    created_at         BIGINT NOT NULL DEFAULT FLOOR(EXTRACT(EPOCH FROM CURRENT_TIMESTAMP))::BIGINT
);

-- 交易：每天约 2 亿行
CREATE TABLE sol_transactions (
    id        BIGSERIAL   PRIMARY KEY,
    slot      BIGINT      NOT NULL,
    tx_index  INTEGER     NOT NULL,          -- 交易在区块内的序号
    signature VARCHAR(88) NOT NULL,
    status    SMALLINT    NOT NULL,          -- 1 成功，2 失败
    version   SMALLINT    NOT NULL,          -- -1 legacy，0，1
    fee       BIGINT      NOT NULL,          -- lamports
    fee_payer VARCHAR(44) NOT NULL,
    UNIQUE (slot, tx_index)
);

-- 转账指令：每天约 4 亿行
CREATE TABLE sol_transfers (
    id           BIGSERIAL     PRIMARY KEY,
    slot         BIGINT        NOT NULL,
    tx_index     INTEGER       NOT NULL,
    ix_index     SMALLINT      NOT NULL,     -- 顶层指令序号
    inner_index  SMALLINT      NOT NULL,     -- 内层指令序号；顶层指令为 -1
    program      SMALLINT      NOT NULL,     -- 1 system，2 spl-token，3 token-2022
    kind         SMALLINT      NOT NULL,     -- 指令类型编码，见下表
    mint         VARCHAR(44)   NOT NULL DEFAULT '',   -- SOL 为空
    decimals     SMALLINT,
    from_account VARCHAR(44)   NOT NULL DEFAULT '',   -- mint 类指令为空
    from_owner   VARCHAR(44)   NOT NULL DEFAULT '',
    to_account   VARCHAR(44)   NOT NULL DEFAULT '',   -- burn 类指令为空
    to_owner     VARCHAR(44)   NOT NULL DEFAULT '',
    amount       NUMERIC(20,0) NOT NULL,
    fee          NUMERIC(20,0),                       -- 仅 transferCheckedWithFee 有值
    UNIQUE (slot, tx_index, ix_index, inner_index)
);

-- Token-2022 账户余额变化：每天约 7000 万行
CREATE TABLE sol_token_balance_changes (
    id            BIGSERIAL     PRIMARY KEY,
    slot          BIGINT        NOT NULL,
    tx_index      INTEGER       NOT NULL,
    account_index SMALLINT      NOT NULL,    -- 账户在交易账户列表里的序号
    program       SMALLINT      NOT NULL,    -- 本期恒为 3
    account       VARCHAR(44)   NOT NULL,
    owner         VARCHAR(44)   NOT NULL DEFAULT '',
    mint          VARCHAR(44)   NOT NULL,
    decimals      SMALLINT      NOT NULL,
    pre_amount    NUMERIC(20,0) NOT NULL,    -- 交易前不存在则为 0
    post_amount   NUMERIC(20,0) NOT NULL,    -- 交易后不存在则为 0
    UNIQUE (slot, tx_index, account_index)
);

-- 扫描进度：只有一行
CREATE TABLE sol_scan_checkpoint (
    id              SMALLINT PRIMARY KEY CHECK (id = 1),
    last_block_slot BIGINT      NOT NULL,
    last_blockhash  VARCHAR(44) NOT NULL,
    updated_at      BIGINT      NOT NULL
);

-- 本服务的发送位置：哪些已入库区块已经成功交给 Kafka。
-- 不是 Kafka 的消费位点。本期只有一个 topic，所以只有一行。
CREATE TABLE sol_publish_cursor (
    id             SMALLINT PRIMARY KEY CHECK (id = 1),
    topic          VARCHAR(255) NOT NULL,
    published_slot BIGINT NOT NULL,
    updated_at     BIGINT NOT NULL
);

-- 停扫记录
CREATE TABLE sol_scan_halts (
    id          BIGSERIAL PRIMARY KEY,
    reason      VARCHAR(32) NOT NULL,   -- HASH_MISMATCH / PARENT_BEHIND / GAP_UNRESOLVED / DECODE_ERROR
    slot        BIGINT NOT NULL,
    detail      TEXT   NOT NULL,        -- 期望值与实际值
    created_at  BIGINT NOT NULL,
    resolved_at BIGINT                  -- 人工处理后填写；为空表示未处理
);
