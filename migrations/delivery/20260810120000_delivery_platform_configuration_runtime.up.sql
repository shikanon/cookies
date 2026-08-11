-- Additive runtime discriminators and approval bindings for DeliveryIntent +
-- tagged PlatformConfiguration. Existing JSON documents and hashes are left
-- byte-for-byte untouched; NULL discriminator/binding columns mean legacy.
CREATE TABLE delivery_intents (
  organization_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  project_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  intent_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  version_number INT NOT NULL,
  schema_version VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  canonical_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  hash_algorithm VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  intent_json JSON NOT NULL,
  created_by VARCHAR(128) NOT NULL,
  created_at DATETIME(6) NOT NULL,
  PRIMARY KEY (organization_id, project_id, intent_id, version_number),
  KEY idx_delivery_intents_hash (organization_id, project_id, canonical_hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE delivery_platform_configurations (
  organization_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  project_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  configuration_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  version_number INT NOT NULL,
  schema_version VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  platform VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  profile_version VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  intent_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  intent_version INT NOT NULL,
  intent_canonical_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  canonical_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  hash_algorithm VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  configuration_json JSON NOT NULL,
  created_by VARCHAR(128) NOT NULL,
  created_at DATETIME(6) NOT NULL,
  PRIMARY KEY (organization_id, project_id, configuration_id, version_number),
  KEY idx_delivery_platform_configurations_hash (organization_id, project_id, canonical_hash),
  CONSTRAINT fk_delivery_platform_configuration_intent
    FOREIGN KEY (organization_id, project_id, intent_id, intent_version)
    REFERENCES delivery_intents (organization_id, project_id, intent_id, version_number)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

ALTER TABLE delivery_plan_versions
  ADD COLUMN payload_schema_version VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER canonical_hash;

ALTER TABLE delivery_change_sets
  ADD COLUMN target_snapshot_schema_version VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER target_snapshot_hash;

ALTER TABLE delivery_approvals
  ADD COLUMN configuration_schema_version VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER target_snapshot_hash,
  ADD COLUMN configuration_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER configuration_schema_version,
  ADD COLUMN configuration_version INT NULL AFTER configuration_id,
  ADD COLUMN configuration_platform VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER configuration_version,
  ADD COLUMN configuration_profile_version VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER configuration_platform,
  ADD COLUMN configuration_canonical_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER configuration_profile_version,
  ADD COLUMN intent_schema_version VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER configuration_canonical_hash,
  ADD COLUMN intent_id VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER intent_schema_version,
  ADD COLUMN intent_version INT NULL AFTER intent_id,
  ADD COLUMN intent_canonical_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER intent_version;

CREATE INDEX idx_delivery_plan_versions_payload_schema
  ON delivery_plan_versions (organization_id, project_id, payload_schema_version, plan_id, version_number);

-- The simulator consumer prefixes its immutable fixture version with the
-- adapter version. Preserve that complete audit identifier for v2 outcomes.
ALTER TABLE delivery_alerts
  MODIFY dataset_version VARCHAR(160) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  MODIFY fixture_version VARCHAR(160) CHARACTER SET ascii COLLATE ascii_bin NOT NULL;
