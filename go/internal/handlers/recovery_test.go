package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/db"
)

// --- Codes de récupération 2FA (audit S-22) ---
//
// Ces tests couvrent le filet de sécurité qui permet de se reconnecter après la
// perte du téléphone. Deux propriétés comptent plus que le reste : un code ne
// sert QU'UNE FOIS, et il disparaît quand le 2FA est désactivé — sinon le filet
// devient un identifiant de secours valide sur un compte sans second facteur.

func enableMFAForTest(t *testing.T, uid int64) {
	t.Helper()
	if err := db.EnableMFA(uid, "SECRETSECRETSECR"); err != nil {
		t.Fatalf("EnableMFA: %v", err)
	}
}

func TestGenerateAndStoreRecoveryCodes(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "rec-gen@example.com", "ValidP@ss1!", "USER")

	codes, err := generateAndStoreRecoveryCodes(uid)
	if err != nil {
		t.Fatalf("generateAndStoreRecoveryCodes: %v", err)
	}
	if len(codes) != auth.RecoveryCodeCount {
		t.Fatalf("want %d codes, got %d", auth.RecoveryCodeCount, len(codes))
	}
	n, err := db.CountUnusedRecoveryCodes(uid)
	if err != nil || n != auth.RecoveryCodeCount {
		t.Errorf("codes stockés : want %d, got %d (err=%v)", auth.RecoveryCodeCount, n, err)
	}
	// Le stockage ne doit JAMAIS contenir le code en clair.
	for _, c := range codes {
		var found int
		norm := auth.NormalizeRecoveryCode(c)
		if err := db.DB.QueryRow(
			`SELECT COUNT(*) FROM mfa_recovery_codes WHERE code_hash = ?`, norm).Scan(&found); err != nil {
			t.Fatalf("requête: %v", err)
		}
		if found != 0 {
			t.Fatalf("le code %q est stocké en clair", c)
		}
	}
}

func TestGenerateAndStoreRecoveryCodes_Errors(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "rec-generr@example.com", "ValidP@ss1!", "USER")
	sentinel := errors.New("boom")

	t.Run("échec de génération", func(t *testing.T) {
		orig := hookGenerateRecoveryCodes
		hookGenerateRecoveryCodes = func() ([]string, error) { return nil, sentinel }
		t.Cleanup(func() { hookGenerateRecoveryCodes = orig })
		if _, err := generateAndStoreRecoveryCodes(uid); !errors.Is(err, sentinel) {
			t.Errorf("want sentinel, got %v", err)
		}
	})
	t.Run("échec de stockage", func(t *testing.T) {
		orig := hookReplaceRecoveryCodes
		hookReplaceRecoveryCodes = func(int64, []string) error { return sentinel }
		t.Cleanup(func() { hookReplaceRecoveryCodes = orig })
		if _, err := generateAndStoreRecoveryCodes(uid); !errors.Is(err, sentinel) {
			t.Errorf("want sentinel, got %v", err)
		}
	})
}

// TestConsumeRecoveryCode_SingleUse est la propriété centrale : un code
// consommé ne doit plus jamais fonctionner.
func TestConsumeRecoveryCode_SingleUse(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "rec-once@example.com", "ValidP@ss1!", "USER")
	codes, err := generateAndStoreRecoveryCodes(uid)
	if err != nil {
		t.Fatalf("génération: %v", err)
	}

	if !consumeRecoveryCode(uid, codes[0]) {
		t.Fatal("le premier usage d'un code valide doit réussir")
	}
	if consumeRecoveryCode(uid, codes[0]) {
		t.Error("un code déjà consommé ne doit PLUS jamais être accepté")
	}
	// Les autres codes du lot restent valides.
	if !consumeRecoveryCode(uid, codes[1]) {
		t.Error("les autres codes du lot doivent rester valides")
	}
	n, _ := db.CountUnusedRecoveryCodes(uid)
	if n != auth.RecoveryCodeCount-2 {
		t.Errorf("restants : want %d, got %d", auth.RecoveryCodeCount-2, n)
	}
}

