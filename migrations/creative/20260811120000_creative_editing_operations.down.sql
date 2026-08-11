DROP TABLE IF EXISTS creative_edit_operation_batches;
ALTER TABLE creative_edit_render_jobs DROP COLUMN compiler_version, DROP COLUMN timeline_schema_version;
ALTER TABLE creative_edit_timeline_versions
  DROP KEY uq_creative_edit_operation_batch,
  DROP COLUMN compiler_compatibility,
  DROP COLUMN operation_batch_id,
  DROP COLUMN change_summary,
  DROP COLUMN parent_version,
  DROP COLUMN schema_version;
