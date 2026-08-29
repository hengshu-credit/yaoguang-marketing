package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIngestBatchRequestValidate(t *testing.T) {
	t.Run("accepts contacts and events within the batch limit", func(t *testing.T) {
		req := IngestBatchRequest{
			WorkspaceID: "workspace-1",
			Contacts: []IngestContact{{
				ID:      "contact-1",
				Contact: json.RawMessage(`{"email":"user@example.com"}`),
			}},
			Events: []IngestEvent{{
				ID: "event-1", Email: "user@example.com",
				EventName: "order.completed", ExternalID: "order-1",
			}},
		}

		require.NoError(t, req.Validate(500))
	})

	t.Run("rejects an empty batch", func(t *testing.T) {
		req := IngestBatchRequest{WorkspaceID: "workspace-1"}
		assert.ErrorContains(t, req.Validate(500), "at least one")
	})

	t.Run("rejects a batch over the configured limit", func(t *testing.T) {
		req := IngestBatchRequest{WorkspaceID: "workspace-1", Contacts: make([]IngestContact, 501)}
		assert.ErrorContains(t, req.Validate(500), "cannot contain more than 500")
	})
}

func TestIngestContactValidate(t *testing.T) {
	t.Run("normalizes tags and accepts all supported membership states", func(t *testing.T) {
		record := IngestContact{
			ID:      "contact-1",
			Contact: json.RawMessage(`{"email":"USER@example.com"}`),
			Tags:    &TagMutation{Operation: TagOperationSet, Values: []string{" paid ", "beta", "paid"}},
			ListMemberships: []IngestListMembership{{
				ListID: "newsletter", Status: ContactListStatusUnsubscribed,
			}},
		}

		contact, err := record.Validate()
		require.NoError(t, err)
		assert.Equal(t, "user@example.com", contact.Email)
		assert.Equal(t, []string{"beta", "paid"}, record.Tags.Values)
	})

	t.Run("rejects an unknown tag operation", func(t *testing.T) {
		record := IngestContact{
			ID:      "contact-1",
			Contact: json.RawMessage(`{"email":"user@example.com"}`),
			Tags:    &TagMutation{Operation: "append", Values: []string{"paid"}},
		}

		_, err := record.Validate()
		assert.ErrorContains(t, err, "tag operation")
	})

	t.Run("rejects an invalid list status", func(t *testing.T) {
		record := IngestContact{
			ID:      "contact-1",
			Contact: json.RawMessage(`{"email":"user@example.com"}`),
			ListMemberships: []IngestListMembership{{
				ListID: "newsletter", Status: "disabled",
			}},
		}

		_, err := record.Validate()
		assert.ErrorContains(t, err, "invalid list status")
	})
}

func TestIngestEventValidateRejectsInvalidEmail(t *testing.T) {
	event := IngestEvent{ID: "event-1", Email: "not-an-email", EventName: "order.created", ExternalID: "order-1"}
	_, err := event.Validate(time.Now())
	assert.ErrorContains(t, err, "invalid email")
}
