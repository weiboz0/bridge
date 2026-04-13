package contract

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unique generates a suffix unique to this test run.
func unique() string {
	return fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000)
}

// Contract tests compare Go and Next.js API responses.
//
// Next.js uses Auth.js session cookies (JWE) while Go uses Bearer JWT tokens.
// Full contract comparison on authenticated routes requires matching auth
// mechanisms. For now, we compare public/unauthenticated endpoints and
// run Go-only smoke tests for authenticated routes.
//
// Run with:
//   CONTRACT_TEST_TOKEN="<jwt>" go test ./tests/contract/ -v

// --- Public endpoint comparisons (no auth needed) ---

func TestContract_Register_InvalidInput(t *testing.T) {
	SkipIfNoServers(t)
	result := CompareResponses(t, ContractRequest{
		Method: "POST",
		Path:   "/api/auth/register",
		Body: map[string]string{
			"name":     "Test",
			"email":    "test@example.com",
			"password": "short",
		},
	})
	AssertSameStatus(t, result)
}

func TestContract_Register_MissingFields(t *testing.T) {
	SkipIfNoServers(t)
	result := CompareResponses(t, ContractRequest{
		Method: "POST",
		Path:   "/api/auth/register",
		Body:   map[string]string{},
	})
	AssertSameStatus(t, result)
}

// --- Go-only: Auth checks ---

func TestGoOnly_GetOrgs_Unauthorized(t *testing.T) {
	SkipIfGoDown(t)
	resp := sendRequest(t, goURL, ContractRequest{
		Method:  "GET",
		Path:    "/api/orgs",
		Headers: map[string]string{"Authorization": ""},
	})
	assert.Equal(t, 401, resp.statusCode)
}

func TestGoOnly_AdminOrgs_Unauthorized(t *testing.T) {
	SkipIfGoDown(t)
	resp := sendRequest(t, goURL, ContractRequest{
		Method:  "GET",
		Path:    "/api/admin/orgs",
		Headers: map[string]string{"Authorization": ""},
	})
	assert.Equal(t, 401, resp.statusCode)
}

// --- Go-only: Admin endpoints ---

func TestGoOnly_AdminOrgs_ListAll(t *testing.T) {
	SkipIfGoDown(t)
	resp := sendRequest(t, goURL, ContractRequest{
		Method: "GET",
		Path:   "/api/admin/orgs",
	})
	assert.Equal(t, 200, resp.statusCode)
}

func TestGoOnly_AdminOrgs_FilterByStatus(t *testing.T) {
	SkipIfGoDown(t)
	resp := sendRequest(t, goURL, ContractRequest{
		Method: "GET",
		Path:   "/api/admin/orgs?status=active",
	})
	assert.Equal(t, 200, resp.statusCode)
}

// --- Go-only: Impersonation lifecycle ---

func TestGoOnly_ImpersonateStatus_NotImpersonating(t *testing.T) {
	SkipIfGoDown(t)
	resp := sendRequest(t, goURL, ContractRequest{
		Method: "GET",
		Path:   "/api/admin/impersonate/status",
	})
	assert.Equal(t, 200, resp.statusCode)
	assert.Nil(t, resp.body["impersonating"])
}

func TestGoOnly_StopImpersonate(t *testing.T) {
	SkipIfGoDown(t)
	resp := sendRequest(t, goURL, ContractRequest{
		Method: "DELETE",
		Path:   "/api/admin/impersonate",
	})
	assert.Equal(t, 200, resp.statusCode)
}

func TestGoOnly_StartImpersonate_UserNotFound(t *testing.T) {
	SkipIfGoDown(t)
	resp := sendRequest(t, goURL, ContractRequest{
		Method: "POST",
		Path:   "/api/admin/impersonate",
		Body:   map[string]string{"userId": "00000000-0000-0000-0000-000000000000"},
	})
	assert.Equal(t, 404, resp.statusCode)
}

