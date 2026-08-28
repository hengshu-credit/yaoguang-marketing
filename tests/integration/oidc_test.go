package integration

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// ---------------------------------------------------------------------------
// Fake OpenID Provider (discovery + JWKS + token endpoint), served over http.
// The /token endpoint returns whatever id_token the test currently stages, so a
// test can mint a token with an arbitrary nonce/sub/email and observe how the
// real Notifuse OIDC pipeline reacts.
// ---------------------------------------------------------------------------

const oidcTestClientID = "integration-client"

type fakeOIDP struct {
	server  *httptest.Server
	key     *rsa.PrivateKey
	kid     string
	idToken string // staged id_token returned by the next /token exchange
}

func newFakeOIDP(t *testing.T) *fakeOIDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	f := &fakeOIDP{key: key, kid: "itest-key"}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                f.server.URL,
			"authorization_endpoint":                f.server.URL + "/authorize",
			"token_endpoint":                        f.server.URL + "/token",
			"jwks_uri":                              f.server.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		pub := f.key.PublicKey
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "RSA", "alg": "RS256", "use": "sig", "kid": f.kid,
				"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			}},
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "itest-access-token",
			"token_type":   "Bearer",
			"id_token":     f.idToken,
		})
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

// mintIDToken signs an RS256 ID token with the given claims (iss/aud default to
// this provider/client unless overridden).
func (f *fakeOIDP) mintIDToken(t *testing.T, sub, email string, emailVerified bool, nonce string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss":            f.server.URL,
		"aud":            oidcTestClientID,
		"sub":            sub,
		"email":          email,
		"email_verified": emailVerified,
		"nonce":          nonce,
		"iat":            time.Now().Unix(),
		"exp":            time.Now().Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = f.kid
	signed, err := tok.SignedString(f.key)
	require.NoError(t, err)
	return signed
}

// noRedirectClient returns a client that surfaces 3xx responses instead of
// following them, so the test can inspect Location headers and Set-Cookie.
func noRedirectClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// oidcCallbackOutcome is the parsed result of driving /start then /callback.
type oidcCallbackOutcome struct {
	code      string // one-time code from the success fragment, "" on failure
	oidcError string // ?oidc_error reason on failure, "" on success
}

// driveLogin performs the real browser-side OIDC handoff against the running app:
// GET /start (capture flow cookie + state/nonce from the 302 to the IdP), stage an
// id_token with the given identity, then GET /callback and parse the result. When
// tamperNonce is true a deliberately-wrong nonce is used (replay/nonce-binding test).
func driveLogin(t *testing.T, base string, idp *fakeOIDP, sub, email string, emailVerified, tamperNonce bool) oidcCallbackOutcome {
	t.Helper()
	client := noRedirectClient()

	// 1. /start -> 302 to IdP authorize URL, carrying state+nonce as query params.
	startResp, err := client.Get(base + "/api/user.oidc.start")
	require.NoError(t, err)
	defer startResp.Body.Close()
	require.Equal(t, http.StatusFound, startResp.StatusCode, "/start must 302 to the IdP")

	var flowCookie *http.Cookie
	for _, c := range startResp.Cookies() {
		if c.Name == "oidc_flow" { // non-__Host- in dev/http (APIEndpoint is non-https in tests)
			flowCookie = c
		}
	}
	require.NotNil(t, flowCookie, "/start must set the flow-state cookie")

	authURL, err := url.Parse(startResp.Header.Get("Location"))
	require.NoError(t, err)
	state := authURL.Query().Get("state")
	nonce := authURL.Query().Get("nonce")
	require.NotEmpty(t, state)
	require.NotEmpty(t, nonce)

	// 2. Stage the id_token the IdP will return on code exchange.
	tokenNonce := nonce
	if tamperNonce {
		tokenNonce = "attacker-supplied-wrong-nonce"
	}
	idp.idToken = idp.mintIDToken(t, sub, email, emailVerified, tokenNonce)

	// 3. /callback with the matching state + flow cookie.
	cbReq, err := http.NewRequest(http.MethodGet, base+"/api/user.oidc.callback?code=itest-code&state="+url.QueryEscape(state), nil)
	require.NoError(t, err)
	cbReq.AddCookie(flowCookie)
	cbResp, err := client.Do(cbReq)
	require.NoError(t, err)
	defer cbResp.Body.Close()
	require.Equal(t, http.StatusFound, cbResp.StatusCode, "/callback must always redirect, never 500")

	loc, err := url.Parse(cbResp.Header.Get("Location"))
	require.NoError(t, err)
	out := oidcCallbackOutcome{oidcError: loc.Query().Get("oidc_error")}
	// success code rides the URL fragment: #oidc_code=...
	if frag := loc.Fragment; strings.HasPrefix(frag, "oidc_code=") {
		out.code = strings.TrimPrefix(frag, "oidc_code=")
	}
	return out
}

