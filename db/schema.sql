-- Accounting application schema.
--
-- Monetary amounts are stored as BIGINT in the minor unit of whatever
-- currency they're posted in (e.g. cents for a currency with 2 decimal
-- places) to avoid floating point rounding errors. A positive amount is
-- a debit, a negative amount is a credit; the entries of a transaction
-- must always sum to zero within each currency (double-entry
-- bookkeeping) — amounts in different currencies are never summed
-- together directly.

-- position orders accounts for display among their siblings — other
-- accounts with the same parent_id (or, for parent_id IS NULL, other root
-- accounts) — lowest first. It's assigned automatically on creation
-- (see AccountStore.Create) and changed only by explicitly moving an
-- account up or down among its siblings (see AccountStore.Move); it's
-- otherwise meaningless to compare across different parents.
CREATE TABLE accounts (
    id        BIGSERIAL PRIMARY KEY,
    name      TEXT NOT NULL,
    code      TEXT,
    parent_id BIGINT REFERENCES accounts (id) ON DELETE RESTRICT,
    position  BIGINT NOT NULL DEFAULT 0,
    CONSTRAINT accounts_code_unique UNIQUE (code)
);

CREATE INDEX idx_accounts_parent_id ON accounts (parent_id);

-- A currency (or, more generally, a commodity): a unit that transaction
-- entries can be posted in, along with how amounts in it should be
-- formatted for display. decimal_places governs both how amounts are
-- stored (as an integer number of the currency's minor unit, e.g. cents)
-- and how many decimal digits to render.
CREATE TABLE currencies (
    id                  BIGSERIAL PRIMARY KEY,
    name                TEXT NOT NULL,
    symbol_before       BOOLEAN NOT NULL DEFAULT FALSE,
    symbol_space        BOOLEAN NOT NULL DEFAULT TRUE,
    thousands_separator TEXT NOT NULL DEFAULT '',
    decimal_separator   TEXT NOT NULL DEFAULT '.',
    decimal_places      SMALLINT NOT NULL DEFAULT 2 CHECK (decimal_places >= 0),
    isin                TEXT,
    CONSTRAINT currencies_name_unique UNIQUE (name),
    CONSTRAINT currencies_isin_unique UNIQUE (isin)
);

-- A directly-observed exchange rate: one unit of base_currency_id was
-- worth `rate` units of quote_currency_id, as of a specific instant
-- (as_of). Unlike transaction amounts, rate is an approximate market
-- price rather than an exact ledger quantity — it's not tied to either
-- currency's decimal_places, so the "no floats" rule for monetary
-- amounts doesn't apply here; DOUBLE PRECISION is deliberate. Querying a
-- rate that doesn't exactly match a stored observation (see
-- CurrencyPriceStore.RateAt) linearly interpolates between the nearest
-- observations before/after the requested instant, and chains through
-- intermediate currencies if there's no data directly relating the two
-- requested currencies.
CREATE TABLE currency_prices (
    id                BIGSERIAL PRIMARY KEY,
    base_currency_id  BIGINT NOT NULL REFERENCES currencies (id) ON DELETE RESTRICT,
    quote_currency_id BIGINT NOT NULL REFERENCES currencies (id) ON DELETE RESTRICT,
    rate              DOUBLE PRECISION NOT NULL CHECK (rate > 0),
    as_of             TIMESTAMPTZ NOT NULL,
    CONSTRAINT currency_prices_distinct_currencies CHECK (base_currency_id <> quote_currency_id),
    CONSTRAINT currency_prices_unique_observation UNIQUE (base_currency_id, quote_currency_id, as_of)
);

CREATE INDEX idx_currency_prices_base_quote ON currency_prices (base_currency_id, quote_currency_id);
CREATE INDEX idx_currency_prices_quote_base ON currency_prices (quote_currency_id, base_currency_id);

CREATE TABLE transactions (
    id          BIGSERIAL PRIMARY KEY,
    "timestamp" TIMESTAMPTZ NOT NULL,
    description TEXT NOT NULL
);

CREATE TABLE transaction_entries (
    id             BIGSERIAL PRIMARY KEY,
    transaction_id BIGINT NOT NULL REFERENCES transactions (id) ON DELETE CASCADE,
    account_id     BIGINT NOT NULL REFERENCES accounts (id) ON DELETE RESTRICT,
    amount         BIGINT NOT NULL,
    currency_id    BIGINT NOT NULL REFERENCES currencies (id) ON DELETE RESTRICT
);

CREATE INDEX idx_transaction_entries_transaction_id ON transaction_entries (transaction_id);
CREATE INDEX idx_transaction_entries_account_id ON transaction_entries (account_id);
CREATE INDEX idx_transaction_entries_currency_id ON transaction_entries (currency_id);

-- Defense in depth: the API validates that entries sum to zero (per
-- currency) before writing, but this deferred constraint trigger enforces
-- the same invariant at the database level for any transaction_entries
-- change, regardless of what wrote it.
CREATE OR REPLACE FUNCTION check_transaction_balance() RETURNS trigger AS $$
DECLARE
    affected_transaction_id BIGINT;
    unbalanced_currency_id  BIGINT;
    unbalanced_amount       BIGINT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        affected_transaction_id := OLD.transaction_id;
    ELSE
        affected_transaction_id := NEW.transaction_id;
    END IF;

    SELECT currency_id, SUM(amount)
    INTO unbalanced_currency_id, unbalanced_amount
    FROM transaction_entries
    WHERE transaction_id = affected_transaction_id
    GROUP BY currency_id
    HAVING SUM(amount) <> 0
    LIMIT 1;

    IF unbalanced_currency_id IS NOT NULL THEN
        RAISE EXCEPTION 'transaction % entries in currency % do not sum to zero (got %)',
            affected_transaction_id, unbalanced_currency_id, unbalanced_amount;
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER trg_check_transaction_balance
    AFTER INSERT OR UPDATE OR DELETE ON transaction_entries
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION check_transaction_balance();
