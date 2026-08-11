ALTER TABLE creative_packages
  DROP FOREIGN KEY fk_creative_packages_edit_task,
  DROP KEY idx_creative_packages_edit_task,
  DROP COLUMN edit_task_id;

ALTER TABLE creative_versions
  DROP CHECK chk_creative_versions_single_source,
  DROP FOREIGN KEY fk_creative_versions_edit_task,
  DROP FOREIGN KEY fk_creative_versions_task,
  DROP KEY uq_creative_versions_edit_timeline,
  DROP KEY uq_creative_versions_task_draft,
  DROP COLUMN edit_task_id,
  MODIFY COLUMN task_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  ADD UNIQUE KEY uq_creative_versions_task_draft (organization_id, task_id, draft_version),
  ADD CONSTRAINT fk_creative_versions_task FOREIGN KEY (organization_id, task_id) REFERENCES creative_tasks(organization_id, id);
