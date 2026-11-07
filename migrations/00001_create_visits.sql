-- +goose Up
CREATE TABLE IF NOT EXISTS visits (
    id    int    PRIMARY KEY,
    count bigint NOT NULL
);

-- +goose Down
DROP TABLE visits;