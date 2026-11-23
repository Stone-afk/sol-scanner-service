-- 扫块服务的投递位置：整步成功交给区块队列后的最大 slot，含被跳过的空 slot。
-- 不是链上 checkpoint，也不是下游发布游标。
CREATE TABLE sol_scan_cursor (
    id          SMALLINT PRIMARY KEY CHECK (id = 1),
    handed_slot BIGINT NOT NULL,
    updated_at  BIGINT NOT NULL
);
