-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION orchestrator_notify() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  card TEXT;
  project TEXT;
BEGIN
  IF TG_TABLE_NAME = 'cards' THEN
    card := NEW.id;
    project := NEW.project_id;
  ELSE
    card := NEW.card_id;
    project := COALESCE((SELECT c.project_id FROM cards c WHERE c.id = NEW.card_id), NEW.project_id);
  END IF;
  PERFORM pg_notify('orchestrator', json_build_object(
    't', TG_TABLE_NAME, 's', current_schema(), 'projectId', project, 'cardId', card)::text);
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION orchestrator_notify_user() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM pg_notify('orchestrator', json_build_object(
    't', TG_TABLE_NAME, 's', current_schema(), 'userId', NEW.user_id, 'id', NEW.id)::text);
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER events_notify AFTER INSERT ON events
  FOR EACH ROW EXECUTE FUNCTION orchestrator_notify();
CREATE TRIGGER cards_notify AFTER INSERT OR UPDATE ON cards
  FOR EACH ROW EXECUTE FUNCTION orchestrator_notify();
CREATE TRIGGER runs_notify AFTER INSERT OR UPDATE ON runs
  FOR EACH ROW EXECUTE FUNCTION orchestrator_notify();

CREATE INDEX events_seq ON events (seq);
CREATE INDEX events_ts ON events (ts);

CREATE TABLE notifications (
  id TEXT PRIMARY KEY,
  event_id TEXT NOT NULL,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  card_id TEXT REFERENCES cards(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  url TEXT NOT NULL,
  push BOOLEAN NOT NULL DEFAULT false,
  created_at TEXT NOT NULL,
  read_at TEXT,
  UNIQUE (event_id, user_id, kind)
);
CREATE INDEX notifications_user_created ON notifications (user_id, created_at);

CREATE TRIGGER notifications_notify AFTER INSERT ON notifications
  FOR EACH ROW EXECUTE FUNCTION orchestrator_notify_user();

CREATE TABLE push_subscriptions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  endpoint TEXT NOT NULL UNIQUE,
  p256dh TEXT NOT NULL,
  auth TEXT NOT NULL,
  failures INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);
CREATE INDEX push_subscriptions_user ON push_subscriptions (user_id);

CREATE TABLE notify_cursor (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  seq BIGINT NOT NULL
);
INSERT INTO notify_cursor (id, seq) SELECT 1, COALESCE(MAX(seq), 0) FROM events;

-- +goose Down
DROP TABLE notify_cursor;
DROP TABLE push_subscriptions;
DROP TRIGGER notifications_notify ON notifications;
DROP TABLE notifications;
DROP INDEX events_ts;
DROP INDEX events_seq;
DROP TRIGGER runs_notify ON runs;
DROP TRIGGER cards_notify ON cards;
DROP TRIGGER events_notify ON events;
DROP FUNCTION orchestrator_notify_user();
DROP FUNCTION orchestrator_notify();