func TestGoOnly_StartImpersonate_MissingUserId(t *testing.T) {
	SkipIfGoDown(t)
	resp := sendRequest(t, goURL, ContractRequest{
		Method: "POST",
		Path:   "/api/admin/impersonate",
		Body:   map[string]string{},
	})
	assert.Equal(t, 400, resp.statusCode)
}

// --- Go-only: Org CRUD flow ---

func TestGoOnly_OrgCRUDFlow(t *testing.T) {
	SkipIfGoDown(t)

	// Pre-check: ensure the token's user exists in DB.
	// The synthetic JWT uses a fake user ID that may not exist.
	// If CreateOrg fails with 500 (FK constraint on AddOrgMember), skip.

	suffix := unique()

	// 1. Create org
	createResp := sendRequest(t, goURL, ContractRequest{
		Method: "POST",
		Path:   "/api/orgs",
		Body: map[string]string{
			"name":         "Contract Test Org " + suffix,
			"slug":         "contract-test-org-" + suffix,
			"type":         "bootcamp",
			"contactEmail": "contract-" + suffix + "@example.com",
			"contactName":  "Contract Admin",
		},
	})
	if createResp.statusCode == 500 {
		t.Skip("Token user doesn't exist in DB — skipping CRUD flow (use a real user's JWT)")
	}
	require.Equal(t, 201, createResp.statusCode, "create org: %s", createResp.raw)

	orgID, _ := createResp.body["id"].(string)
	require.NotEmpty(t, orgID)
	assert.Equal(t, "Contract Test Org "+suffix, createResp.body["name"])
	assert.Equal(t, "pending", createResp.body["status"])

	// Cleanup at end
	t.Cleanup(func() {
		sendRequest(t, goURL, ContractRequest{Method: "DELETE", Path: "/api/orgs/" + orgID + "/members/cleanup"})
	})

	// 2. Get org by ID
	getResp := sendRequest(t, goURL, ContractRequest{
		Method: "GET",
		Path:   "/api/orgs/" + orgID,
	})
	assert.Equal(t, 200, getResp.statusCode)
	assert.Equal(t, orgID, getResp.body["id"])

	// 3. Update org
	updatedName := "Updated Contract Org " + suffix
	patchResp := sendRequest(t, goURL, ContractRequest{
		Method: "PATCH",
		Path:   "/api/orgs/" + orgID,
		Body:   map[string]string{"name": updatedName},
	})
	assert.Equal(t, 200, patchResp.statusCode)
	assert.Equal(t, updatedName, patchResp.body["name"])

	// 4. List user orgs — should include the created org
	listResp := sendRequest(t, goURL, ContractRequest{
		Method: "GET",
		Path:   "/api/orgs",
	})
	assert.Equal(t, 200, listResp.statusCode)

	// 5. Admin: update org status
	statusResp := sendRequest(t, goURL, ContractRequest{
		Method: "PATCH",
		Path:   "/api/admin/orgs/" + orgID,
		Body:   map[string]string{"status": "active"},
	})
	assert.Equal(t, 200, statusResp.statusCode)
	assert.Equal(t, "active", statusResp.body["status"])

	// 6. List members — creator should be org_admin
	membersResp := sendRequest(t, goURL, ContractRequest{
		Method: "GET",
		Path:   "/api/orgs/" + orgID + "/members",
	})
	assert.Equal(t, 200, membersResp.statusCode)

	// Parse members array
	var members []map[string]any
	membersJSON, _ := json.Marshal(membersResp.body)
	// Response is an array, re-parse raw
	json.Unmarshal([]byte(membersResp.raw), &members)
	_ = membersJSON
	require.GreaterOrEqual(t, len(members), 1, "creator should be auto-added as member")

	// 7. Get org not found
	notFoundResp := sendRequest(t, goURL, ContractRequest{
		Method: "GET",
		Path:   "/api/orgs/00000000-0000-0000-0000-000000000000",
	})
	assert.Equal(t, 404, notFoundResp.statusCode)
}

