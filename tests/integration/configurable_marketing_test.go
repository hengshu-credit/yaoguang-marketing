//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/repository"
	"github.com/hengshu-credit/yaoguang-marketing/tests/testutil"
	"github.com/stretchr/testify/require"
)

// Exercise the actual authenticated application and freshly initialized workspace,
// including repeated occurrence IDs, pending resolution and canonical alias merges.
func TestConfigurableMarketingHTTPIntegration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()
	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()
	owner, err := suite.DataFactory.CreateUser()
	require.NoError(t, err)
	member, err := suite.DataFactory.CreateUser()
	require.NoError(t, err)
	w, err := suite.DataFactory.CreateWorkspace(testutil.WithWorkspaceName("Configurable marketing HTTP"))
	require.NoError(t, err)
	require.NoError(t, suite.DataFactory.AddUserToWorkspace(owner.ID, w.ID, "owner"))
	permissions := domain.UserPermissions{}
	require.NoError(t, json.Unmarshal([]byte(`{"contacts":{"read":true,"write":true}}`), &permissions))
	require.NoError(t, suite.DataFactory.AddUserToWorkspaceWithPermissions(member.ID, w.ID, "member", permissions))
	client := suite.APIClient
	require.NoError(t, client.Login(owner.Email, "password"))
	post := func(path string, body any, status int, target any) {
		t.Helper()
		resp := postCustomer(t, client, path, body, status)
		if target != nil {
			decodeCustomerResponse(t, resp, target)
		} else {
			resp.Body.Close()
		}
	}
	save := func(kind, key string, settings map[string]any) map[string]any {
		return map[string]any{"workspace_id": w.ID, "definition": map[string]any{"kind": kind, "key": key, "name": key, "status": "active", "schema": map[string]any{"type": "object"}, "settings": settings}}
	}
	object := save("object_type", "contract", map[string]any{})
	post("/api/businessDefinitions.save", object, http.StatusOK, nil)
	event := save("event", "contract.signed", map[string]any{"subject_type": "contract", "subject_id_path": "$.data.contract_id", "expiry_mode": "never", "unmatched_customer": "create"})
	post("/api/businessDefinitions.save", event, http.StatusOK, nil)
	post("/api/businessDefinitions.save", save("event", "application.pending", map[string]any{"subject_type": "customer", "expiry_mode": "never", "unmatched_customer": "pending"}), http.StatusOK, nil)
	track := func(report, code, external string) map[string]any {
		return map[string]any{"workspace_id": w.ID, "source": "http-test", "event": map[string]any{"report_id": report, "event_code": code, "customer_attributes": map[string]any{"external_user_id": external}, "data": map[string]any{"contract_id": "C-1"}, "triggered_at": time.Now().UTC().Format(time.RFC3339Nano)}}
	}
	firstBody := track("sign-1", "contract.signed", "person-one")
	var first, second, retry domain.TrackedEventResult
	post("/api/events.track", firstBody, http.StatusOK, &first)
	require.NotEmpty(t, first.CustomerID)
	post("/api/events.track", track("sign-2", "contract.signed", "person-one"), http.StatusOK, &second)
	require.Equal(t, first.CustomerID, second.CustomerID)
	require.NotEqual(t, first.EventID, second.EventID)
	post("/api/events.track", firstBody, http.StatusOK, &retry)
	require.True(t, retry.Duplicate)
	require.Equal(t, first.EventID, retry.EventID)
	resp, err := client.Get("/api/events.list", map[string]string{"workspace_id": w.ID, "customer_id": first.CustomerID})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var timeline struct {
		Events []domain.TrackedEventResult `json:"events"`
	}
	decodeCustomerResponse(t, resp, &timeline)
	require.Len(t, timeline.Events, 2)
	resp, err = client.Get("/api/businessObjects.list", map[string]string{"workspace_id": w.ID, "customer_id": first.CustomerID, "object_type": "contract"})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var objects struct {
		Objects []domain.BusinessObject `json:"objects"`
	}
	decodeCustomerResponse(t, resp, &objects)
	require.Len(t, objects.Objects, 1)
	require.Equal(t, first.CustomerID, objects.Objects[0].CustomerID)
	var pending domain.TrackedEventResult
	post("/api/events.track", track("pending-1", "application.pending", "unbound-alias"), http.StatusOK, &pending)
	require.Equal(t, "pending_identity", pending.ProcessingStatus)
	resolve := map[string]any{"workspace_id": w.ID, "source": "http-test", "report_id": "pending-1", "customer_id": first.CustomerID, "reason": "Verified application identity"}
	require.NoError(t, client.Login(member.Email, "password"))
	post("/api/businessDefinitions.save", save("object_type", "blocked", map[string]any{}), http.StatusForbidden, nil)
	post("/api/events.resolvePending", resolve, http.StatusForbidden, nil)
	var allowed domain.TrackedEventResult
	post("/api/events.track", track("member-sign", "contract.signed", "person-one"), http.StatusOK, &allowed)
	require.NoError(t, client.Login(owner.Email, "password"))
	var resolved domain.TrackedEventResult
	post("/api/events.resolvePending", resolve, http.StatusOK, &resolved)
	require.Equal(t, pending.EventID, resolved.EventID)
	require.Equal(t, pending.ReportedAt, resolved.ReportedAt)
	require.Equal(t, first.CustomerID, resolved.CustomerID)
	require.Equal(t, "accepted", resolved.ProcessingStatus)
	var alias domain.TrackedEventResult
	post("/api/events.track", track("alias-event", "application.pending", "unbound-alias"), http.StatusOK, &alias)
	require.Equal(t, first.CustomerID, alias.CustomerID)
	var other customerMutationEnvelope
	post("/api/customers.upsert", map[string]any{"workspace_id": w.ID, "idempotency_key": "other-person", "customer": map[string]any{"external_user_id": "person-two"}}, http.StatusOK, &other)
	var beforeMerge domain.TrackedEventResult
	post("/api/events.track", track("source-before-merge", "application.pending", "person-two"), http.StatusOK, &beforeMerge)
	mergeDB, err := suite.DataFactory.GetWorkspaceDB(w.ID)
	require.NoError(t, err)
	funnelRepo := repository.NewFunnelRepositoryWithDB(mergeDB)
	beforeFacts, err := funnelRepo.EventsForSubject(context.Background(), w.ID, "customer", other.Customer.CustomerID, time.Now().Add(-time.Hour), time.Now().Add(time.Minute), time.Now().Add(time.Minute))
	require.NoError(t, err)
	foundBefore := false
	var beforeReceived time.Time
	for _, fact := range beforeFacts {
		if fact.ID == beforeMerge.EventID {
			foundBefore = true
			beforeReceived = fact.ReceivedAt
		}
	}
	require.True(t, foundBefore)
	_, err = mergeDB.Exec(`INSERT INTO journey_event_subjects(contact_automation_id,subject_type,subject_id,customer_id,origin_event_id) VALUES('merge-binding','customer',$1::text,$1::uuid,$2)`, other.Customer.CustomerID, beforeMerge.EventID)
	require.NoError(t, err)
	post("/api/customers.merge", map[string]any{"workspace_id": w.ID, "idempotency_key": "merge-known", "source": map[string]any{"customer_id": other.Customer.CustomerID}, "target": map[string]any{"customer_id": first.CustomerID}, "reason": "Confirmed duplicate external account"}, http.StatusOK, nil)
	afterFacts, err := funnelRepo.EventsForSubject(context.Background(), w.ID, "customer", first.CustomerID, time.Now().Add(-time.Hour), time.Now().Add(time.Minute), time.Now().Add(time.Minute))
	require.NoError(t, err)
	foundAfter := false
	for _, fact := range afterFacts {
		if fact.ID == beforeMerge.EventID {
			foundAfter = true
			require.True(t, fact.ReceivedAt.Equal(beforeReceived))
		}
	}
	require.True(t, foundAfter, "canonical target funnel retains source occurrence")
	var boundCustomer, boundSubject string
	require.NoError(t, mergeDB.QueryRow(`SELECT customer_id::text,subject_id FROM journey_event_subjects WHERE contact_automation_id='merge-binding'`).Scan(&boundCustomer, &boundSubject))
	require.Equal(t, first.CustomerID, boundCustomer)
	require.Equal(t, first.CustomerID, boundSubject)
	var merged domain.TrackedEventResult
	post("/api/events.track", track("merged-event", "application.pending", "person-two"), http.StatusOK, &merged)
	require.Equal(t, first.CustomerID, merged.CustomerID)
	var aliasObject struct {
		Object domain.BusinessObject `json:"object"`
	}
	post("/api/businessObjects.upsert", map[string]any{"workspace_id": w.ID, "object": map[string]any{"object_type": "contract", "source": "http-test", "external_key": "C-via-old-uuid", "customer_id": other.Customer.CustomerID, "attributes": map[string]any{}}}, http.StatusOK, &aliasObject)
	require.Equal(t, first.CustomerID, aliasObject.Object.CustomerID)
	workspaceDB, err := suite.DataFactory.GetWorkspaceDB(w.ID)
	require.NoError(t, err)
	var ledgerCustomer, ledgerSubject, outboxCustomer string
	require.NoError(t, workspaceDB.QueryRow(`SELECT customer_id::text,subject_id FROM event_ledger WHERE id=$1`, beforeMerge.EventID).Scan(&ledgerCustomer, &ledgerSubject))
	require.Equal(t, first.CustomerID, ledgerCustomer)
	require.Equal(t, first.CustomerID, ledgerSubject)
	require.NoError(t, workspaceDB.QueryRow(`SELECT payload->'subject'->>'customer_id' FROM event_outbox WHERE event_id=$1 LIMIT 1`, beforeMerge.EventID).Scan(&outboxCustomer))
	require.Equal(t, first.CustomerID, outboxCustomer)
	var contacts, facts int
	require.NoError(t, workspaceDB.QueryRow(`SELECT COUNT(*) FROM contacts WHERE customer_id=$1`, first.CustomerID).Scan(&contacts))
	require.Zero(t, contacts, "no email contact is fabricated")
	require.NoError(t, workspaceDB.QueryRow(`SELECT COUNT(*) FROM event_ledger WHERE properties->>'report_id' IN ('sign-1','sign-2')`).Scan(&facts))
	require.Equal(t, 2, facts)
	// A read-only member cannot ingest an occurrence.
	_, err = suite.DBManager.GetDB().Exec(`UPDATE user_workspaces SET permissions='{"contacts":{"read":true}}'::jsonb WHERE user_id=$1 AND workspace_id=$2`, member.ID, w.ID)
	require.NoError(t, err)
	require.NoError(t, client.Login(member.Email, "password"))
	post("/api/events.track", track(fmt.Sprintf("denied-%d", time.Now().UnixNano()), "contract.signed", "person-one"), http.StatusForbidden, nil)
	if fixturePath := os.Getenv("CONFIGURABLE_BROWSER_FIXTURE"); fixturePath != "" {
		require.NoError(t, client.Login(owner.Email, "password"))
		holdConfigurableBrowserFixture(t, suite, fixturePath, w.ID, first.CustomerID)
	}
}

