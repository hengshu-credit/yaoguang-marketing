package schema

// CustomerTableDefinitions returns the Workspace-local UUID Customer schema.
// These statements are shared by fresh initialization and V46 upgrades so a
// first-run database (which is stamped directly at the current version) cannot
// silently miss tables that an upgraded database receives through migrations.
func CustomerTableDefinitions() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS customers (
			id UUID PRIMARY KEY,
			customer_no VARCHAR(53) NOT NULL,
			external_user_id VARCHAR(255),
			merged_into_id UUID REFERENCES customers(id) ON DELETE RESTRICT,
			merged_at TIMESTAMPTZ,
			version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_customers_customer_no
			ON customers (customer_no)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_customers_external_user_id
			ON customers (external_user_id) WHERE external_user_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_customers_merged_into_id
			ON customers (merged_into_id) WHERE merged_into_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_customers_created_at_id
			ON customers (created_at, id)`,

		`CREATE TABLE IF NOT EXISTS customer_profiles (
			customer_id UUID PRIMARY KEY REFERENCES customers(id) ON DELETE CASCADE,
			status VARCHAR(64),
			language VARCHAR(50),
			timezone VARCHAR(50),
			attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
			version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_customer_profiles_status
			ON customer_profiles (status) WHERE status IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_customer_profiles_attributes
			ON customer_profiles USING GIN (attributes jsonb_path_ops)`,

		`CREATE TABLE IF NOT EXISTS customer_identities (
			id UUID PRIMARY KEY,
			customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
			identity_type VARCHAR(32) NOT NULL CHECK (identity_type IN (
				'email', 'phone', 'anonymous_id', 'device_id', 'whatsapp', 'telegram', 'custom'
			)),
			value_ciphertext TEXT NOT NULL,
			lookup_fingerprint CHAR(64) NOT NULL,
			display_hint VARCHAR(255) NOT NULL,
			verified BOOLEAN NOT NULL DEFAULT FALSE,
			is_primary BOOLEAN NOT NULL DEFAULT FALSE,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_customer_identities_lookup
			ON customer_identities (identity_type, lookup_fingerprint)`,
		`CREATE INDEX IF NOT EXISTS idx_customer_identities_customer
			ON customer_identities (customer_id, identity_type, enabled)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_customer_identities_primary
			ON customer_identities (customer_id, identity_type)
			WHERE is_primary AND enabled`,

		`CREATE TABLE IF NOT EXISTS customer_tags (
			customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
			tag VARCHAR(64) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (customer_id, tag)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_customer_tags_tag_customer
			ON customer_tags (tag, customer_id)`,

		`CREATE TABLE IF NOT EXISTS customer_consents (
			id UUID PRIMARY KEY,
			customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
			purpose VARCHAR(64) NOT NULL,
			channel VARCHAR(32) NOT NULL,
			status VARCHAR(32) NOT NULL,
			source VARCHAR(128),
			valid_from TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			revoked_at TIMESTAMPTZ,
			metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_customer_consents_scope
			ON customer_consents (customer_id, purpose, channel)`,
		`CREATE INDEX IF NOT EXISTS idx_customer_consents_customer_status
			ON customer_consents (customer_id, status)`,

		`CREATE TABLE IF NOT EXISTS customer_list_memberships (
			customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
			list_id VARCHAR(32) NOT NULL REFERENCES lists(id) ON DELETE RESTRICT,
			status VARCHAR(20) NOT NULL CHECK (status IN (
				'active', 'pending', 'unsubscribed', 'bounced', 'complained'
			)),
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (customer_id, list_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_customer_list_memberships_list
			ON customer_list_memberships (list_id, status, customer_id)`,

		`CREATE TABLE IF NOT EXISTS customer_merge_log (
			id UUID PRIMARY KEY,
			source_customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
			target_customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
			actor_id VARCHAR(255),
			reason VARCHAR(500),
			source_snapshot JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_customer_merge_log_source
			ON customer_merge_log (source_customer_id)`,
		`CREATE INDEX IF NOT EXISTS idx_customer_merge_log_target
			ON customer_merge_log (target_customer_id, created_at)`,

		`CREATE TABLE IF NOT EXISTS customer_idempotency (
			operation VARCHAR(64) NOT NULL,
			idempotency_key VARCHAR(255) NOT NULL,
			payload_hash CHAR(64) NOT NULL,
			customer_id UUID REFERENCES customers(id) ON DELETE SET NULL,
			response JSONB,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (operation, idempotency_key)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_customer_idempotency_created_at
			ON customer_idempotency (created_at)`,

		`ALTER TABLE contacts ADD COLUMN IF NOT EXISTS customer_id UUID`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_contacts_customer_id
			ON contacts (customer_id) WHERE customer_id IS NOT NULL`,
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'contacts_customer_id_fkey'
				AND conrelid = 'contacts'::regclass
			) THEN
				ALTER TABLE contacts
					ADD CONSTRAINT contacts_customer_id_fkey
					FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE SET NULL;
			END IF;
		END $$`,
		`ALTER TABLE contact_endpoints ADD COLUMN IF NOT EXISTS customer_id UUID`,
		`CREATE INDEX IF NOT EXISTS idx_contact_endpoints_customer_id
			ON contact_endpoints (customer_id) WHERE customer_id IS NOT NULL`,
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'contact_endpoints_customer_id_fkey'
				AND conrelid = 'contact_endpoints'::regclass
			) THEN
				ALTER TABLE contact_endpoints
					ADD CONSTRAINT contact_endpoints_customer_id_fkey
					FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE SET NULL;
			END IF;
		END $$`,
	}
}