// exchangeCode POSTs the one-time code and returns the minted session JWT.
func exchangeCode(t *testing.T, base, code string) (token string, status int) {
	t.Helper()
	resp, err := http.Post(base+"/api/user.oidc.exchange", "application/json",
		strings.NewReader(fmt.Sprintf(`{"code":%q}`, code)))
	require.NoError(t, err)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", resp.StatusCode
	}
	var body struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	return body.Token, resp.StatusCode
}

// meEmail calls /api/user.me with a bearer token and returns (userID, email, status).
func meIdentity(t *testing.T, base, token string) (userID, email string, status int) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, base+"/api/user.me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", resp.StatusCode
	}
	var body struct {
		User struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"user"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	return body.User.ID, body.User.Email, resp.StatusCode
}

func TestOIDCIntegration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	idp := newFakeOIDP(t)

	// appFactory variant: enable OIDC (invited-only) pointing at the fake IdP.
	oidcAppFactory := func(cfg *config.Config) testutil.AppInterface {
		cfg.OIDC = config.OIDCConfig{
			Enabled:         true,
			IssuerURL:       idp.server.URL,
			ClientID:        oidcTestClientID,
			ClientSecret:    "integration-secret",
			RedirectURI:     "http://localhost/api/user.oidc.callback",
			Scopes:          []string{"openid", "email", "profile"},
			ButtonLabel:     "Sign in with SSO",
			AutoCreateUsers: false, // invited-only
		}
		return app.NewApp(cfg)
	}

	suite := testutil.NewIntegrationTestSuite(t, oidcAppFactory)
	defer func() { suite.Cleanup() }()

	base := suite.ServerManager.GetURL()
	db := suite.DBManager.GetDB()
	issuer := idp.server.URL

	countFedIdentities := func(sub string) int {
		var n int
		require.NoError(t, db.QueryRow(
			`SELECT count(*) FROM federated_identities WHERE idp_issuer=$1 AND idp_sub=$2`, issuer, sub).Scan(&n))
		return n
	}
	countUsersByEmail := func(email string) int {
		var n int
		require.NoError(t, db.QueryRow(
			`SELECT count(*) FROM users WHERE lower(email)=lower($1)`, email).Scan(&n))
		return n
	}
	countMemberships := func(userID string) int {
		var n int
		require.NoError(t, db.QueryRow(
			`SELECT count(*) FROM user_workspaces WHERE user_id=$1`, userID).Scan(&n))
		return n
	}

	// existing-user@example.com is seeded as a real user but joins no workspace.
	const invitedEmail = "existing-user@example.com"

	t.Run("happy path: invited user logs in via SSO end-to-end", func(t *testing.T) {
		out := driveLogin(t, base, idp, "sub-happy", invitedEmail, true, false)
		require.Empty(t, out.oidcError, "expected success, got error %q", out.oidcError)
		require.NotEmpty(t, out.code, "expected a one-time code in the success fragment")

		token, status := exchangeCode(t, base, out.code)
		require.Equal(t, http.StatusOK, status)
		require.NotEmpty(t, token, "exchange must return a session JWT")

		// The OIDC-minted JWT must be byte-compatible with the real middleware.
		uid, gotEmail, meStatus := meIdentity(t, base, token)
		require.Equal(t, http.StatusOK, meStatus, "OIDC-minted token must authenticate /api/user.me")
		assert.Equal(t, invitedEmail, gotEmail)
		assert.NotEmpty(t, uid)

		// The federated identity link was persisted in Postgres (real repo, not a mock).
		assert.Equal(t, 1, countFedIdentities("sub-happy"), "a federated_identities row must exist")

		// login != authorization: this user joined no workspace, so a workspace-scoped
		// endpoint must be denied even though authentication succeeded.
		require.Equal(t, 0, countMemberships(uid), "precondition: invited user has no membership")
		wsReq, _ := http.NewRequest(http.MethodGet, base+"/api/workspaces.get?id=test-workspace-id", nil)
		wsReq.Header.Set("Authorization", "Bearer "+token)
		wsResp, err := http.DefaultClient.Do(wsReq)
		require.NoError(t, err)
		defer wsResp.Body.Close()
		assert.GreaterOrEqual(t, wsResp.StatusCode, 400,
			"a member-less SSO user must NOT access a workspace (login != authz)")
		assert.NotEqual(t, http.StatusOK, wsResp.StatusCode)
	})

	t.Run("one-time code is single-use (replay rejected)", func(t *testing.T) {
		// Distinct unlinked seeded email so this fresh login does not collide with the
		// (issuer, sub) link created by the happy-path subtest.
		out := driveLogin(t, base, idp, "sub-replay", "blog-reader@example.com", true, false)
		require.NotEmpty(t, out.code, "login should succeed; got error %q", out.oidcError)

		token1, status1 := exchangeCode(t, base, out.code)
		require.Equal(t, http.StatusOK, status1)
		require.NotEmpty(t, token1)

		// Second exchange of the SAME code must fail (single-use GetAndDelete).
		_, status2 := exchangeCode(t, base, out.code)
		assert.Equal(t, http.StatusUnauthorized, status2, "replayed one-time code must be rejected")
	})

	t.Run("invited-only: unknown email is rejected and provisions nothing", func(t *testing.T) {
		const ghost = "ghost-never-invited@example.com"
		require.Equal(t, 0, countUsersByEmail(ghost), "precondition: ghost is not a user")

		out := driveLogin(t, base, idp, "sub-ghost", ghost, true, false)
		assert.Equal(t, "not_provisioned", out.oidcError, "unknown email must be rejected")
		assert.Empty(t, out.code, "no one-time code may be issued on rejection")

		// Critical: the rejection must NOT have created a user or a session/identity.
		assert.Equal(t, 0, countUsersByEmail(ghost), "rejected login must not create a user")
		assert.Equal(t, 0, countFedIdentities("sub-ghost"), "rejected login must not create a federated identity")
	})

	t.Run("nonce tampering is rejected (replay/injection defense), no session minted", func(t *testing.T) {
		out := driveLogin(t, base, idp, "sub-nonce", invitedEmail, true, true /* tamperNonce */)
		assert.Empty(t, out.code, "a token with a mismatched nonce must not yield a code")
		assert.NotEmpty(t, out.oidcError, "nonce mismatch must surface an error")
		assert.Equal(t, 0, countFedIdentities("sub-nonce"), "no identity may be linked on a nonce mismatch")
	})

	t.Run("callback without a flow cookie is rejected (login-CSRF guard)", func(t *testing.T) {
		client := noRedirectClient()
		// No /start, so no flow cookie in this browser: an attacker-crafted callback link.
		resp, err := client.Get(base + "/api/user.oidc.callback?code=x&state=anything")
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusFound, resp.StatusCode)
		loc, _ := url.Parse(resp.Header.Get("Location"))
		assert.Equal(t, "auth_failed", loc.Query().Get("oidc_error"))
		assert.NotContains(t, resp.Header.Get("Location"), "oidc_code=", "no code may be issued without a flow cookie")
	})

	t.Run("durable identity key: email change keeps the same account; recycling is refused", func(t *testing.T) {
		const recycledEmail = "non-member@example.com"

		// (a) First login binds (issuer, sub-stable) -> non-member's account.
		out1 := driveLogin(t, base, idp, "sub-stable", recycledEmail, true, false)
		require.NotEmpty(t, out1.code, "first login should succeed; got error %q", out1.oidcError)
		tok1, _ := exchangeCode(t, base, out1.code)
		uid1, _, st1 := meIdentity(t, base, tok1)
		require.Equal(t, http.StatusOK, st1)
		require.NotEmpty(t, uid1)

		// (b) Same sub, DIFFERENT email (IdP-side email change) -> SAME account,
		// matched by (issuer, sub), NOT by the new email.
		out2 := driveLogin(t, base, idp, "sub-stable", "renamed-mailbox@example.com", true, false)
		require.NotEmpty(t, out2.code, "email change under the same sub must still log in")
		tok2, _ := exchangeCode(t, base, out2.code)
		uid2, _, st2 := meIdentity(t, base, tok2)
		require.Equal(t, http.StatusOK, st2)
		assert.Equal(t, uid1, uid2, "same (issuer,sub) must map to the same user despite an email change")
		assert.Equal(t, 0, countUsersByEmail("renamed-mailbox@example.com"),
			"a changed IdP email must NOT spawn a second account")

		// (c) Email recycling / account-takeover attempt: a DIFFERENT sub presenting
		// the SAME email as the already-linked account must be REFUSED, not silently
		// re-linked, and must not take over the victim's account.
		out3 := driveLogin(t, base, idp, "sub-attacker", recycledEmail, true, false)
		assert.Equal(t, "link_conflict", out3.oidcError,
			"a new sub reusing a linked email must be refused (takeover guard)")
		assert.Empty(t, out3.code)
		assert.Equal(t, 0, countFedIdentities("sub-attacker"),
			"the attacker sub must NOT be linked to the victim account")
	})

	t.Run("config.js never leaks OIDC secrets to the browser", func(t *testing.T) {
		resp, err := http.Get(base + "/config.js")
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		js := string(body)
		assert.Contains(t, js, "window.OIDC_ENABLED = true;")
		assert.NotContains(t, js, "integration-secret", "client secret must never reach the browser")
		assert.NotContains(t, js, oidcTestClientID, "client id must never reach the browser")
		assert.NotContains(t, js, issuer, "issuer URL must never reach the browser")
	})
}

