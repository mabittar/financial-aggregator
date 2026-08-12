-- 000001_create_users_table.up.sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY DEFAULT uuidv7(),
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  display_name TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);


-- Create indexes for frequently queried columns
CREATE INDEX idx_users_id ON users(id);
CREATE INDEX idx_email_id ON users(email);