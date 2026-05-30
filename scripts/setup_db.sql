-- Wallet Service — DB Setup Script
-- Run once before starting the service for the first time:
--   psql $DATABASE_URL -f scripts/setup_db.sql

CREATE TABLE IF NOT EXISTS wallets (
    wallet_id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id TEXT          NOT NULL,
    balance     NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK (balance >= 100),
    version     INT           NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_wallets_customer ON wallets(customer_id);

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'money_movement_type') THEN
        CREATE TYPE money_movement_type AS ENUM ('TOPUP', 'DEDUCT');
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS wallet_transactions (
    transaction_id  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wallet_id       UUID                NOT NULL REFERENCES wallets(wallet_id),
    type            money_movement_type NOT NULL,
    amount          NUMERIC(12,2)       NOT NULL CHECK (amount > 0),
    reference_id    TEXT,
    idempotency_key TEXT,
    created_at      TIMESTAMPTZ         NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_txn_wallet_id ON wallet_transactions(wallet_id);

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'deduction_outcome') THEN
        CREATE TYPE deduction_outcome AS ENUM ('SUCCESS', 'INSUFFICIENT_BALANCE');
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS deduction_idempotency (
    wallet_id        UUID              NOT NULL REFERENCES wallets(wallet_id),
    idempotency_key  TEXT              NOT NULL,
    requested_amount NUMERIC(12,2)     NOT NULL,
    outcome          deduction_outcome NOT NULL,
    transaction_id   UUID,
    balance_after    NUMERIC(12,2),
    created_at       TIMESTAMPTZ       NOT NULL DEFAULT now(),
    PRIMARY KEY (wallet_id, idempotency_key)
);