// Opt-in private-network bridge for real browser verification. Its credentials
// belong only to this disposable integration database; ordinary tests never listen.
func holdConfigurableBrowserFixture(t *testing.T, suite *testutil.IntegrationTestSuite, path, workspace, customer string) {
	t.Helper()
	target, err := url.Parse(suite.ServerManager.GetURL())
	require.NoError(t, err)
	stop := make(chan struct{})
	var once sync.Once
	mux := http.NewServeMux()
	mux.HandleFunc("/__fixture/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		once.Do(func() { close(stop) })
		w.WriteHeader(http.StatusNoContent)
	})
	mux.Handle("/", httputil.NewSingleHostReverseProxy(target))
	listener, err := net.Listen("tcp", "0.0.0.0:18580")
	require.NoError(t, err)
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = server.Serve(listener) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	payload, err := json.Marshal(map[string]string{"workspace_id": workspace, "customer_id": customer, "auth_token": suite.APIClient.GetToken(), "api_url": "http://notifuse-configurable-go:18580"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, payload, 0600))
	defer os.Remove(path)
	t.Log("Browser fixture ready; waiting for stop endpoint or 15 minute deadline")
	select {
	case <-stop:
	case <-time.After(15 * time.Minute):
		t.Error("browser fixture timed out")
	}
}
