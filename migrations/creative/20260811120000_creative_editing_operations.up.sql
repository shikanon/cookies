ALTER TABLE creative_edit_timeline_versions
  ADD COLUMN schema_version VARCHAR(64) NOT NULL DEFAULT 'editing-timeline/v1' AFTER version,
  ADD COLUMN parent_version BIGINT NULL AFTER schema_version,
  ADD COLUMN change_summary VARCHAR(255) NULL AFTER content_hash,
  ADD COLUMN operation_batch_id VARCHAR(96) NULL AFTER change_summary,
  ADD COLUMN compiler_compatibility VARCHAR(96) NOT NULL DEFAULT 'editing-v1' AFTER operation_batch_id,
  ADD UNIQUE KEY uq_creative_edit_operation_batch (organization_id, project_id, edit_task_id, operation_batch_id);

CREATE TABLE creative_edit_operation_batches (
  organization_id VARCHAR(96) NOT NULL,
  project_id VARCHAR(96) NOT NULL,
  edit_task_id VARCHAR(96) NOT NULL,
  batch_id VARCHAR(96) NOT NULL,
  base_timeline_version BIGINT NOT NULL,
  result_timeline_version BIGINT NOT NULL,
  actor VARCHAR(255) NOT NULL,
  operations_payload JSON NOT NULL,
  inverse_payload JSON NOT NULL,
  result_content_hash VARCHAR(80) NOT NULL,
  change_summary VARCHAR(255) NOT NULL,
  created_at DATETIME(6) NOT NULL,
  PRIMARY KEY (organization_id, project_id, edit_task_id, batch_id),
  KEY idx_creative_edit_operation_replay (organization_id, project_id, edit_task_id, result_timeline_version)
);

ALTER TABLE creative_edit_render_jobs
  ADD COLUMN timeline_schema_version VARCHAR(64) NOT NULL DEFAULT 'editing-timeline/v1' AFTER timeline_version,
  ADD COLUMN compiler_version VARCHAR(96) NOT NULL DEFAULT 'editing-v1' AFTER timeline_hash;
