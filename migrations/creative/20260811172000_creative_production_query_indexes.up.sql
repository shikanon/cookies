CREATE INDEX idx_creative_render_production_query
  ON creative_render_jobs (organization_id, project_id, status, created_at, id);

CREATE INDEX idx_creative_edit_render_production_query
  ON creative_edit_render_jobs (organization_id, project_id, status, created_at, edit_render_job_id);
