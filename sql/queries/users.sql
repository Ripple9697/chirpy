-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email,hashed_password)
VALUES (
  gen_random_uuid(),
  NOW(),
  NOW(),
  $1,
  $2
)
RETURNING *;

-- name: DeleteUsers :exec
DELETE FROM users;

-- name: GetUser :one
SELECT * from users
where email = $1;

-- name: UpdateUserCreds :one
UPDATE users
set email = $2,hashed_password = $3
  where id = $1
  RETURNING *;

-- name: UpgradeUserChirpyRed :exec
UPDATE users
  SET is_chirpy_red = TRUE
  WHERE id = $1;
