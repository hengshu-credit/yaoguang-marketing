package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBroadcastExcludesPendingDoubleOptIn_Integration is a regression test for issue #344.
//
// When a list has double opt-in enabled, a contact whose contact_list.status is
// 'pending' has clicked the sign-up form but never confirmed via the confirmation
// email. Such a contact must NOT be a broadcast recipient.
//
// The broadcast recipient set is computed by ContactRepository.CountContactsForBroadcast
// and ContactRepository.GetContactsForBroadcast, so this test drives those two methods
// directly against a real Postgres database (the same path the broadcast orchestrator
// uses in BroadcastOrchestrator.GetTotalRecipientCount / FetchBatch).
//
// Against the buggy code the pending contact leaks in (count==2, both returned).
// With the double opt-in guard it is excluded (count==1, only the confirmed contact).
func TestBroadcastExcludesPendingDoubleOptIn_Integration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	factory := suite.DataFactory
	contactRepo := suite.ServerManager.GetApp().GetContactRepository()
	ctx := context.Background()

	// Standard setup: user + workspace.
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// A double opt-in list — the configuration described in the bug report.
	doiList, err := factory.CreateList(workspace.ID, testutil.WithListDoubleOptin(true))
	require.NoError(t, err)

	ts := time.Now().UnixNano()

	// Confirmed subscriber (completed double opt-in) → must receive the broadcast.
	confirmedEmail := fmt.Sprintf("confirmed-%d@example.com", ts)
	_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(confirmedEmail))
	require.NoError(t, err)
	_, err = factory.CreateContactList(workspace.ID,
		testutil.WithContactListEmail(confirmedEmail),
		testutil.WithContactListListID(doiList.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive))
	require.NoError(t, err)

	// Pending subscriber (never confirmed double opt-in) → must NOT receive the broadcast.
	pendingEmail := fmt.Sprintf("pending-%d@example.com", ts)
	_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(pendingEmail))
	require.NoError(t, err)
	_, err = factory.CreateContactList(workspace.ID,
		testutil.WithContactListEmail(pendingEmail),
		testutil.WithContactListListID(doiList.ID),
		testutil.WithContactListStatus(domain.ContactListStatusPending))
	require.NoError(t, err)

	// Mirror a real campaign audience targeting the list. Note that ExcludeUnsubscribed
	// only filters unsubscribed/bounced/complained — it does NOT cover 'pending', which is
	// exactly why the bug slips through even for a normally-configured broadcast.
	audience := domain.AudienceSettings{
		List:                doiList.ID,
		ExcludeUnsubscribed: true,
	}

	t.Run("CountContactsForBroadcast excludes the pending contact", func(t *testing.T) {
		count, err := contactRepo.CountContactsForBroadcast(ctx, workspace.ID, audience)
		require.NoError(t, err)
		assert.Equal(t, 1, count,
			"issue #344: a pending (unconfirmed) contact on a double opt-in list must not be counted as a recipient")
	})

	t.Run("GetContactsForBroadcast excludes the pending contact", func(t *testing.T) {
		contacts, err := contactRepo.GetContactsForBroadcast(ctx, workspace.ID, audience, 100, "")
		require.NoError(t, err)

		emails := make([]string, 0, len(contacts))
		for _, c := range contacts {
			emails = append(emails, c.Contact.Email)
		}

		assert.Contains(t, emails, confirmedEmail, "the confirmed contact must be a recipient")
		assert.NotContains(t, emails, pendingEmail,
			"issue #344: the pending (unconfirmed) contact must be excluded from broadcast recipients")
		assert.Len(t, emails, 1, "only the confirmed contact should be returned")
	})

	// Control: a single opt-in (non-DOI) list must be unaffected by the guard. Even a
	// 'pending' row (an unusual state for a non-DOI list) is still eligible, because the
	// guard is a no-op when l.is_double_optin = false.
	t.Run("non-double-opt-in list is unaffected by the guard", func(t *testing.T) {
		singleList, err := factory.CreateList(workspace.ID, testutil.WithListDoubleOptin(false))
		require.NoError(t, err)

		soiEmail := fmt.Sprintf("soi-active-%d@example.com", ts)
		_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(soiEmail))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(soiEmail),
			testutil.WithContactListListID(singleList.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		soiAudience := domain.AudienceSettings{List: singleList.ID, ExcludeUnsubscribed: true}
		count, err := contactRepo.CountContactsForBroadcast(ctx, workspace.ID, soiAudience)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "the confirmed contact on a single opt-in list must still be counted")
	})
}
