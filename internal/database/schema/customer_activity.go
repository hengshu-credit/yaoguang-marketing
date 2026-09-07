package schema

// CustomerActivityTimelineDefinitions is shared by fresh workspaces and V56.
// All entries are inserted in the same transaction as their source mutation;
// the existing timeline bridge retains ledger/outbox and automation semantics.
func CustomerActivityTimelineDefinitions() []string {
	return []string{
		ContactFieldsTimelineFunction(),
		ContactListTimelineFunction(),
		`DROP TRIGGER IF EXISTS contact_list_changes_trigger ON contact_lists`,
		`CREATE TRIGGER contact_list_changes_trigger AFTER INSERT OR UPDATE OR DELETE ON contact_lists
			FOR EACH ROW EXECUTE FUNCTION track_contact_list_changes()`,
		CustomerProfileTimelineFunction(),
		`DROP TRIGGER IF EXISTS customer_profile_timeline_trigger ON customer_profiles`,
		`CREATE TRIGGER customer_profile_timeline_trigger AFTER INSERT OR UPDATE ON customer_profiles
			FOR EACH ROW EXECUTE FUNCTION track_customer_profile_timeline()`,
		CustomerTagTimelineFunction(),
		`DROP TRIGGER IF EXISTS customer_tag_timeline_trigger ON customer_tags`,
		`CREATE TRIGGER customer_tag_timeline_trigger AFTER INSERT OR DELETE ON customer_tags
			FOR EACH ROW EXECUTE FUNCTION track_customer_tag_timeline()`,
	}
}

// Compare business columns dynamically so new contact fields are not silently
// omitted. Version/timestamp and customer projection maintenance are not edits.
func ContactFieldsTimelineFunction() string {
	return `CREATE OR REPLACE FUNCTION track_contact_changes() RETURNS TRIGGER AS $$
	DECLARE
		v_old JSONB;
		v_new JSONB;
		v_key TEXT;
		v_changes JSONB := '{}'::jsonb;
	BEGIN
		IF TG_OP = 'INSERT' THEN
			INSERT INTO contact_timeline (email, customer_id, operation, entity_type, kind, changes, created_at)
			VALUES (NEW.email, NEW.customer_id, 'insert', 'contact', 'contact.created', NULL, NEW.created_at);
			RETURN NEW;
		END IF;
		v_old := to_jsonb(OLD) - ARRAY['customer_id', 'created_at', 'updated_at', 'version'];
		v_new := to_jsonb(NEW) - ARRAY['customer_id', 'created_at', 'updated_at', 'version'];
		FOR v_key IN SELECT jsonb_object_keys(v_new) LOOP
			IF v_old->v_key IS DISTINCT FROM v_new->v_key THEN
				v_changes := v_changes || jsonb_build_object(v_key, jsonb_build_object('old', v_old->v_key, 'new', v_new->v_key));
			END IF;
		END LOOP;
		IF v_changes <> '{}'::jsonb THEN
			INSERT INTO contact_timeline (email, customer_id, operation, entity_type, kind, changes, created_at)
			VALUES (NEW.email, NEW.customer_id, 'update', 'contact', 'contact.updated', v_changes, CURRENT_TIMESTAMP);
		END IF;
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql`
}

func ContactListTimelineFunction() string {
	return `CREATE OR REPLACE FUNCTION track_contact_list_changes() RETURNS TRIGGER AS $$
	DECLARE
		v_row contact_lists%ROWTYPE;
		v_op TEXT := 'update';
		v_kind TEXT;
		v_changes JSONB;
		v_name TEXT;
	BEGIN
		IF TG_OP = 'DELETE' THEN
			IF OLD.deleted_at IS NOT NULL THEN RETURN OLD; END IF;
			v_row := OLD;
			v_op := 'delete';
			v_kind := 'list.removed';
			v_changes := jsonb_build_object('status', jsonb_build_object('old', OLD.status, 'new', NULL));
		ELSE
			v_row := NEW;
			IF TG_OP = 'INSERT' THEN
				IF NEW.deleted_at IS NOT NULL THEN RETURN NEW; END IF;
				v_op := 'insert';
				v_kind := CASE NEW.status WHEN 'active' THEN 'list.subscribed' ELSE 'list.' || NEW.status END;
				v_changes := jsonb_build_object('status', jsonb_build_object('new', NEW.status));
			ELSIF OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL THEN
				v_op := 'delete';
				v_kind := 'list.removed';
				v_changes := jsonb_build_object(
					'status', jsonb_build_object('old', OLD.status, 'new', NULL),
					'deleted_at', jsonb_build_object('old', OLD.deleted_at, 'new', NEW.deleted_at));
			ELSIF OLD.deleted_at IS NOT NULL AND NEW.deleted_at IS NULL THEN
				v_op := 'insert';
				v_kind := CASE NEW.status WHEN 'active' THEN 'list.resubscribed' ELSE 'list.' || NEW.status END;
				v_changes := jsonb_build_object('status', jsonb_build_object('new', NEW.status));
			ELSIF NEW.deleted_at IS NULL AND OLD.status IS DISTINCT FROM NEW.status THEN
				v_kind := CASE
					WHEN OLD.status = 'pending' AND NEW.status = 'active' THEN 'list.confirmed'
					WHEN OLD.status IN ('unsubscribed', 'bounced', 'complained') AND NEW.status = 'active' THEN 'list.resubscribed'
					WHEN NEW.status = 'active' THEN 'list.subscribed'
					ELSE 'list.' || NEW.status END;
				v_changes := jsonb_build_object('status', jsonb_build_object('old', OLD.status, 'new', NEW.status));
			ELSE RETURN NEW;
			END IF;
		END IF;
		SELECT name INTO v_name FROM lists WHERE id = v_row.list_id;
		v_changes := v_changes || jsonb_build_object(
			'list_id', jsonb_build_object('new', v_row.list_id),
			'list_name', jsonb_build_object('new', v_name));
		INSERT INTO contact_timeline (email, customer_id, operation, entity_type, kind, entity_id, changes, created_at)
		VALUES (v_row.email, v_row.customer_id, v_op, 'contact_list', v_kind, v_row.list_id, v_changes, CURRENT_TIMESTAMP);
		IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql`
}

