-- name: GetUserByRefreshToken :one
SELECT users.* FROM users
INNER JOIN refresh_tokens
ON users.ID = refresh_tokens.user_id
WHERE refresh_tokens.token = $1
AND refresh_tokens.expires_at > NOW()
AND refresh_tokens.revoked_at IS NULL;