func TestConsumeRecoveryCode_Rejections(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "rec-rej@example.com", "ValidP@ss1!", "USER")
	if _, err := generateAndStoreRecoveryCodes(uid); err != nil {
		t.Fatalf("génération: %v", err)
	}

	cases := []struct{ name, input string }{
		{"code TOTP à 6 chiffres", "123456"},
		{"saisie vide", ""},
		{"code bien formé mais inconnu", "AAAA-BBBB-CCCC"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if consumeRecoveryCode(uid, tc.input) {
				t.Errorf("%q ne doit pas être accepté", tc.input)
			}
		})
	}

	t.Run("erreur base de données", func(t *testing.T) {
		orig := hookConsumeRecoveryCode
		hookConsumeRecoveryCode = func(int64, string) (bool, error) { return false, errors.New("boom") }
		t.Cleanup(func() { hookConsumeRecoveryCode = orig })
		if consumeRecoveryCode(uid, "AAAA-BBBB-CCCC") {
			t.Error("une erreur DB doit refuser, jamais accepter")
		}
	})
}

func TestMFARecoveryCount(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "rec-count@example.com", "ValidP@ss1!", "USER")

	t.Run("non authentifié", func(t *testing.T) {
		rr := httptest.NewRecorder()
		MFARecoveryCount(rr, httptest.NewRequest(http.MethodGet, "/settings/mfa/recovery/count", nil))
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("want 401, got %d", rr.Code)
		}
	})

	t.Run("aucun code restant", func(t *testing.T) {
		req := injectUser(httptest.NewRequest(http.MethodGet, "/settings/mfa/recovery/count", nil), mu(uid, "USER"))
		rr := httptest.NewRecorder()
		MFARecoveryCount(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "0") {
			t.Errorf("l'épuisement doit être signalé, got: %s", rr.Body.String())
		}
	})

	t.Run("codes restants", func(t *testing.T) {
		if _, err := generateAndStoreRecoveryCodes(uid); err != nil {
			t.Fatalf("génération: %v", err)
		}
		req := injectUser(httptest.NewRequest(http.MethodGet, "/settings/mfa/recovery/count", nil), mu(uid, "USER"))
		rr := httptest.NewRecorder()
		MFARecoveryCount(rr, req)
		if !strings.Contains(rr.Body.String(), "10") {
			t.Errorf("le compte doit apparaître, got: %s", rr.Body.String())
		}
	})

	t.Run("erreur de comptage", func(t *testing.T) {
		orig := hookCountRecoveryCodes
		hookCountRecoveryCodes = func(int64) (int, error) { return 0, errors.New("boom") }
		t.Cleanup(func() { hookCountRecoveryCodes = orig })
		req := injectUser(httptest.NewRequest(http.MethodGet, "/settings/mfa/recovery/count", nil), mu(uid, "USER"))
		rr := httptest.NewRecorder()
		MFARecoveryCount(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("want 500, got %d", rr.Code)
		}
	})
}

func regenRequest(uid int64, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/settings/mfa/recovery/regenerate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return injectUser(req, mu(uid, "USER"))
}

