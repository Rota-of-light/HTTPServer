-- name: GetRefreshByToken :one
SELECT * FROM refresh_tokens
WHERE token = $1
AND revoked_at IS NULL;