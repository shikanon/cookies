-- MySQL DDL auto-commits even inside the migration runner transaction. Every
-- step is therefore conditional so an interrupted upgrade can be rerun.
SET @ddl := IF(
  EXISTS(SELECT 1 FROM information_schema.TABLE_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME = 'creative_versions' AND CONSTRAINT_NAME = 'fk_creative_versions_task'),
  'ALTER TABLE creative_versions DROP FOREIGN KEY fk_creative_versions_task',
  'DO 0'
);
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @ddl := IF(
  EXISTS(SELECT 1 FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'creative_versions' AND INDEX_NAME = 'uq_creative_versions_task_draft'),
  'ALTER TABLE creative_versions DROP KEY uq_creative_versions_task_draft',
  'DO 0'
);
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

ALTER TABLE creative_versions MODIFY COLUMN task_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NULL;
SET @ddl := IF(
  NOT EXISTS(SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'creative_versions' AND COLUMN_NAME = 'edit_task_id'),
  'ALTER TABLE creative_versions ADD COLUMN edit_task_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER task_id',
  'DO 0'
);
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @ddl := IF(
  NOT EXISTS(SELECT 1 FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'creative_versions' AND INDEX_NAME = 'uq_creative_versions_task_draft'),
  'ALTER TABLE creative_versions ADD UNIQUE KEY uq_creative_versions_task_draft (organization_id, task_id, draft_version)',
  'DO 0'
);
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @ddl := IF(
  NOT EXISTS(SELECT 1 FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'creative_versions' AND INDEX_NAME = 'uq_creative_versions_edit_timeline'),
  'ALTER TABLE creative_versions ADD UNIQUE KEY uq_creative_versions_edit_timeline (organization_id, project_id, edit_task_id, draft_version)',
  'DO 0'
);
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @ddl := IF(
  NOT EXISTS(SELECT 1 FROM information_schema.TABLE_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME = 'creative_versions' AND CONSTRAINT_NAME = 'fk_creative_versions_task'),
  'ALTER TABLE creative_versions ADD CONSTRAINT fk_creative_versions_task FOREIGN KEY (organization_id, task_id) REFERENCES creative_tasks(organization_id, id)',
  'DO 0'
);
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @ddl := IF(
  NOT EXISTS(SELECT 1 FROM information_schema.TABLE_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME = 'creative_versions' AND CONSTRAINT_NAME = 'fk_creative_versions_edit_task'),
  'ALTER TABLE creative_versions ADD CONSTRAINT fk_creative_versions_edit_task FOREIGN KEY (organization_id, project_id, edit_task_id) REFERENCES creative_edit_tasks(organization_id, project_id, edit_task_id)',
  'DO 0'
);
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @ddl := IF(
  NOT EXISTS(SELECT 1 FROM information_schema.TABLE_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME = 'creative_versions' AND CONSTRAINT_NAME = 'chk_creative_versions_single_source'),
  'ALTER TABLE creative_versions ADD CONSTRAINT chk_creative_versions_single_source CHECK ((task_id IS NULL) <> (edit_task_id IS NULL))',
  'DO 0'
);
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @ddl := IF(
  NOT EXISTS(SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'creative_packages' AND COLUMN_NAME = 'edit_task_id'),
  'ALTER TABLE creative_packages ADD COLUMN edit_task_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER creative_version_id',
  'DO 0'
);
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @ddl := IF(
  NOT EXISTS(SELECT 1 FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'creative_packages' AND INDEX_NAME = 'idx_creative_packages_edit_task'),
  'ALTER TABLE creative_packages ADD KEY idx_creative_packages_edit_task (organization_id, project_id, edit_task_id, created_at)',
  'DO 0'
);
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @ddl := IF(
  NOT EXISTS(SELECT 1 FROM information_schema.TABLE_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME = 'creative_packages' AND CONSTRAINT_NAME = 'fk_creative_packages_edit_task'),
  'ALTER TABLE creative_packages ADD CONSTRAINT fk_creative_packages_edit_task FOREIGN KEY (organization_id, project_id, edit_task_id) REFERENCES creative_edit_tasks(organization_id, project_id, edit_task_id)',
  'DO 0'
);
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;
