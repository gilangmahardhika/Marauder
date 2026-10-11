-- +goose Up
ALTER TABLE runs ADD COLUMN card_id TEXT REFERENCES cards(id) ON DELETE CASCADE;
CREATE UNIQUE INDEX runs_open_card ON runs(card_id) WHERE card_id IS NOT NULL AND processed_at IS NULL;

-- +goose Down
DROP INDEX runs_open_card;
ALTER TABLE runs DROP COLUMN card_id;
