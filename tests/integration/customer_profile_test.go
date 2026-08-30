package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/app"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/migrations"
	"github.com/hengshu-credit/yaoguang-marketing/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	integrationCustomerNumberPattern       = regexp.MustCompile(`^U[0-9a-z]{3}[0-9]{14}08[0-9a-z]{6}$`)
	integrationLegacyCustomerNumberPattern = regexp.MustCompile(`^U[0-9]{4}[0-9]{14}08[0-9a-f]{32}$`)
)

func integrationWorkspaceCode(sequence uint16) string {
	if sequence <= 999 {
		return fmt.Sprintf("%03d", sequence)
	}
	offset := uint64(sequence - 1000)
	suffix := strconv.FormatUint(offset%(36*36), 36)
	return string(rune('a')+rune(offset/(36*36))) + strings.Repeat("0", 2-len(suffix)) + suffix
}

func TestCustomerProfileAPIIntegration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	user, err := suite.DataFactory.CreateUser()
	require.NoError(t, err)
	workspaceOne, err := suite.DataFactory.CreateWorkspace(testutil.WithWorkspaceName("Customer authority one"))
	require.NoError(t, err)
	workspaceTwo, err := suite.DataFactory.CreateWorkspace(testutil.WithWorkspaceName("Customer authority two"))
	require.NoError(t, err)
	for _, workspace := range []*domain.Workspace{workspaceOne, workspaceTwo} {
		require.NoError(t, suite.DataFactory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	}
	require.NoError(t, suite.APIClient.Login(user.Email, "password"))

	sharedExternalID := "bank-user-shared"
	sharedEmail := "shared.customer@example.com"
	createKnown := func(workspaceID, key string) customerMutationEnvelope {
		response := postCustomer(t, suite.APIClient, "/api/customers.upsert", map[string]interface{}{
			"workspace_id":    workspaceID,
			"idempotency_key": key,
			"customer": map[string]interface{}{
				"external_user_id": sharedExternalID,
				"profile": map[string]interface{}{
					"language":   "zh-CN",
					"timezone":   "Asia/Shanghai",
					"attributes": map[string]interface{}{"set": map[string]interface{}{"tier": "gold"}},
				},
				"identities": []map[string]interface{}{{"type": "email", "value": sharedEmail, "primary": true}},
			},
		}, http.StatusOK)
		var envelope customerMutationEnvelope
		decodeCustomerResponse(t, response, &envelope)
		return envelope
	}

	knownOne := createKnown(workspaceOne.ID, "known-one")
	knownTwo := createKnown(workspaceTwo.ID, "known-two")
	require.NotEqual(t, knownOne.Customer.CustomerID, knownTwo.Customer.CustomerID)
	require.True(t, integrationCustomerNumberPattern.MatchString(knownOne.Customer.CustomerNo))
	require.True(t, integrationCustomerNumberPattern.MatchString(knownTwo.Customer.CustomerNo))
	require.True(t, strings.HasPrefix(knownOne.Customer.CustomerNo, "U"+integrationWorkspaceCode(workspaceOne.Sequence)))
	require.True(t, strings.HasPrefix(knownTwo.Customer.CustomerNo, "U"+integrationWorkspaceCode(workspaceTwo.Sequence)))

	t.Run("workspace isolation and conflicts", func(t *testing.T) {
		response := postCustomer(t, suite.APIClient, "/api/customers.upsert", map[string]interface{}{
			"workspace_id":    workspaceOne.ID,
			"idempotency_key": "duplicate-identity",
			"customer": map[string]interface{}{
				"external_user_id": "another-bank-user",
				"identities":       []map[string]interface{}{{"type": "email", "value": sharedEmail}},
			},
		}, http.StatusConflict)
		var apiError customerErrorEnvelope
		decodeCustomerResponse(t, response, &apiError)
		assert.Equal(t, "external_id_conflict", apiError.Error.Code)

		response = postCustomer(t, suite.APIClient, "/api/customers.upsert", map[string]interface{}{
			"workspace_id":    workspaceOne.ID,
			"idempotency_key": "create-other-customer",
			"customer":        map[string]interface{}{"external_user_id": "identity-conflict-owner"},
		}, http.StatusOK)
		var other customerMutationEnvelope
		decodeCustomerResponse(t, response, &other)
		response = postCustomer(t, suite.APIClient, "/api/customers.upsert", map[string]interface{}{
			"workspace_id":    workspaceOne.ID,
			"idempotency_key": "claim-taken-identity",
			"customer": map[string]interface{}{
				"locator":    map[string]interface{}{"customer_id": other.Customer.CustomerID},
				"identities": []map[string]interface{}{{"type": "email", "value": sharedEmail}},
			},
		}, http.StatusConflict)
		decodeCustomerResponse(t, response, &apiError)
		assert.Equal(t, "identity_conflict", apiError.Error.Code)
	})

	t.Run("concurrent identity claim has one authority owner", func(t *testing.T) {
		type claimResult struct {
			Status int
			Body   []byte
			Err    error
		}
		start := make(chan struct{})
		results := make(chan claimResult, 2)
		var workers sync.WaitGroup
		for index := 0; index < 2; index++ {
			index := index
			workers.Add(1)
			go func() {
				defer workers.Done()
				<-start
				response, requestErr := suite.APIClient.Post("/api/customers.upsert", map[string]interface{}{
					"workspace_id":    workspaceOne.ID,
					"idempotency_key": fmt.Sprintf("concurrent-identity-%d", index),
					"customer": map[string]interface{}{
						"external_user_id": fmt.Sprintf("concurrent-user-%d", index),
						"identities":       []map[string]interface{}{{"type": "email", "value": "concurrent.claim@example.com"}},
					},
				})
				if requestErr != nil {
					results <- claimResult{Err: requestErr}
					return
				}
				defer response.Body.Close()
				body, readErr := io.ReadAll(response.Body)
				results <- claimResult{Status: response.StatusCode, Body: body, Err: readErr}
			}()
		}
		close(start)
		workers.Wait()
		close(results)

		statuses := make([]int, 0, 2)
		for result := range results {
			require.NoError(t, result.Err)
			statuses = append(statuses, result.Status)
			if result.Status == http.StatusConflict {
				var apiError customerErrorEnvelope
				require.NoError(t, json.Unmarshal(result.Body, &apiError))
				assert.Contains(t, []string{"identity_conflict", "external_id_conflict"}, apiError.Error.Code)
			}
		}
		sort.Ints(statuses)
		assert.Equal(t, []int{http.StatusOK, http.StatusConflict}, statuses)

		workspaceDB, dbErr := suite.DBManager.GetWorkspaceDB(workspaceOne.ID)
		require.NoError(t, dbErr)
		var customerCount, consistentProjectionCount int
		require.NoError(t, workspaceDB.QueryRow(`SELECT COUNT(*) FROM customers WHERE external_user_id IN ('concurrent-user-0', 'concurrent-user-1')`).Scan(&customerCount))
		require.NoError(t, workspaceDB.QueryRow(`SELECT COUNT(*) FROM contacts c
			JOIN customer_identities ci ON ci.customer_id = c.customer_id
			WHERE c.email = 'concurrent.claim@example.com' AND ci.identity_type = 'email'`).Scan(&consistentProjectionCount))
		assert.Equal(t, 1, customerCount)
		assert.Equal(t, 1, consistentProjectionCount)
	})

	t.Run("all lookup forms mask identities", func(t *testing.T) {
		locators := []struct {
			name    string
			locator map[string]interface{}
		}{
			{name: "customer_id", locator: map[string]interface{}{"customer_id": knownOne.Customer.CustomerID}},
			{name: "customer_no", locator: map[string]interface{}{"customer_no": knownOne.Customer.CustomerNo}},
			{name: "external_user_id", locator: map[string]interface{}{"external_user_id": sharedExternalID}},
			{name: "identity", locator: map[string]interface{}{"identity": map[string]interface{}{"type": "email", "value": sharedEmail}}},
		}
		for _, locatorCase := range locators {
			t.Run(locatorCase.name, func(t *testing.T) {
				response := postCustomer(t, suite.APIClient, "/api/customers.get", map[string]interface{}{
					"workspace_id": workspaceOne.ID,
					"locator":      locatorCase.locator,
				}, http.StatusOK)
				var envelope customerGetEnvelope
				decodeCustomerResponse(t, response, &envelope)
				assert.Equal(t, knownOne.Customer.CustomerID, envelope.Customer.CustomerID)
				require.Len(t, envelope.Customer.Identities, 1)
				assert.NotEmpty(t, envelope.Customer.Identities[0].DisplayHint)
				assert.NotContains(t, envelope.RawCustomer, `"value"`)
				assert.NotContains(t, envelope.RawCustomer, sharedEmail)
			})
		}
	})

	t.Run("idempotent replay and payload conflict", func(t *testing.T) {
		replay := createKnown(workspaceOne.ID, "known-one")
		assert.True(t, replay.Customer.Replayed)
		response := postCustomer(t, suite.APIClient, "/api/customers.upsert", map[string]interface{}{
			"workspace_id":    workspaceOne.ID,
			"idempotency_key": "known-one",
			"customer": map[string]interface{}{
				"external_user_id": sharedExternalID,
				"profile":          map[string]interface{}{"status": "changed-payload"},
			},
		}, http.StatusConflict)
		var apiError customerErrorEnvelope
		decodeCustomerResponse(t, response, &apiError)
		assert.Equal(t, "idempotency_conflict", apiError.Error.Code)
	})

	t.Run("external ID only customer has no Contact projection", func(t *testing.T) {
		response := postCustomer(t, suite.APIClient, "/api/customers.upsert", map[string]interface{}{
			"workspace_id":    workspaceOne.ID,
			"idempotency_key": "external-only",
			"customer":        map[string]interface{}{"external_user_id": "external-only-user"},
		}, http.StatusOK)
		var envelope customerMutationEnvelope
		decodeCustomerResponse(t, response, &envelope)
		workspaceDB, dbErr := suite.DBManager.GetWorkspaceDB(workspaceOne.ID)
		require.NoError(t, dbErr)
		var contactCount int
		require.NoError(t, workspaceDB.QueryRow(`SELECT COUNT(*) FROM contacts WHERE customer_id = $1`, envelope.Customer.CustomerID).Scan(&contactCount))
		assert.Zero(t, contactCount)
	})

	t.Run("disabled email is not searchable or projected", func(t *testing.T) {
		response := postCustomer(t, suite.APIClient, "/api/customers.upsert", map[string]interface{}{
			"workspace_id":    workspaceOne.ID,
			"idempotency_key": "enabled-then-disabled",
			"customer": map[string]interface{}{
				"external_user_id": "disable-user",
				"identities":       []map[string]interface{}{{"type": "email", "value": "disable.me@example.com", "enabled": true}},
			},
		}, http.StatusOK)
		var created customerMutationEnvelope
		decodeCustomerResponse(t, response, &created)

		response = postCustomer(t, suite.APIClient, "/api/customers.upsert", map[string]interface{}{
			"workspace_id":    workspaceOne.ID,
			"idempotency_key": "disable-existing-email",
			"customer": map[string]interface{}{
				"locator":    map[string]interface{}{"customer_id": created.Customer.CustomerID},
				"identities": []map[string]interface{}{{"type": "email", "value": "disable.me@example.com", "enabled": false}},
			},
		}, http.StatusOK)
		response.Body.Close()

		response = postCustomer(t, suite.APIClient, "/api/customers.get", map[string]interface{}{
			"workspace_id": workspaceOne.ID,
			"locator":      map[string]interface{}{"identity": map[string]interface{}{"type": "email", "value": "disable.me@example.com"}},
		}, http.StatusNotFound)
		response.Body.Close()

		workspaceDB, dbErr := suite.DBManager.GetWorkspaceDB(workspaceOne.ID)
		require.NoError(t, dbErr)
		var enabled, contactLinked bool
		require.NoError(t, workspaceDB.QueryRow(`SELECT enabled FROM customer_identities WHERE customer_id = $1 AND identity_type = 'email'`, created.Customer.CustomerID).Scan(&enabled))
		require.NoError(t, workspaceDB.QueryRow(`SELECT EXISTS (SELECT 1 FROM contacts WHERE email = 'disable.me@example.com' AND customer_id IS NOT NULL)`).Scan(&contactLinked))
		assert.False(t, enabled)
		assert.False(t, contactLinked)
	})

	t.Run("complete ordered batch results", func(t *testing.T) {
		response := postCustomer(t, suite.APIClient, "/api/customers.batch", map[string]interface{}{
			"workspace_id": workspaceOne.ID,
			"items": []map[string]interface{}{
				{"idempotency_key": "batch-a", "customer": map[string]interface{}{"external_user_id": "batch-a"}},
				{"idempotency_key": "", "customer": map[string]interface{}{"external_user_id": "batch-invalid"}},
				{"idempotency_key": "batch-c", "customer": map[string]interface{}{"external_user_id": "batch-c"}},
			},
		}, http.StatusOK)
		var envelope struct {
			Accepted int `json:"accepted"`
			Failed   int `json:"failed"`
			Results  []struct {
				Index  int    `json:"index"`
				Status string `json:"status"`
			} `json:"results"`
		}
		decodeCustomerResponse(t, response, &envelope)
		assert.Equal(t, 2, envelope.Accepted)
		assert.Equal(t, 1, envelope.Failed)
		require.Len(t, envelope.Results, 3)
		for index := range envelope.Results {
			assert.Equal(t, index, envelope.Results[index].Index)
		}
		assert.Equal(t, "accepted", envelope.Results[0].Status)
		assert.Equal(t, "error", envelope.Results[1].Status)
		assert.Equal(t, "accepted", envelope.Results[2].Status)
	})

	t.Run("clean workspace reconciliation has zero gaps", func(t *testing.T) {
		response := postCustomer(t, suite.APIClient, "/api/customers.reconciliation.scan", map[string]interface{}{
			"workspace_id": workspaceTwo.ID,
		}, http.StatusOK)
		var envelope struct {
			Reconciliation domain.CustomerReconciliationRun `json:"reconciliation"`
		}
		decodeCustomerResponse(t, response, &envelope)
		assert.Equal(t, domain.CustomerReconciliationCompleted, envelope.Reconciliation.Status)
		assert.Zero(t, envelope.Reconciliation.MissingCount)
		assert.Zero(t, envelope.Reconciliation.ConflictCount)
		require.NotEmpty(t, envelope.Reconciliation.ID)

		getResponse, getErr := suite.APIClient.Get("/api/customers.reconciliation.get", map[string]string{
			"workspace_id": workspaceTwo.ID,
			"run_id":       envelope.Reconciliation.ID,
		})
		require.NoError(t, getErr)
		require.Equal(t, http.StatusOK, getResponse.StatusCode)
		var persisted struct {
			Reconciliation domain.CustomerReconciliationRun `json:"reconciliation"`
		}
		decodeCustomerResponse(t, getResponse, &persisted)
		assert.Equal(t, envelope.Reconciliation.ID, persisted.Reconciliation.ID)
		assert.Zero(t, persisted.Reconciliation.MissingCount)
		assert.Zero(t, persisted.Reconciliation.ConflictCount)
	})

	t.Run("explicit anonymous to known merge redirects source", func(t *testing.T) {
		anonymousEmail := "anonymous.session@example.com"
		response := postCustomer(t, suite.APIClient, "/api/customers.upsert", map[string]interface{}{
			"workspace_id":    workspaceOne.ID,
			"idempotency_key": "anonymous-source",
			"customer": map[string]interface{}{
				"identities": []map[string]interface{}{
					{"type": "anonymous_id", "value": "browser-session-123"},
					{"type": "email", "value": anonymousEmail, "primary": true},
				},
				"profile": map[string]interface{}{"attributes": map[string]interface{}{"merge": map[string]interface{}{"first_seen_campaign": "spring"}}},
			},
		}, http.StatusOK)
		var anonymous customerMutationEnvelope
		decodeCustomerResponse(t, response, &anonymous)
		workspaceDB, dbErr := suite.DBManager.GetWorkspaceDB(workspaceOne.ID)
		require.NoError(t, dbErr)
		_, err = workspaceDB.Exec(`INSERT INTO customer_consents
			(id, customer_id, purpose, channel, status) VALUES
			('11111111-1111-4111-8111-111111111101', $1, 'marketing', 'email', 'granted'),
			('11111111-1111-4111-8111-111111111102', $2, 'marketing', 'email', 'denied'),
			('11111111-1111-4111-8111-111111111103', $2, 'analytics', 'email', 'granted')`,
			knownOne.Customer.CustomerID, anonymous.Customer.CustomerID)
		require.NoError(t, err)

		mergeRequest := map[string]interface{}{
			"workspace_id":    workspaceOne.ID,
			"idempotency_key": "merge-anonymous-known",
			"source":          map[string]interface{}{"customer_id": anonymous.Customer.CustomerID},
			"target":          map[string]interface{}{"customer_id": knownOne.Customer.CustomerID},
			"reason":          "login identified the browser session",
		}
		mergeResponse := postCustomer(t, suite.APIClient, "/api/customers.merge", mergeRequest, http.StatusOK)
		var merged customerMergeEnvelope
		decodeCustomerResponse(t, mergeResponse, &merged)
		assert.Equal(t, anonymous.Customer.CustomerID, merged.Merge.SourceCustomerID)
		assert.Equal(t, knownOne.Customer.CustomerID, merged.Merge.TargetCustomerID)

		getResponse := postCustomer(t, suite.APIClient, "/api/customers.get", map[string]interface{}{
			"workspace_id": workspaceOne.ID,
			"locator":      map[string]interface{}{"customer_id": anonymous.Customer.CustomerID},
		}, http.StatusOK)
		var resolved customerGetEnvelope
		decodeCustomerResponse(t, getResponse, &resolved)
		assert.Equal(t, knownOne.Customer.CustomerID, resolved.Customer.CustomerID)
		assert.Equal(t, anonymous.Customer.CustomerID, resolved.Customer.ResolvedFromCustomerID)

		var targetContactCount, detachedSourceContactCount int
		require.NoError(t, workspaceDB.QueryRow(`SELECT COUNT(*) FROM contacts WHERE customer_id = $1`, knownOne.Customer.CustomerID).Scan(&targetContactCount))
		require.NoError(t, workspaceDB.QueryRow(`SELECT COUNT(*) FROM contacts WHERE email = $1 AND customer_id IS NULL`, anonymousEmail).Scan(&detachedSourceContactCount))
		assert.Equal(t, 1, targetContactCount)
		assert.Equal(t, 1, detachedSourceContactCount)
		var sourceConsentCount int
		var marketingStatus, analyticsStatus string
		require.NoError(t, workspaceDB.QueryRow(`SELECT COUNT(*) FROM customer_consents WHERE customer_id = $1`, anonymous.Customer.CustomerID).Scan(&sourceConsentCount))
		require.NoError(t, workspaceDB.QueryRow(`SELECT status FROM customer_consents WHERE customer_id = $1 AND purpose = 'marketing' AND channel = 'email'`, knownOne.Customer.CustomerID).Scan(&marketingStatus))
		require.NoError(t, workspaceDB.QueryRow(`SELECT status FROM customer_consents WHERE customer_id = $1 AND purpose = 'analytics' AND channel = 'email'`, knownOne.Customer.CustomerID).Scan(&analyticsStatus))
		assert.Zero(t, sourceConsentCount)
		assert.Equal(t, "granted", marketingStatus)
		assert.Equal(t, "granted", analyticsStatus)

		previousCustomerNo := fmt.Sprintf("U%04d2026083015304508%s", workspaceOne.Sequence,
			strings.ReplaceAll(knownOne.Customer.CustomerID, "-", ""))
		_, err = workspaceDB.Exec(`UPDATE customers SET customer_no = $1 WHERE id = $2`, previousCustomerNo, knownOne.Customer.CustomerID)
		require.NoError(t, err)
		_, err = workspaceDB.Exec(`UPDATE customer_idempotency SET response = CASE
			WHEN operation = 'customer.upsert' THEN jsonb_set(response, '{customer_no}', to_jsonb($1::text), false)
			WHEN operation = 'customer.merge' THEN jsonb_set(response, '{target_customer_no}', to_jsonb($1::text), false)
			ELSE response END
			WHERE (operation = 'customer.upsert' AND idempotency_key = 'known-one')
				OR (operation = 'customer.merge' AND idempotency_key = 'merge-anonymous-known')`, previousCustomerNo)
		require.NoError(t, err)

		migrationTx, txErr := workspaceDB.BeginTx(context.Background(), nil)
		require.NoError(t, txErr)
		require.NoError(t, (&migrations.V51Migration{}).UpdateWorkspace(context.Background(), suite.Config, workspaceOne, migrationTx))
		require.NoError(t, migrationTx.Commit())
		var authoritativeCustomerNo string
		require.NoError(t, workspaceDB.QueryRow(`SELECT customer_no FROM customers WHERE id = $1`, knownOne.Customer.CustomerID).
			Scan(&authoritativeCustomerNo))
		assert.NotEqual(t, previousCustomerNo, authoritativeCustomerNo)

		replayedUpsert := createKnown(workspaceOne.ID, "known-one")
		assert.True(t, replayedUpsert.Customer.Replayed)
		assert.Equal(t, authoritativeCustomerNo, replayedUpsert.Customer.CustomerNo)
		replayedMergeResponse := postCustomer(t, suite.APIClient, "/api/customers.merge", mergeRequest, http.StatusOK)
		var replayedMerge customerMergeEnvelope
		decodeCustomerResponse(t, replayedMergeResponse, &replayedMerge)
		assert.True(t, replayedMerge.Merge.Replayed)
		assert.Equal(t, authoritativeCustomerNo, replayedMerge.Merge.TargetCustomerNo)
	})

	t.Run("legacy Contact backfill", func(t *testing.T) {
		legacyWorkspace, createErr := suite.DataFactory.CreateWorkspace(testutil.WithWorkspaceName("V46 backfill"))
		require.NoError(t, createErr)
		legacyEmail := "legacy.customer@example.com"
		_, createErr = suite.DataFactory.CreateContact(legacyWorkspace.ID,
			testutil.WithContactEmail(legacyEmail),
			testutil.WithContactExternalID("legacy-external"),
		)
		require.NoError(t, createErr)
		workspaceDB, dbErr := suite.DBManager.GetWorkspaceDB(legacyWorkspace.ID)
		require.NoError(t, dbErr)

		migration := &migrations.V46Migration{}
		require.NoError(t, migration.UpdateWorkspace(context.Background(), suite.Config, legacyWorkspace, workspaceDB))

		var customerID, customerNo, ciphertext, fingerprint, displayHint string
		require.NoError(t, workspaceDB.QueryRow(`SELECT c.customer_id, cu.customer_no, ci.value_ciphertext, ci.lookup_fingerprint, ci.display_hint
			FROM contacts c
			JOIN customers cu ON cu.id = c.customer_id
			JOIN customer_identities ci ON ci.customer_id = cu.id AND ci.identity_type = 'email'
			WHERE c.email = $1`, legacyEmail).Scan(&customerID, &customerNo, &ciphertext, &fingerprint, &displayHint))
		assert.NotEmpty(t, customerID)
		assert.True(t, integrationLegacyCustomerNumberPattern.MatchString(customerNo))
		assert.NotEqual(t, legacyEmail, ciphertext)
		assert.Len(t, fingerprint, 64)
		assert.NotEqual(t, legacyEmail, displayHint)
		var profileCount int
		require.NoError(t, workspaceDB.QueryRow(`SELECT COUNT(*) FROM customer_profiles WHERE customer_id = $1`, customerID).Scan(&profileCount))
		assert.Equal(t, 1, profileCount)

		migrationTx, txErr := workspaceDB.BeginTx(context.Background(), nil)
		require.NoError(t, txErr)
		require.NoError(t, (&migrations.V51Migration{}).UpdateWorkspace(context.Background(), suite.Config, legacyWorkspace, migrationTx))
		require.NoError(t, migrationTx.Commit())
		var regeneratedCustomerID, regeneratedCustomerNo string
		require.NoError(t, workspaceDB.QueryRow(`SELECT id, customer_no FROM customers WHERE id = $1`, customerID).
			Scan(&regeneratedCustomerID, &regeneratedCustomerNo))
		assert.Equal(t, customerID, regeneratedCustomerID)
		assert.NotEqual(t, customerNo, regeneratedCustomerNo)
		assert.True(t, integrationCustomerNumberPattern.MatchString(regeneratedCustomerNo))
		assert.True(t, strings.HasPrefix(regeneratedCustomerNo, "U"+integrationWorkspaceCode(legacyWorkspace.Sequence)))
	})

	t.Run("legacy backfill rejects whitespace-normalized external ID collision", func(t *testing.T) {
		legacyWorkspace, createErr := suite.DataFactory.CreateWorkspace(testutil.WithWorkspaceName("V46 trimmed external conflict"))
		require.NoError(t, createErr)
		_, createErr = suite.DataFactory.CreateContact(legacyWorkspace.ID,
			testutil.WithContactEmail("trimmed.one@example.com"), testutil.WithContactExternalID("crm-trimmed"))
		require.NoError(t, createErr)
		_, createErr = suite.DataFactory.CreateContact(legacyWorkspace.ID,
			testutil.WithContactEmail("trimmed.two@example.com"), testutil.WithContactExternalID(" crm-trimmed "))
		require.NoError(t, createErr)
		workspaceDB, dbErr := suite.DBManager.GetWorkspaceDB(legacyWorkspace.ID)
		require.NoError(t, dbErr)

		migrationErr := (&migrations.V46Migration{}).UpdateWorkspace(context.Background(), suite.Config, legacyWorkspace, workspaceDB)
		assert.ErrorContains(t, migrationErr, `duplicate external user ID "crm-trimmed"`)
	})
}

