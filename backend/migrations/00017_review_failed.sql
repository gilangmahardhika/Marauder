-- +goose Up
ALTER TABLE attempts DROP CONSTRAINT attempts_review_verdict_check;
ALTER TABLE attempts ADD CONSTRAINT attempts_review_verdict_check
  CHECK (review_verdict IN ('recommend_approve', 'request_changes', 'failed'));

-- +goose Down
UPDATE attempts SET review_verdict = NULL WHERE review_verdict = 'failed';
ALTER TABLE attempts DROP CONSTRAINT attempts_review_verdict_check;
ALTER TABLE attempts ADD CONSTRAINT attempts_review_verdict_check
  CHECK (review_verdict IN ('recommend_approve', 'request_changes'));
