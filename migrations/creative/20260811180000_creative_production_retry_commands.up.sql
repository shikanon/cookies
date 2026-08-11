ALTER TABLE creative_edit_render_jobs
  ADD COLUMN retry_idempotency_key VARCHAR(255) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER retry_of,
  ADD COLUMN retry_request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER retry_idempotency_key,
  ADD UNIQUE KEY uq_creative_edit_render_retry_key (organization_id, project_id, retry_idempotency_key);

CREATE TABLE creative_production_retry_commands (
  organization_id VARCHAR(64) NOT NULL,
  project_id VARCHAR(64) NOT NULL,
  idempotency_key VARCHAR(255) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  previous_source VARCHAR(32) NOT NULL,
  previous_run_id VARCHAR(96) NOT NULL,
  status ENUM('pending','completed') NOT NULL,
  new_source VARCHAR(32) NULL,
  new_run_id VARCHAR(96) NULL,
  actor_id VARCHAR(96) NOT NULL,
  actor_kind VARCHAR(32) NOT NULL,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  completed_at DATETIME(6) NULL,
  PRIMARY KEY (organization_id, project_id, idempotency_key),
  KEY idx_creative_production_retry_previous (organization_id, project_id, previous_source, previous_run_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
