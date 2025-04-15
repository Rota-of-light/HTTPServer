-- name: RevokeRefresh :exec
UPDATE refresh_tokens
SET updated_at = $1, revoked_at = $1
WHERE token = $2 AND revoked_at IS NULL;