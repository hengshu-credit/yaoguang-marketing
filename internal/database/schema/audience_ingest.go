package schema

// AudienceIngestTableDefinitions returns the workspace-local external profile
// and tag schema. Both triggers append semantic timeline events, which the v40
// bridge captures into the durable event ledger and outbox in the same commit.
func AudienceIngestTableDefinitions() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS contact_profiles (
			email VARCHAR(255) PRIMARY KEY REFERENCES contacts(email) ON DELETE CASCADE,
			status VARCHAR(64),
			attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
			version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contact_profiles_status
			ON contact_profiles (status) WHERE status IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_contact_profiles_attributes
			ON contact_profiles USING GIN (attributes jsonb_path_ops)`,
		`CREATE TABLE IF NOT EXISTS contact_tags (
			email VARCHAR(255) NOT NULL REFERENCES contacts(email) ON DELETE CASCADE,
			tag VARCHAR(64) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (email, tag)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contact_tags_tag_email ON contact_tags (tag, email)`,
		AudienceProfileTimelineFunction(),
		`DROP TRIGGER IF EXISTS contact_profile_timeline_trigger ON contact_profiles`,
		`CREATE TRIGGER contact_profile_timeline_trigger
			AFTER INSERT OR UPDATE ON contact_profiles
			FOR EACH ROW EXECUTE FUNCTION track_contact_profile_timeline()`,
		AudienceTagTimelineFunction(),
		`DROP TRIGGER IF EXISTS contact_tag_timeline_trigger ON contact_tags`,
		`CREATE TRIGGER contact_tag_timeline_trigger
			AFTER INSERT OR DELETE ON contact_tags
			FOR EACH ROW EXECUTE FUNCTION track_contact_tag_timeline()`,
	}
}

func AudienceProfileTimelineFunction() string {
	return `CREATE OR REPLACE FUNCTION track_contact_profile_timeline()
	RETURNS TRIGGER AS $$
	DECLARE
		v_changes JSONB := '{}'::jsonb;
		v_key TEXT;
		v_attribute_changes JSONB := '{}'::jsonb;
	BEGIN
		IF TG_OP = 'INSERT' THEN
			v_changes := jsonb_build_object(
				'status', jsonb_build_object('new', NEW.status),
				'attributes', jsonb_build_object('new', NEW.attributes),
				'version', jsonb_build_object('new', NEW.version)
			);
			INSERT INTO contact_timeline (email, operation, entity_type, kind, changes, created_at)
			VALUES (NEW.email, 'insert', 'contact_profile', 'contact.profile_created', v_changes, NEW.created_at);
			RETURN NEW;
		END IF;

		IF OLD.status IS DISTINCT FROM NEW.status THEN
			v_changes := v_changes || jsonb_build_object(
				'status', jsonb_build_object('old', OLD.status, 'new', NEW.status)
			);
		END IF;
		FOR v_key IN
			SELECT DISTINCT key FROM (
				SELECT jsonb_object_keys(OLD.attributes) AS key
				UNION
				SELECT jsonb_object_keys(NEW.attributes) AS key
			) keys
		LOOP
			IF OLD.attributes->v_key IS DISTINCT FROM NEW.attributes->v_key THEN
				v_attribute_changes := v_attribute_changes || jsonb_build_object(
					v_key, jsonb_build_object('old', OLD.attributes->v_key, 'new', NEW.attributes->v_key)
				);
			END IF;
		END LOOP;
		IF v_attribute_changes <> '{}'::jsonb THEN
			v_changes := v_changes || jsonb_build_object('attributes', v_attribute_changes);
		END IF;
		IF v_changes = '{}'::jsonb THEN
			RETURN NEW;
		END IF;
		v_changes := v_changes || jsonb_build_object(
			'version', jsonb_build_object('old', OLD.version, 'new', NEW.version)
		);
		INSERT INTO contact_timeline (email, operation, entity_type, kind, changes, created_at)
		VALUES (NEW.email, 'update', 'contact_profile', 'contact.profile_updated', v_changes, NEW.updated_at);
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql`
}

func AudienceTagTimelineFunction() string {
	return `CREATE OR REPLACE FUNCTION track_contact_tag_timeline()
	RETURNS TRIGGER AS $$
	BEGIN
		IF TG_OP = 'INSERT' THEN
			INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
			VALUES (
				NEW.email, 'insert', 'contact_tag', 'contact.tagged', NEW.tag,
				jsonb_build_object('tag', jsonb_build_object('new', NEW.tag)), NEW.created_at
			);
			RETURN NEW;
		END IF;
		INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
		VALUES (
			OLD.email, 'delete', 'contact_tag', 'contact.untagged', OLD.tag,
			jsonb_build_object('tag', jsonb_build_object('old', OLD.tag)), CURRENT_TIMESTAMP
		);
		RETURN OLD;
	END;
	$$ LANGUAGE plpgsql`
}
