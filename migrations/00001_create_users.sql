-- +goose Up
CREATE SCHEMA cloud_file_storage;

CREATE TABLE IF NOT EXISTS cloud_file_storage.users (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  username      VARCHAR(255) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL
);

-- +goose Down
DROP TABLE cloud_file_storage.users;
DROP SCHEMA cloud_file_storage;