type customerMutationEnvelope struct {
	RequestID string `json:"request_id"`
	Customer  struct {
		CustomerID string `json:"customer_id"`
		CustomerNo string `json:"customer_no"`
		Replayed   bool   `json:"replayed"`
	} `json:"customer"`
}

type customerGetEnvelope struct {
	RequestID string `json:"request_id"`
	Customer  struct {
		CustomerID             string `json:"customer_id"`
		ResolvedFromCustomerID string `json:"resolved_from_customer_id"`
		Identities             []struct {
			DisplayHint string `json:"display_hint"`
		} `json:"identities"`
	} `json:"customer"`
	RawCustomer string `json:"-"`
}

type customerMergeEnvelope struct {
	Merge struct {
		SourceCustomerID string `json:"source_customer_id"`
		TargetCustomerID string `json:"target_customer_id"`
		TargetCustomerNo string `json:"target_customer_no"`
		Replayed         bool   `json:"replayed"`
	} `json:"merge"`
}

type customerErrorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func postCustomer(t *testing.T, client *testutil.APIClient, path string, body interface{}, expectedStatus int) *http.Response {
	t.Helper()
	response, err := client.Post(path, body)
	require.NoError(t, err)
	if response.StatusCode != expectedStatus {
		payload, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		t.Fatalf("POST %s returned %d, want %d: %s", path, response.StatusCode, expectedStatus, payload)
	}
	return response
}

func decodeCustomerResponse(t *testing.T, response *http.Response, target interface{}) {
	t.Helper()
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	if envelope, ok := target.(*customerGetEnvelope); ok {
		var raw struct {
			Customer json.RawMessage `json:"customer"`
		}
		require.NoError(t, json.Unmarshal(payload, &raw))
		envelope.RawCustomer = string(raw.Customer)
	}
	require.NoError(t, json.Unmarshal(payload, target), "response: %s", payload)
}
