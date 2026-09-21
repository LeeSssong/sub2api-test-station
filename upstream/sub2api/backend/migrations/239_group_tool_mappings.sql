-- Explicit administrator-owned mapping; never infer tools from group names/platforms.
CREATE TABLE IF NOT EXISTS group_tool_mappings (
 group_id bigint PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
 tool_ids jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(tool_ids) = 'array'),
 version bigint NOT NULL DEFAULT 1,
 effective_at timestamptz NOT NULL DEFAULT NOW(),
 updated_by bigint NOT NULL
);
CREATE TABLE IF NOT EXISTS group_tool_mapping_audit (
 id bigserial PRIMARY KEY,
 group_id bigint NOT NULL,
 tool_ids jsonb NOT NULL,
 version bigint NOT NULL,
 effective_at timestamptz NOT NULL,
 updated_by bigint NOT NULL
);
