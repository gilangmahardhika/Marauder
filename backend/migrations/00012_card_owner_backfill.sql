-- +goose Up
-- Cards created before 00009 have no owner. Give each the creator its
-- requirements share, the same rule create_card applies (RFC-054).
UPDATE cards c SET owner_id = s.creator
FROM (
  SELECT c2.id, min(NULLIF(r.created_by, '')) AS creator
  FROM cards c2
  CROSS JOIN LATERAL jsonb_array_elements_text(c2.requirement_ids_json::jsonb) AS req(id)
  LEFT JOIN requirements r ON r.project_id = c2.project_id AND r.id = req.id
  WHERE c2.owner_id IS NULL
  GROUP BY c2.id
  HAVING count(DISTINCT NULLIF(r.created_by, '')) = 1 AND count(NULLIF(r.created_by, '')) = count(*)
) s
WHERE c.id = s.id AND c.owner_id IS NULL
  AND EXISTS (SELECT 1 FROM users u WHERE u.id = s.creator);

-- +goose Down
SELECT 1;
