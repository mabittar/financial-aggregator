CREATE TABLE portfolios (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
-- Create indexes for frequently queried columns
CREATE INDEX idx_portfolios_id ON portfolios(id);
CREATE INDEX idx_portfolios_user_id ON portfolios(user_id);