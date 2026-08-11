CREATE INDEX idx_provider_jobs_production_query
  ON provider_jobs (organization_id, project_id, source_system, provider_status, created_at, id);