func TestMFARecoveryRegenerate(t *testing.T) {
	setupHandlerTest(t)
	const pwd = "ValidP@ss1!"
	uid := newUser(t, "rec-regen@example.com", pwd, "USER")

	t.Run("non authentifié", func(t *testing.T) {
		rr := httptest.NewRecorder()
		MFARecoveryRegenerate(rr, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("{}")))
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("want 401, got %d", rr.Code)
		}
	})
	t.Run("corps illisible", func(t *testing.T) {
		rr := httptest.NewRecorder()
		MFARecoveryRegenerate(rr, regenRequest(uid, "pas du json"))
		if rr.Code != http.StatusBadRequest {
			t.Errorf("want 400, got %d", rr.Code)
		}
	})
	t.Run("mot de passe absent", func(t *testing.T) {
		rr := httptest.NewRecorder()
		MFARecoveryRegenerate(rr, regenRequest(uid, `{"currentPassword":""}`))
		if rr.Code != http.StatusBadRequest {
			t.Errorf("want 400, got %d", rr.Code)
		}
	})
	t.Run("mot de passe incorrect", func(t *testing.T) {
		rr := httptest.NewRecorder()
		MFARecoveryRegenerate(rr, regenRequest(uid, `{"currentPassword":"mauvais"}`))
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("want 401, got %d", rr.Code)
		}
	})
	t.Run("2FA inactive : refus", func(t *testing.T) {
		rr := httptest.NewRecorder()
		MFARecoveryRegenerate(rr, regenRequest(uid, `{"currentPassword":"`+pwd+`"}`))
		if rr.Code != http.StatusConflict {
			t.Errorf("sans 2FA active want 409, got %d", rr.Code)
		}
	})

	enableMFAForTest(t, uid)

	t.Run("utilisateur introuvable", func(t *testing.T) {
		orig := hookGetUserByID
		hookGetUserByID = func(int64) (*db.User, error) { return nil, errors.New("boom") }
		t.Cleanup(func() { hookGetUserByID = orig })
		rr := httptest.NewRecorder()
		MFARecoveryRegenerate(rr, regenRequest(uid, `{"currentPassword":"`+pwd+`"}`))
		if rr.Code != http.StatusNotFound {
			t.Errorf("want 404, got %d", rr.Code)
		}
	})

	t.Run("échec de génération", func(t *testing.T) {
		orig := hookGenerateRecoveryCodes
		hookGenerateRecoveryCodes = func() ([]string, error) { return nil, errors.New("boom") }
		t.Cleanup(func() { hookGenerateRecoveryCodes = orig })
		rr := httptest.NewRecorder()
		MFARecoveryRegenerate(rr, regenRequest(uid, `{"currentPassword":"`+pwd+`"}`))
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("want 500, got %d", rr.Code)
		}
	})

	t.Run("succès : nouveau lot, ancien invalidé", func(t *testing.T) {
		first, err := generateAndStoreRecoveryCodes(uid)
		if err != nil {
			t.Fatalf("génération initiale: %v", err)
		}
		rr := httptest.NewRecorder()
		MFARecoveryRegenerate(rr, regenRequest(uid, `{"currentPassword":"`+pwd+`"}`))
		if rr.Code != http.StatusOK {
			t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body.String())
		}
		var resp struct {
			RecoveryCodes []string `json:"recoveryCodes"`
		}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(resp.RecoveryCodes) != auth.RecoveryCodeCount {
			t.Fatalf("want %d codes, got %d", auth.RecoveryCodeCount, len(resp.RecoveryCodes))
		}
		// La régénération doit invalider IMMÉDIATEMENT le lot précédent.
		if consumeRecoveryCode(uid, first[0]) {
			t.Error("un code de l'ancien lot doit être invalidé par la régénération")
		}
		if !consumeRecoveryCode(uid, resp.RecoveryCodes[0]) {
			t.Error("un code du nouveau lot doit fonctionner")
		}
	})
}

// TestMFADisable_PurgesRecoveryCodes : désactiver le 2FA doit supprimer les
// codes, sinon des identifiants de secours restent valides pour un compte qui
// n'a plus de second facteur, et ressusciteraient à la réactivation.
func TestMFADisable_PurgesRecoveryCodes(t *testing.T) {
	setupHandlerTest(t)
	const pwd = "ValidP@ss1!"
	uid := newUser(t, "rec-disable@example.com", pwd, "USER")
	enableMFAForTest(t, uid)
	if _, err := generateAndStoreRecoveryCodes(uid); err != nil {
		t.Fatalf("génération: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/settings/mfa/disable",
		strings.NewReader("current_password="+pwd))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	MFADisable(rr, injectUser(req, mu(uid, "USER")))

	if rr.Code != http.StatusSeeOther && rr.Code != http.StatusOK {
		t.Fatalf("désactivation échouée: %d (%s)", rr.Code, rr.Body.String())
	}
	n, err := db.CountUnusedRecoveryCodes(uid)
	if err != nil {
		t.Fatalf("comptage: %v", err)
	}
	if n != 0 {
		t.Errorf("les codes doivent être purgés à la désactivation, restants: %d", n)
	}
}

// TestMFAEnable_RecoveryFailureLeavesMFAOff vérifie l'ordre d'écriture : les
// codes sont stockés AVANT l'activation du TOTP, donc un échec doit laisser le
// compte intact. L'ordre inverse activerait le 2FA sans aucun code de secours.
func TestMFAEnable_RecoveryFailureLeavesMFAOff(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "rec-enablerr@example.com", "ValidP@ss1!", "USER")

	secret, _ := hookGenerateTOTPSecret()
	code, _ := totp.GenerateCode(secret, time.Now())

	orig := hookReplaceRecoveryCodes
	hookReplaceRecoveryCodes = func(int64, []string) error { return errTest }
	t.Cleanup(func() { hookReplaceRecoveryCodes = orig })

	mfaTok, _ := auth.GenerateMFASetupToken(uid, secret)
	body, _ := json.Marshal(map[string]string{"code": code})
	req := injectUser(httptest.NewRequest(http.MethodPost, "/api/mfa/enable", bytes.NewReader(body)), mu(uid, "USER"))
	req.AddCookie(&http.Cookie{Name: "mfa_setup", Value: mfaTok})
	rr := httptest.NewRecorder()
	MFAEnable(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rr.Code)
	}
	u, err := db.GetUserByID(uid)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if u.MFAEnabled {
		t.Error("le 2FA ne doit PAS être activé si les codes de secours n'ont pas pu être stockés")
	}
}

