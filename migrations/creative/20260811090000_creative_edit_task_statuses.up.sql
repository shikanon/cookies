ALTER TABLE creative_edit_tasks
    DROP CHECK chk_creative_edit_task_status,
    ADD CONSTRAINT chk_creative_edit_task_status
      CHECK (status IN ('draft', 'rendering', 'review_ready', 'completed', 'failed', 'archived')),
    ADD KEY idx_creative_edit_tasks_project_status_updated
      (organization_id, project_id, status, updated_at, edit_task_id);
