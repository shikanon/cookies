ALTER TABLE creative_edit_render_jobs
  ADD COLUMN renderer_fingerprint CHAR(71) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER compiler_version;

UPDATE creative_edit_render_jobs
SET renderer_fingerprint = CONCAT('sha256:', SHA2(CONCAT(timeline_hash, ':', timeline_schema_version, ':', compiler_version, ':', kind), 256))
WHERE renderer_fingerprint IS NULL;

ALTER TABLE creative_edit_render_jobs
  MODIFY COLUMN renderer_fingerprint CHAR(71) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  ADD KEY idx_creative_edit_render_reuse (organization_id, project_id, renderer_fingerprint, kind, status);
