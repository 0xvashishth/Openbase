-- Every API-key-authenticated data request looks up its key by hash; without
-- an index that is a sequential scan on the hottest path in the system.
-- UNIQUE also closes the (previously possible) duplicate-hash hole.
CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);