// TestOIDCIntegrationJIT exercises JIT provisioning against real Postgres: a real
// CreateUser INSERT + federated_identities row, the domain allowlist, and the
// ROOT_EMAIL bridge guard (fix) — none of which the JIT-off suite covers.
func TestOIDCIntegrationJIT(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	idp := newFakeOIDP(t)

	jitFactory := func(cfg *config.Config) testutil.AppInterface {
		cfg.OIDC = config.OIDCConfig{
			Enabled:         true,
			IssuerURL:       idp.server.URL,
			ClientID:        oidcTestClientID,
			ClientSecret:    "integration-secret",
			RedirectURI:     "http://localhost/api/user.oidc.callback",
			Scopes:          []string{"openid", "email", "profile"},
			AutoCreateUsers: true,
			AllowedDomains:  []string{"example.com"}, // matches the seed/root domain
		}
		return app.NewApp(cfg)
	}

	suite := testutil.NewIntegrationTestSuite(t, jitFactory)
	defer func() { suite.Cleanup() }()

	base := suite.ServerManager.GetURL()
	db := suite.DBManager.GetDB()
	issuer := idp.server.URL

	countUsers := func(email string) int {
		var n int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM users WHERE lower(email)=lower($1)`, email).Scan(&n))
		return n
	}
	countFed := func(sub string) int {
		var n int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM federated_identities WHERE idp_issuer=$1 AND idp_sub=$2`, issuer, sub).Scan(&n))
		return n
	}

	t.Run("allowed-domain unknown email is JIT-provisioned and persisted", func(t *testing.T) {
		const newEmail = "jit-newcomer@example.com"
		require.Equal(t, 0, countUsers(newEmail), "precondition: not yet a user")

		out := driveLogin(t, base, idp, "jit-sub-1", newEmail, true, false)
		require.Empty(t, out.oidcError, "JIT login should succeed; got %q", out.oidcError)
		require.NotEmpty(t, out.code)

		token, status := exchangeCode(t, base, out.code)
		require.Equal(t, http.StatusOK, status)
		_, gotEmail, meStatus := meIdentity(t, base, token)
		require.Equal(t, http.StatusOK, meStatus)
		assert.Equal(t, newEmail, gotEmail)

		// Real rows landed in Postgres (not just a mock).
		assert.Equal(t, 1, countUsers(newEmail), "JIT must create exactly one user")
		assert.Equal(t, 1, countFed("jit-sub-1"), "JIT must persist the federated identity")
	})

	t.Run("out-of-allowlist domain is rejected and provisions nothing", func(t *testing.T) {
		const outsider = "intruder@evil.com"
		out := driveLogin(t, base, idp, "jit-sub-evil", outsider, true, false)
		assert.NotEmpty(t, out.oidcError, "a non-allowlisted domain must be rejected")
		assert.Empty(t, out.code)
		assert.Equal(t, 0, countUsers(outsider), "rejected JIT must not create a user")
		assert.Equal(t, 0, countFed("jit-sub-evil"), "rejected JIT must not create an identity")
	})

	t.Run("ROOT_EMAIL is never JIT/bridge-linked even with JIT on (escalation guard)", func(t *testing.T) {
		// The integration config seeds ROOT_EMAIL=test@example.com as a real user, so
		// this hits the invited-bridge path; the guard must still refuse.
		const rootEmail = "test@example.com"
		out := driveLogin(t, base, idp, "jit-sub-root", rootEmail, true, false)
		assert.Equal(t, "not_provisioned", out.oidcError,
			"linking an external identity to ROOT_EMAIL must be refused (privilege escalation)")
		assert.Empty(t, out.code)
		assert.Equal(t, 0, countFed("jit-sub-root"), "the root account must not be linked to the attacker sub")
	})
}
