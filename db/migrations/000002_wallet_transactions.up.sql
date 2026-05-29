CREATE TYPE money_movement_type AS ENUM ('TOPUP', 'DEDUCT');

CREATE TABLE wallet_transactions (
    transaction_id  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wallet_id       UUID                NOT NULL REFERENCES wallets(wallet_id),
    type            money_movement_type NOT NULL,
    amount          NUMERIC(12,2)       NOT NULL CHECK (amount > 0),
    reference_id    TEXT,
    idempotency_key TEXT,
    created_at      TIMESTAMPTZ         NOT NULL DEFAULT now()
);

CREATE INDEX idx_txn_wallet_id ON wallet_transactions(wallet_id);

