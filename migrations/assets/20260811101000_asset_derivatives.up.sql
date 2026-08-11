CREATE TABLE asset_derivatives (
  id VARCHAR(96) PRIMARY KEY,
  organization_id VARCHAR(96) NOT NULL,
  project_id VARCHAR(96) NOT NULL,
  source_asset_id VARCHAR(96) NOT NULL,
  source_asset_version BIGINT NOT NULL,
  profile VARCHAR(64) NOT NULL,
  status VARCHAR(24) NOT NULL,
  output_asset_id VARCHAR(96) NULL,
  output_asset_version BIGINT NULL,
  error_code VARCHAR(96) NULL,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  UNIQUE KEY uq_asset_derivative (organization_id, project_id, source_asset_id, source_asset_version, profile),
  KEY idx_asset_derivative_status (organization_id, status, updated_at)
);

CREATE TABLE asset_processing_jobs (
  id VARCHAR(96) PRIMARY KEY,
  organization_id VARCHAR(96) NOT NULL,
  project_id VARCHAR(96) NOT NULL,
  derivative_id VARCHAR(96) NOT NULL,
  status VARCHAR(24) NOT NULL,
  attempt INT NOT NULL,
  error_code VARCHAR(96) NULL,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  KEY idx_asset_processing_derivative (derivative_id, attempt),
  CONSTRAINT fk_asset_processing_derivative FOREIGN KEY (derivative_id) REFERENCES asset_derivatives(id)
);
