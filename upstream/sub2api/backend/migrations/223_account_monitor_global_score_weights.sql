CREATE TABLE IF NOT EXISTS account_monitor_global_score_weights (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    cost_weight SMALLINT NOT NULL CHECK (cost_weight >= 0),
    success_weight SMALLINT NOT NULL CHECK (success_weight >= 0),
    ttft_weight SMALLINT NOT NULL CHECK (ttft_weight >= 0),
    latency_weight SMALLINT NOT NULL CHECK (latency_weight >= 0),
    updated_by BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (cost_weight + success_weight + ttft_weight + latency_weight = 100)
);
