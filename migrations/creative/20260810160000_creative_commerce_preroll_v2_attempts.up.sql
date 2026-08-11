CREATE TABLE creative_commerce_preroll_v2_attempts (
  id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  organization_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  project_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  task_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  operation_kind VARCHAR(48) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  draft_revision BIGINT NOT NULL,
  input_identity_hash VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  prompt_hash VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL,
  generation_spec_hash VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL,
  status VARCHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  progress_percent INT NOT NULL DEFAULT 0,
  provider_job_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NULL,
  error_code VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NULL,
  error_message VARCHAR(1000) NULL,
  output_asset_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NULL,
  output_asset_version BIGINT NULL,
  adopted_at DATETIME(6) NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (id),
  UNIQUE KEY uq_creative_commerce_v2_operation (
    organization_id, project_id, task_id, operation_kind, draft_revision, input_identity_hash
  ),
  UNIQUE KEY uq_creative_commerce_v2_provider_job (organization_id, provider_job_id),
  KEY idx_creative_commerce_v2_task (organization_id, project_id, task_id, created_at),
  CONSTRAINT fk_creative_commerce_v2_attempt_task
    FOREIGN KEY (organization_id, task_id) REFERENCES creative_tasks(organization_id, id),
  CONSTRAINT chk_creative_commerce_v2_attempt_revision CHECK (draft_revision >= 1),
  CONSTRAINT chk_creative_commerce_v2_attempt_progress CHECK (progress_percent BETWEEN 0 AND 100),
  CONSTRAINT chk_creative_commerce_v2_attempt_status CHECK (
    status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled', 'stale')
  ),
  CONSTRAINT chk_creative_commerce_v2_attempt_output CHECK (
    (output_asset_id IS NULL AND output_asset_version IS NULL)
    OR (output_asset_id IS NOT NULL AND output_asset_version >= 1)
  )
);

CREATE TABLE creative_commerce_preroll_v2_outbox (
  operation_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  organization_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  project_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  task_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  operation_kind VARCHAR(48) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  payload_json JSON NOT NULL,
  status VARCHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'pending',
  attempts INT NOT NULL DEFAULT 0,
  available_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (operation_id),
  KEY idx_creative_commerce_v2_outbox_claim (status, available_at),
  CONSTRAINT fk_creative_commerce_v2_outbox_attempt
    FOREIGN KEY (operation_id) REFERENCES creative_commerce_preroll_v2_attempts(id),
  CONSTRAINT chk_creative_commerce_v2_outbox_status CHECK (
    status IN ('pending', 'processing', 'dispatched', 'failed')
  )
);