// TestMFADisable_PurgeErrorIsNotFatal : la désactivation reste effective même
// si la purge échoue (elle est journalisée, pas remontée) — refuser laisserait
// l'utilisateur bloqué avec un 2FA qu'il veut retirer.
func TestMFADisable_PurgeErrorIsNotFatal(t *testing.T) {
	setupHandlerTest(t)
	const pwd = "ValidP@ss1!"
	uid := newUser(t, "rec-purgerr@example.com", pwd, "USER")
	enableMFAForTest(t, uid)

	orig := hookDeleteRecoveryCodes
	hookDeleteRecoveryCodes = func(int64) error { return errTest }
	t.Cleanup(func() { hookDeleteRecoveryCodes = orig })

	req := httptest.NewRequest(http.MethodPost, "/settings/mfa/disable",
		strings.NewReader("current_password="+pwd))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	MFADisable(rr, injectUser(req, mu(uid, "USER")))

	if rr.Code >= 500 {
		t.Fatalf("une purge échouée ne doit pas faire échouer la désactivation, got %d", rr.Code)
	}
	u, _ := db.GetUserByID(uid)
	if u.MFAEnabled {
		t.Error("le 2FA doit être désactivé malgré l'échec de purge")
	}
}

// TestHandleLogin_WithRecoveryCode : un code de secours ouvre la session à la
// place du TOTP, une seule fois, et laisse une trace d'audit dédiée.
func TestHandleLogin_WithRecoveryCode(t *testing.T) {
	setupHandlerTest(t)
	uid, _ := newMFAUser(t, "rec-login@example.com")
	codes, err := generateAndStoreRecoveryCodes(uid)
	if err != nil {
		t.Fatalf("génération: %v", err)
	}

	var actions []string
	origAudit := hookLogAudit
	hookLogAudit = func(userID int64, action, ip, ua string) { actions = append(actions, action) }
	t.Cleanup(func() { hookLogAudit = origAudit })

	login := func(code string) *httptest.ResponseRecorder {
		pendingToken, err := auth.GeneratePending2FAToken(uid)
		if err != nil {
			t.Fatalf("GeneratePending2FAToken: %v", err)
		}
		req := post("/login", url.Values{"twoFactorCode": {code}})
		req.AddCookie(&http.Cookie{Name: "pending_2fa", Value: pendingToken})
		rr := httptest.NewRecorder()
		HandleLogin(rr, req)
		return rr
	}

	rr := login(codes[0])
	if rr.Code == http.StatusUnauthorized {
		t.Fatalf("un code de secours valide doit ouvrir la session, got 401 (%s)", rr.Body.String())
	}
	var gotSession bool
	for _, c := range rr.Result().Cookies() {
		if c.Name == "session" && c.Value != "" {
			gotSession = true
		}
	}
	if !gotSession {
		t.Error("aucun cookie de session posé")
	}

	var sawRecovery bool
	for _, a := range actions {
		if a == db.AuditMFARecoveryUsed {
			sawRecovery = true
		}
	}
	if !sawRecovery {
		t.Errorf("l'usage d'un code de secours doit être audité (%v)", actions)
	}

	// Rejeu du même code : refusé.
	if rr2 := login(codes[0]); rr2.Code != http.StatusUnauthorized {
		t.Errorf("le rejeu d'un code consommé doit être refusé, got %d", rr2.Code)
	}
}
