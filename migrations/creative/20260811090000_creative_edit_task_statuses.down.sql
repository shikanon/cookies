UPDATE creative_edit_tasks SET status='draft' WHERE status <> 'draft';

ALTER TABLE creative_edit_tasks
    DROP KEY idx_creative_edit_tasks_project_status_updated,
    DROP CHECK chk_creative_edit_task_status,
    ADD CONSTRAINT chk_creative_edit_task_status CHECK (status IN ('draft'));
