CREATE TABLE IF NOT EXISTS wallets (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(255) UNIQUE NOT NULL,
    balance BIGINT NOT NULL DEFAULT 500,
    currency VARCHAR(3) NOT NULL DEFAULT 'PTS',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wallets_user_id ON wallets(user_id);

ALTER TABLE wallets DROP CONSTRAINT IF EXISTS balance_non_negative;
ALTER TABLE wallets ADD CONSTRAINT balance_non_negative CHECK (balance >= 0);