func CustomerProfileTimelineFunction() string {
	return `CREATE OR REPLACE FUNCTION track_customer_profile_timeline() RETURNS TRIGGER AS $$
	DECLARE
		v_old JSONB := '{}'::jsonb;
		v_new JSONB;
		v_changes JSONB := '{}'::jsonb;
		v_attributes JSONB := '{}'::jsonb;
		v_key TEXT;
		v_email TEXT;
	BEGIN
		IF TG_OP = 'UPDATE' THEN v_old := to_jsonb(OLD); END IF;
		v_new := to_jsonb(NEW) - ARRAY['customer_id', 'created_at', 'updated_at', 'version', 'attributes'];
		FOR v_key IN SELECT jsonb_object_keys(v_new) LOOP
			IF COALESCE(v_old->v_key, 'null'::jsonb) IS DISTINCT FROM v_new->v_key THEN
				v_changes := v_changes || jsonb_build_object(v_key, jsonb_build_object('old', v_old->v_key, 'new', v_new->v_key));
			END IF;
		END LOOP;
		FOR v_key IN SELECT jsonb_object_keys(COALESCE(v_old->'attributes', '{}'::jsonb) || NEW.attributes) LOOP
			IF v_old->'attributes'->v_key IS DISTINCT FROM NEW.attributes->v_key THEN
				v_attributes := v_attributes || jsonb_build_object(v_key, jsonb_build_object('old', v_old->'attributes'->v_key, 'new', NEW.attributes->v_key));
			END IF;
		END LOOP;
		IF v_attributes <> '{}'::jsonb THEN v_changes := v_changes || jsonb_build_object('attributes', v_attributes); END IF;
		IF v_changes = '{}'::jsonb THEN RETURN NEW; END IF;
		SELECT email INTO v_email FROM contacts WHERE customer_id = NEW.customer_id;
		INSERT INTO contact_timeline (email, customer_id, operation, entity_type, kind, entity_id, changes, created_at)
		VALUES (COALESCE(v_email, ''), NEW.customer_id, lower(TG_OP), 'contact_profile',
			CASE TG_OP WHEN 'INSERT' THEN 'contact.profile_created' ELSE 'contact.profile_updated' END,
			NEW.customer_id::text, v_changes, CURRENT_TIMESTAMP);
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql`
}

func CustomerTagTimelineFunction() string {
	return `CREATE OR REPLACE FUNCTION track_customer_tag_timeline() RETURNS TRIGGER AS $$
	DECLARE
		v_id UUID;
		v_tag TEXT;
		v_email TEXT;
	BEGIN
		IF TG_OP = 'INSERT' THEN v_id := NEW.customer_id; v_tag := NEW.tag;
		ELSE v_id := OLD.customer_id; v_tag := OLD.tag; END IF;
		-- A customer's own deletion cascades to tags; do not recreate their history.
		IF NOT EXISTS (SELECT 1 FROM customers WHERE id = v_id) THEN RETURN OLD; END IF;
		SELECT email INTO v_email FROM contacts WHERE customer_id = v_id;
		INSERT INTO contact_timeline (email, customer_id, operation, entity_type, kind, entity_id, changes, created_at)
		VALUES (COALESCE(v_email, ''), v_id, lower(TG_OP), 'contact_tag',
			CASE TG_OP WHEN 'INSERT' THEN 'contact.tagged' ELSE 'contact.untagged' END, v_tag,
			jsonb_build_object('tag', CASE TG_OP WHEN 'INSERT' THEN jsonb_build_object('new', v_tag) ELSE jsonb_build_object('old', v_tag) END), CURRENT_TIMESTAMP);
		IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql`
}
