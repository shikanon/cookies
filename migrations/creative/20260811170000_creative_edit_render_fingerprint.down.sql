ALTER TABLE creative_edit_render_jobs
  DROP KEY idx_creative_edit_render_reuse,
  DROP COLUMN renderer_fingerprint;
