CREATE TYPE deduction_outcome AS ENUM ('SUCCESS', 'INSUFFICIENT_BALANCE');

CREATE TABLE deduction_idempotency (
    wallet_id        UUID              NOT NULL REFERENCES wallets(wallet_id),
    idempotency_key  TEXT              NOT NULL,
    requested_amount NUMERIC(12,2)     NOT NULL,
    outcome          deduction_outcome NOT NULL,
    transaction_id   UUID,
    balance_after    NUMERIC(12,2),
    created_at       TIMESTAMPTZ       NOT NULL DEFAULT now(),
    PRIMARY KEY (wallet_id, idempotency_key)
);

