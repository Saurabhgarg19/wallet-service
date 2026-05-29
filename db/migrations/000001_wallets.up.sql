CREATE TABLE wallets (
    wallet_id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id TEXT          NOT NULL,
    balance     NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK (balance >= 0),
    version     INT           NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_wallets_customer ON wallets(customer_id);

