-- 000002_create_monthly_statements.up.sql
CREATE TABLE IF NOT EXISTS monthly_statements (
  id UUID PRIMARY KEY DEFAULT uuidv7(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  portfolio_name TEXT NOT NULL,
  reference_date DATE NOT NULL,
  ingest_key TEXT NOT NULL,
  raw_payload JSONB NOT NULL,
  parsed_payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  source TEXT NOT NULL,
  submitted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (user_id, portfolio_name, reference_date, ingest_key)
);

-- Create indexes for frequently queried columns
CREATE INDEX idx_monthly_statements_id ON monthly_statements(id);
CREATE INDEX idx_monthly_statements_user_id ON monthly_statements(user_id);
