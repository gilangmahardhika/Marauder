-- +goose Up
CREATE INDEX events_kind_ts ON events (kind, ts);

-- +goose Down
DROP INDEX events_kind_ts;
