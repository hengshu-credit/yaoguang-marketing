package schema

// ContactEndpointTableDefinitions returns the workspace-local encrypted client
// endpoint schema. Timeline changes deliberately contain metadata only: provider
// addresses, ciphertext, and fingerprints never enter the event ledger/outbox.
func ContactEndpointTableDefinitions() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS contact_endpoints (
			endpoint_id VARCHAR(128) PRIMARY KEY,
			email VARCHAR(255) NOT NULL REFERENCES contacts(email) ON DELETE CASCADE,
			channel VARCHAR(20) NOT NULL CHECK (channel IN ('sms', 'push')),
			provider VARCHAR(20) NOT NULL CHECK (provider IN ('twilio', 'fcm', 'apns', 'webpush')),
			platform VARCHAR(20) NOT NULL CHECK (platform IN ('phone', 'android', 'ios', 'web')),
			address_ciphertext TEXT NOT NULL,
			address_fingerprint CHAR(64) NOT NULL,
			locale VARCHAR(35),
			timezone VARCHAR(100),
			app_id VARCHAR(255),
			device_id VARCHAR(255),
			attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_seen_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CHECK ((channel = 'sms' AND provider = 'twilio' AND platform = 'phone')
				OR (channel = 'push' AND provider = 'apns' AND platform = 'ios')
				OR (channel = 'push' AND provider = 'fcm' AND platform IN ('android', 'ios'))
				OR (channel = 'push' AND provider = 'webpush' AND platform = 'web'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contact_endpoints_active_contact
			ON contact_endpoints (email, channel, provider) WHERE enabled`,
		`CREATE INDEX IF NOT EXISTS idx_contact_endpoints_fingerprint
			ON contact_endpoints (provider, address_fingerprint)`,
		ContactEndpointTimelineFunction(),
		`DROP TRIGGER IF EXISTS contact_endpoint_timeline_trigger ON contact_endpoints`,
		`CREATE TRIGGER contact_endpoint_timeline_trigger
			AFTER INSERT OR UPDATE ON contact_endpoints
			FOR EACH ROW EXECUTE FUNCTION track_contact_endpoint_timeline()`,
	}
}

func ContactEndpointTimelineFunction() string {
	return `CREATE OR REPLACE FUNCTION track_contact_endpoint_timeline()
	RETURNS TRIGGER AS $$
	DECLARE
		v_kind TEXT;
		v_changes JSONB;
	BEGIN
		IF TG_OP = 'INSERT' THEN
			v_kind := 'contact.endpoint_registered';
			v_changes := jsonb_build_object(
				'channel', jsonb_build_object('new', NEW.channel),
				'provider', jsonb_build_object('new', NEW.provider),
				'platform', jsonb_build_object('new', NEW.platform),
				'locale', jsonb_build_object('new', NEW.locale),
				'timezone', jsonb_build_object('new', NEW.timezone),
				'app_id', jsonb_build_object('new', NEW.app_id),
				'device_id', jsonb_build_object('new', NEW.device_id),
				'attributes', jsonb_build_object('new', NEW.attributes),
				'enabled', jsonb_build_object('new', NEW.enabled),
				'version', jsonb_build_object('new', NEW.version)
			);
		ELSE
			IF ROW(OLD.email, OLD.channel, OLD.provider, OLD.platform, OLD.locale, OLD.timezone,
				OLD.app_id, OLD.device_id, OLD.attributes, OLD.enabled, OLD.version)
				IS NOT DISTINCT FROM
				ROW(NEW.email, NEW.channel, NEW.provider, NEW.platform, NEW.locale, NEW.timezone,
				NEW.app_id, NEW.device_id, NEW.attributes, NEW.enabled, NEW.version) THEN
				RETURN NEW;
			END IF;
			IF OLD.enabled AND NOT NEW.enabled THEN
				v_kind := 'contact.endpoint_disabled';
			ELSIF NOT OLD.enabled AND NEW.enabled THEN
				v_kind := 'contact.endpoint_registered';
			ELSE
				v_kind := 'contact.endpoint_updated';
			END IF;
			v_changes := jsonb_strip_nulls(jsonb_build_object(
				'email', CASE WHEN OLD.email IS DISTINCT FROM NEW.email THEN jsonb_build_object('old', OLD.email, 'new', NEW.email) END,
				'channel', CASE WHEN OLD.channel IS DISTINCT FROM NEW.channel THEN jsonb_build_object('old', OLD.channel, 'new', NEW.channel) END,
				'provider', CASE WHEN OLD.provider IS DISTINCT FROM NEW.provider THEN jsonb_build_object('old', OLD.provider, 'new', NEW.provider) END,
				'platform', CASE WHEN OLD.platform IS DISTINCT FROM NEW.platform THEN jsonb_build_object('old', OLD.platform, 'new', NEW.platform) END,
				'locale', CASE WHEN OLD.locale IS DISTINCT FROM NEW.locale THEN jsonb_build_object('old', OLD.locale, 'new', NEW.locale) END,
				'timezone', CASE WHEN OLD.timezone IS DISTINCT FROM NEW.timezone THEN jsonb_build_object('old', OLD.timezone, 'new', NEW.timezone) END,
				'app_id', CASE WHEN OLD.app_id IS DISTINCT FROM NEW.app_id THEN jsonb_build_object('old', OLD.app_id, 'new', NEW.app_id) END,
				'device_id', CASE WHEN OLD.device_id IS DISTINCT FROM NEW.device_id THEN jsonb_build_object('old', OLD.device_id, 'new', NEW.device_id) END,
				'attributes', CASE WHEN OLD.attributes IS DISTINCT FROM NEW.attributes THEN jsonb_build_object('old', OLD.attributes, 'new', NEW.attributes) END,
				'enabled', CASE WHEN OLD.enabled IS DISTINCT FROM NEW.enabled THEN jsonb_build_object('old', OLD.enabled, 'new', NEW.enabled) END,
				'version', jsonb_build_object('old', OLD.version, 'new', NEW.version)
			));
		END IF;

		INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
		VALUES (
			NEW.email, CASE WHEN TG_OP = 'INSERT' THEN 'insert' ELSE 'update' END,
			'contact_endpoint', v_kind, NEW.endpoint_id, v_changes, NEW.updated_at
		);
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql`
}
