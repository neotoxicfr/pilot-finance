package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/middleware"
)

// TestIntegration_RegisterLoginCreateAccount tests the full user flow:
// register → login → create account → view dashboard.
func TestIntegration_RegisterLoginCreateAccount(t *testing.T) {
	setupHandlerTest(t)

	const email = "integration@example.com"
	const password = "IntegrationP@ss1!"

	// 1. Register
	rr := httptest.NewRecorder()
	HandleRegister(rr, post("/register", url.Values{
		"email":           {email},
		"password":        {password},
		"confirmPassword": {password},
	}))
	if rr.Code != http.StatusSeeOther && rr.Code != http.StatusOK {
		t.Fatalf("register: got %d, want 303 or 200 (body: %s)", rr.Code, rr.Body.String())
	}

	// Extract session cookie from register response
	var sessionToken string
	for _, c := range rr.Result().Cookies() {
		if c.Name == "session" {
			sessionToken = c.Value
			break
		}
	}
	if sessionToken == "" {
		t.Fatal("register did not set session cookie")
	}

	// 2. Login (verify credentials work)
	rr = httptest.NewRecorder()
	HandleLogin(rr, post("/login", url.Values{
		"email":    {email},
		"password": {password},
	}))
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("login: got %d, want 303", rr.Code)
	}
	// 3. Create account — derive user ID from session cookie
	claims, err := auth.ValidateToken(sessionToken)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	user := &middleware.User{ID: claims.UserID, Role: claims.Role, Language: claims.Language, Currency: claims.Currency, SessionVersion: claims.SessionVersion}
	req := injectUser(post("/accounts", url.Values{
		"name":    {"Livret A"},
		"balance": {"22950"},
		"color":   {"#10b981"},
	}), user)
	rr = httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create account: got %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Livret A") {
		t.Error("create account response should contain account name")
	}

	// 4. Dashboard API (verify account appears in JSON)
	req = injectUser(httptest.NewRequest(http.MethodGet, "/api/dashboard", nil), user)
	rr = httptest.NewRecorder()
	DashboardAPI(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard API: got %d, want 200", rr.Code)
	}
	var dashData map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&dashData); err != nil {
		t.Fatalf("decode dashboard: %v", err)
	}
	if dashData["totalBalance"].(float64) != 22950 {
		t.Errorf("totalBalance: want 22950, got %v", dashData["totalBalance"])
	}
	accounts, ok := dashData["accounts"].([]interface{})
	if !ok || len(accounts) != 1 {
		t.Errorf("want 1 account, got %v", len(accounts))
	}

	// 5. Create a second account and verify projection
	req = injectUser(post("/accounts", url.Values{
		"name":             {"PEA"},
		"balance":          {"50000"},
		"color":            {"#3b82f6"},
		"isYieldActive":    {"on"},
		"yieldType":        {"RANGE"},
		"yieldMin":         {"3"},
		"yieldMax":         {"8"},
		"reinvestmentRate":  {"100"},
	}), user)
	rr = httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create account 2: got %d, want 200", rr.Code)
	}

	// 6. Dashboard API with projection
	req = injectUser(httptest.NewRequest(http.MethodGet, "/api/dashboard?years=5", nil), user)
	rr = httptest.NewRecorder()
	DashboardAPI(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard API 2: got %d, want 200", rr.Code)
	}
	if err := json.NewDecoder(rr.Body).Decode(&dashData); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if dashData["totalBalance"].(float64) != 72950 {
		t.Errorf("totalBalance: want 72950, got %v", dashData["totalBalance"])
	}
	if dashData["totalInterests"].(float64) <= 0 {
		t.Error("totalInterests should be > 0 with yield active")
	}
}
