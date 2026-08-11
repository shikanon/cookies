DROP TABLE IF EXISTS creative_production_retry_commands;

ALTER TABLE creative_edit_render_jobs
  DROP INDEX uq_creative_edit_render_retry_key,
  DROP COLUMN retry_request_hash,
  DROP COLUMN retry_idempotency_key;
