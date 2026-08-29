package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/app"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/migrations"
	"github.com/hengshu-credit/yaoguang-marketing/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var integrationCustomerNumberPattern = regexp.MustCompile(`^U[0-9]{4}[0-9]{14}08[0-9a-f]{32}$`)

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
	require.Contains(t, knownOne.Customer.CustomerNo, fmt.Sprintf("U%04d", workspaceOne.Sequence))
	require.Contains(t, knownTwo.Customer.CustomerNo, fmt.Sprintf("U%04d", workspaceTwo.Sequence))

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

	t.Run("explicit anonymous to known merge redirects source", func(t *testing.T) {
		response := postCustomer(t, suite.APIClient, "/api/customers.upsert", map[string]interface{}{
			"workspace_id":    workspaceOne.ID,
			"idempotency_key": "anonymous-source",
			"customer": map[string]interface{}{
				"identities": []map[string]interface{}{{"type": "anonymous_id", "value": "browser-session-123"}},
				"profile":    map[string]interface{}{"attributes": map[string]interface{}{"merge": map[string]interface{}{"first_seen_campaign": "spring"}}},
			},
		}, http.StatusOK)
		var anonymous customerMutationEnvelope
		decodeCustomerResponse(t, response, &anonymous)

		mergeResponse := postCustomer(t, suite.APIClient, "/api/customers.merge", map[string]interface{}{
			"workspace_id":    workspaceOne.ID,
			"idempotency_key": "merge-anonymous-known",
			"source":          map[string]interface{}{"customer_id": anonymous.Customer.CustomerID},
			"target":          map[string]interface{}{"customer_id": knownOne.Customer.CustomerID},
			"reason":          "login identified the browser session",
		}, http.StatusOK)
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
		assert.True(t, integrationCustomerNumberPattern.MatchString(customerNo))
		assert.NotEqual(t, legacyEmail, ciphertext)
		assert.Len(t, fingerprint, 64)
		assert.NotEqual(t, legacyEmail, displayHint)
		var profileCount int
		require.NoError(t, workspaceDB.QueryRow(`SELECT COUNT(*) FROM customer_profiles WHERE customer_id = $1`, customerID).Scan(&profileCount))
		assert.Equal(t, 1, profileCount)
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
