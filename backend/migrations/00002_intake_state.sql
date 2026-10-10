-- +goose Up
CREATE TABLE intake_state (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  draft_pending BOOLEAN NOT NULL DEFAULT false,
  plan_error TEXT
);
INSERT INTO intake_state (id, draft_pending) VALUES (1, false);

-- +goose Down
DROP TABLE intake_state;