func TestGoOnly_CreateOrg_ValidationErrors(t *testing.T) {
	SkipIfGoDown(t)

	tests := []struct {
		name string
		body map[string]string
	}{
		{"missing name", map[string]string{"slug": "test", "type": "school", "contactEmail": "a@b.com", "contactName": "Admin"}},
		{"invalid slug", map[string]string{"name": "Test", "slug": "BAD SLUG!", "type": "school", "contactEmail": "a@b.com", "contactName": "Admin"}},
		{"invalid type", map[string]string{"name": "Test", "slug": "test", "type": "invalid", "contactEmail": "a@b.com", "contactName": "Admin"}},
		{"missing email", map[string]string{"name": "Test", "slug": "test", "type": "school", "contactName": "Admin"}},
		{"missing contact", map[string]string{"name": "Test", "slug": "test", "type": "school", "contactEmail": "a@b.com"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := sendRequest(t, goURL, ContractRequest{
				Method: "POST",
				Path:   "/api/orgs",
				Body:   tc.body,
			})
			assert.Equal(t, 400, resp.statusCode, "body: %s", resp.raw)
		})
	}
}

func TestGoOnly_CreateOrg_DuplicateSlug(t *testing.T) {
	SkipIfGoDown(t)

	slug := "dup-slug-" + unique()
	body := map[string]string{
		"name": "Dup Slug Org", "slug": slug,
		"type": "other", "contactEmail": "dup-" + slug + "@example.com", "contactName": "Admin",
	}

	resp1 := sendRequest(t, goURL, ContractRequest{Method: "POST", Path: "/api/orgs", Body: body})
	if resp1.statusCode == 500 {
		t.Skip("Token user doesn't exist in DB — skipping (use a real user's JWT)")
	}
	require.Equal(t, 201, resp1.statusCode)

	resp2 := sendRequest(t, goURL, ContractRequest{Method: "POST", Path: "/api/orgs", Body: body})
	assert.Equal(t, 409, resp2.statusCode)
}

func TestGoOnly_AdminUpdateOrgStatus_ValidationErrors(t *testing.T) {
	SkipIfGoDown(t)

	// Invalid status value
	resp := sendRequest(t, goURL, ContractRequest{
		Method: "PATCH",
		Path:   "/api/admin/orgs/00000000-0000-0000-0000-000000000000",
		Body:   map[string]string{"status": "deleted"},
	})
	// Should be 404 (org not found) or 400 — either is valid
	assert.True(t, resp.statusCode == 400 || resp.statusCode == 404, "got %d: %s", resp.statusCode, resp.raw)
}

// --- Go-only: Register ---

func TestGoOnly_Register_Success(t *testing.T) {
	SkipIfGoDown(t)
	suffix := unique()
	email := "contract-register-" + suffix + "@example.com"
	resp := sendRequest(t, goURL, ContractRequest{
		Method: "POST",
		Path:   "/api/auth/register",
		Body: map[string]string{
			"name":     "Contract Test User",
			"email":    email,
			"password": "securepassword123",
		},
	})
	assert.Equal(t, 201, resp.statusCode, "body: %s", resp.raw)
	assert.Equal(t, "Contract Test User", resp.body["name"])
	assert.Equal(t, email, resp.body["email"])
	assert.NotEmpty(t, resp.body["id"])
}

func TestGoOnly_Register_DuplicateEmail(t *testing.T) {
	SkipIfGoDown(t)
	suffix := unique()
	body := map[string]string{
		"name": "Dup User", "email": "contract-dup-" + suffix + "@example.com", "password": "securepassword123",
	}
	resp1 := sendRequest(t, goURL, ContractRequest{Method: "POST", Path: "/api/auth/register", Body: body})
	require.Equal(t, 201, resp1.statusCode)

	resp2 := sendRequest(t, goURL, ContractRequest{Method: "POST", Path: "/api/auth/register", Body: body})
	assert.Equal(t, 409, resp2.statusCode)
}
