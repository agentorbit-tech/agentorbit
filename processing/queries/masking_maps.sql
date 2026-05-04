-- name: InsertSpanMaskingMap :exec
INSERT INTO span_masking_maps (span_id, mask_type, original_value, masked_value)
VALUES ($1, $2, $3, $4);

-- name: GetSpanMaskingMaps :many
SELECT m.id, m.span_id, m.mask_type, m.original_value, m.masked_value, m.created_at
FROM span_masking_maps m
JOIN spans s ON s.id = m.span_id
WHERE m.span_id = $1 AND s.organization_id = $2
ORDER BY m.created_at;
