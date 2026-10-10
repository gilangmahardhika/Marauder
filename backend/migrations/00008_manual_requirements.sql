-- +goose Up
ALTER TABLE requirements DROP CONSTRAINT requirements_source_kind_check;
ALTER TABLE requirements ADD CONSTRAINT requirements_source_kind_check
  CHECK (source_kind IN ('prd', 'rfc', 'draft', 'manual'));
ALTER TABLE requirements ADD COLUMN created_by TEXT;

-- +goose Down
DELETE FROM requirements WHERE source_kind = 'manual';
ALTER TABLE requirements DROP COLUMN created_by;
ALTER TABLE requirements DROP CONSTRAINT requirements_source_kind_check;
ALTER TABLE requirements ADD CONSTRAINT requirements_source_kind_check
  CHECK (source_kind IN ('prd', 'rfc', 'draft'));
