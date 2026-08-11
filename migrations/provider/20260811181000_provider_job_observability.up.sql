CREATE TABLE provider_job_usage (
  provider_job_id VARCHAR(96) NOT NULL,
  organization_id VARCHAR(96) NOT NULL,
  project_id VARCHAR(96) NOT NULL,
  unit_kind VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  requested_units BIGINT UNSIGNED NOT NULL,
  billed_units BIGINT UNSIGNED NOT NULL DEFAULT 0,
  currency CHAR(3) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  actual_cost_minor BIGINT UNSIGNED NULL,
  measured_at DATETIME(6) NOT NULL,
  PRIMARY KEY (provider_job_id),
  KEY idx_provider_usage_project (organization_id, project_id, measured_at),
  CONSTRAINT fk_provider_usage_job FOREIGN KEY (provider_job_id) REFERENCES provider_jobs(id)
) ENGINE=InnoDB;

CREATE TABLE provider_job_events (
  provider_job_id VARCHAR(96) NOT NULL,
  organization_id VARCHAR(96) NOT NULL,
  project_id VARCHAR(96) NOT NULL,
  ordinal INT UNSIGNED NOT NULL,
  stage VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  safe_message VARCHAR(512) NOT NULL,
  error_code VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL,
  occurred_at DATETIME(6) NOT NULL,
  PRIMARY KEY (provider_job_id, ordinal),
  KEY idx_provider_events_project (organization_id, project_id, provider_job_id, ordinal),
  CONSTRAINT fk_provider_events_job FOREIGN KEY (provider_job_id) REFERENCES provider_jobs(id)
) ENGINE=InnoDB;
