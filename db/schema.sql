-- Accounting application schema.
--
-- Monetary values are stored as BIGINT in minor currency units (e.g. cents)
-- to avoid floating point rounding errors. A positive value is a debit,
-- a negative value is a credit; the entries of a transaction must always
-- sum to zero (double-entry bookkeeping).

CREATE TABLE accounts (
    id        BIGSERIAL PRIMARY KEY,
    name      TEXT NOT NULL,
    code      TEXT,
    parent_id BIGINT REFERENCES accounts (id) ON DELETE RESTRICT,
    CONSTRAINT accounts_code_unique UNIQUE (code)
);

CREATE INDEX idx_accounts_parent_id ON accounts (parent_id);

CREATE TABLE transactions (
    id          BIGSERIAL PRIMARY KEY,
    "timestamp" TIMESTAMPTZ NOT NULL,
    description TEXT NOT NULL
);

CREATE TABLE transaction_entries (
    id             BIGSERIAL PRIMARY KEY,
    transaction_id BIGINT NOT NULL REFERENCES transactions (id) ON DELETE CASCADE,
    account_id     BIGINT NOT NULL REFERENCES accounts (id) ON DELETE RESTRICT,
    value          BIGINT NOT NULL
);

CREATE INDEX idx_transaction_entries_transaction_id ON transaction_entries (transaction_id);
CREATE INDEX idx_transaction_entries_account_id ON transaction_entries (account_id);

-- Defense in depth: the API validates that entries sum to zero before
-- writing, but this deferred constraint trigger enforces the same
-- invariant at the database level for any transaction_entries change,
-- regardless of what wrote it.
CREATE OR REPLACE FUNCTION check_transaction_balance() RETURNS trigger AS $$
DECLARE
    affected_transaction_id BIGINT;
    balance                 BIGINT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        affected_transaction_id := OLD.transaction_id;
    ELSE
        affected_transaction_id := NEW.transaction_id;
    END IF;

    SELECT COALESCE(SUM(value), 0)
    INTO balance
    FROM transaction_entries
    WHERE transaction_id = affected_transaction_id;

    IF balance <> 0 THEN
        RAISE EXCEPTION 'transaction % entries do not sum to zero (got %)',
            affected_transaction_id, balance;
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER trg_check_transaction_balance
    AFTER INSERT OR UPDATE OR DELETE ON transaction_entries
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION check_transaction_balance();
