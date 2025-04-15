-- name: CheckRevokeStatus :one
SELECT revoked_at FROM refresh_tokens
WHERE token = $1;