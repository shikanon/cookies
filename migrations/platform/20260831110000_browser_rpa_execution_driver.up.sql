ALTER TABLE browser_rpa_runs
  ADD COLUMN execution_driver VARCHAR(64) NOT NULL DEFAULT 'playwright-rpa/edge/v3' AFTER account_id;
