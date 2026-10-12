-- +goose Up
ALTER TABLE events ADD COLUMN seq BIGSERIAL;
CREATE INDEX events_card_seq ON events (card_id, ts, seq);

-- +goose Down
DROP INDEX events_card_seq;
ALTER TABLE events DROP COLUMN seq;
