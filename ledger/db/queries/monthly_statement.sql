-- name: CreateMonthlyStatement :one
INSERT INTO monthly_statements (user_id, portfolio_name, reference_date, ingest_key, raw_payload, parsed_payload, status, source)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, ingest_key, status, submitted_at, source;

-- name: ListStatementsForPortfolio :many
SELECT id, ingest_key, status, submitted_at, updated_at, source
FROM monthly_statements
WHERE user_id = $1 AND portfolio_name = $2 AND reference_date = $3
ORDER BY submitted_at DESC;