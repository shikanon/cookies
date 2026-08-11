DELETE FROM creative_edit_tasks WHERE current_timeline_version IS NULL;

ALTER TABLE creative_edit_tasks
    DROP CHECK chk_creative_edit_task_current_version,
    MODIFY current_timeline_version BIGINT NOT NULL,
    ADD CONSTRAINT chk_creative_edit_task_current_version
        CHECK (current_timeline_version > 0);
