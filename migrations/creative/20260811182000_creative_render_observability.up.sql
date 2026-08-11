CREATE TABLE creative_render_job_usage (
  render_job_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  organization_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  project_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  currency CHAR(3) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  actual_cost_minor BIGINT UNSIGNED NULL,
  unavailable_reason VARCHAR(500) NULL,
  measured_at DATETIME(6) NOT NULL,
  PRIMARY KEY (render_job_id),
  KEY idx_creative_render_usage_project (organization_id, project_id, measured_at),
  CONSTRAINT fk_creative_render_usage_job FOREIGN KEY (render_job_id) REFERENCES creative_render_jobs(id),
  CONSTRAINT chk_creative_render_usage_cost CHECK (
    (actual_cost_minor IS NOT NULL AND unavailable_reason IS NULL) OR
    (actual_cost_minor IS NULL AND unavailable_reason IS NOT NULL)
  )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE creative_render_job_events (
  render_job_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  organization_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  project_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  ordinal INT UNSIGNED NOT NULL,
  stage VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  safe_message VARCHAR(512) NOT NULL,
  error_code VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL,
  occurred_at DATETIME(6) NOT NULL,
  PRIMARY KEY (render_job_id, ordinal),
  UNIQUE KEY uq_creative_render_event_stage (render_job_id, stage),
  KEY idx_creative_render_events_project (organization_id, project_id, render_job_id, ordinal),
  CONSTRAINT fk_creative_render_events_job FOREIGN KEY (render_job_id) REFERENCES creative_render_jobs(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE creative_edit_render_job_usage (
  organization_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  project_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  render_job_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  currency CHAR(3) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  actual_cost_minor BIGINT UNSIGNED NULL,
  unavailable_reason VARCHAR(500) NULL,
  measured_at DATETIME(6) NOT NULL,
  PRIMARY KEY (organization_id, project_id, render_job_id),
  KEY idx_creative_edit_render_usage_project (organization_id, project_id, measured_at),
  CONSTRAINT fk_creative_edit_render_usage_job FOREIGN KEY (organization_id, project_id, render_job_id)
    REFERENCES creative_edit_render_jobs(organization_id, project_id, edit_render_job_id),
  CONSTRAINT chk_creative_edit_render_usage_cost CHECK (
    (actual_cost_minor IS NOT NULL AND unavailable_reason IS NULL) OR
    (actual_cost_minor IS NULL AND unavailable_reason IS NOT NULL)
  )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE creative_edit_render_job_events (
  organization_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  project_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  render_job_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  ordinal INT UNSIGNED NOT NULL,
  stage VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  safe_message VARCHAR(512) NOT NULL,
  error_code VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL,
  occurred_at DATETIME(6) NOT NULL,
  PRIMARY KEY (organization_id, project_id, render_job_id, ordinal),
  UNIQUE KEY uq_creative_edit_render_event_stage (organization_id, project_id, render_job_id, stage),
  KEY idx_creative_edit_render_events_project (organization_id, project_id, render_job_id, ordinal),
  CONSTRAINT fk_creative_edit_render_events_job FOREIGN KEY (organization_id, project_id, render_job_id)
    REFERENCES creative_edit_render_jobs(organization_id, project_id, edit_render_job_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
