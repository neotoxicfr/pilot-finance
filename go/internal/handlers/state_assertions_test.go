package handlers

// state_assertions_test.go — findings S-14 et S-36 du giga-audit.
//
// La couverture de lignes existante exécutait ces transformations sans jamais
// observer leur RÉSULTAT : plusieurs tests se contentaient d'un « want 200 »,
// ce qui laissait passer une neutralisation complète de la logique métier.
// Les tests ci-dessous capturent les arguments réellement transmis à la couche
// DB (via les hooks de hooks.go) et assertent l'état produit.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"pilot-finance/internal/db"
)

// recurringCall mémorise les arguments transmis à la couche DB par les
// handlers d'opérations récurrentes.
type recurringCall struct {
	called      bool
	accountID   int64
	amount      int64
	day         int
	toAccountID *int64
}

// captureRecurring branche hookCreateRecurring et hookUpdateRecurring sur des
// stubs qui enregistrent leurs arguments, et restaure les originaux en fin de
// test. Le pointeur retourné est réécrit à chaque appel capté.
func captureRecurring(t *testing.T) *recurringCall {
	t.Helper()
	call := &recurringCall{}

	origCreate := hookCreateRecurring
	origUpdate := hookUpdateRecurring
	hookCreateRecurring = func(userID, accountID int64, toAccountID *int64, description string, amount int64, dayOfMonth int) error {
		*call = recurringCall{called: true, accountID: accountID, amount: amount, day: dayOfMonth, toAccountID: toAccountID}
		return nil
	}
	hookUpdateRecurring = func(id, userID, accountID int64, description string, amount int64, dayOfMonth int, toAccountID *int64) error {
		*call = recurringCall{called: true, accountID: accountID, amount: amount, day: dayOfMonth, toAccountID: toAccountID}
		return nil
	}
	t.Cleanup(func() {
		hookCreateRecurring = origCreate
		hookUpdateRecurring = origUpdate
	})
	return call
}

// ── S-14 : clamp du jour du mois ────────────────────────────────────────────

// TestRecurring_DayOfMonthClamp vérifie que le jour du mois transmis à la base
// est bien ramené à 1 hors de l'intervalle 1-31, et laissé intact à
// l'intérieur. Sans ce test, supprimer le clamp de parseRecurringForm ne
// faisait rougir aucune assertion (l'ancien test se limitait à « want 200 »).
func TestRecurring_DayOfMonthClamp(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "s14_day@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)
	call := captureRecurring(t)

	cases := []struct {
		name string
		in   string
		want int
	}{
		{"zero_clampe", "0", 1},
		{"negatif_clampe", "-5", 1},
		{"trente_deux_clampe", "32", 1},
		{"tres_grand_clampe", "999", 1},
		{"non_numerique_clampe", "abc", 1},
		{"vide_clampe", "", 1},
		{"borne_basse_intacte", "1", 1},
		{"milieu_intact", "15", 15},
		{"borne_haute_intacte", "31", 31},
	}

	for _, tc := range cases {
		t.Run("create_"+tc.name, func(t *testing.T) {
			*call = recurringCall{}
			req := injectUser(post("/recurring", url.Values{
				"description": {"Loyer"},
				"amount":      {"100"},
				"dayOfMonth":  {tc.in},
				"type":        {"expense"},
				"accountId":   {intStr(accID)},
			}), mu(uid, "USER"))
			rr := httptest.NewRecorder()
			CreateRecurring(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("want 200, got %d", rr.Code)
			}
			if !call.called {
				t.Fatal("hookCreateRecurring n'a pas été appelé")
			}
			if call.day != tc.want {
				t.Errorf("dayOfMonth=%q : want %d, got %d", tc.in, tc.want, call.day)
			}
		})

		t.Run("update_"+tc.name, func(t *testing.T) {
			*call = recurringCall{}
			req := injectUser(
				withParam(
					post("/recurring/"+intStr(recID), url.Values{
						"description": {"Loyer"},
						"amount":      {"100"},
						"dayOfMonth":  {tc.in},
						"type":        {"expense"},
					}),
					"id", intStr(recID),
				),
				mu(uid, "USER"),
			)
			rr := httptest.NewRecorder()
			UpdateRecurring(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("want 200, got %d", rr.Code)
			}
			if !call.called {
				t.Fatal("hookUpdateRecurring n'a pas été appelé")
			}
			if call.day != tc.want {
				t.Errorf("dayOfMonth=%q : want %d, got %d", tc.in, tc.want, call.day)
			}
		})
	}
}

// ── S-14 : inversion du signe selon le type d'opération ─────────────────────

// TestRecurring_AmountSignByType vérifie que le montant persisté porte le signe
// imposé par le type : une dépense est toujours négative, un revenu toujours
// positif, quel que soit le signe saisi. Inverser la règle dans
// parseRecurringForm doit faire échouer ce test.
func TestRecurring_AmountSignByType(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "s14_sign@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)
	call := captureRecurring(t)

	cases := []struct {
		name   string
		opType string
		amount string
		want   int64
	}{
		{"depense_positive_devient_negative", "expense", "500", -50000},
		{"depense_negative_reste_negative", "expense", "-500", -50000},
		{"depense_decimale_positive", "expense", "12.34", -1234},
		{"revenu_negatif_devient_positif", "income", "-500", 50000},
		{"revenu_positif_reste_positif", "income", "500", 50000},
		{"revenu_decimal_negatif", "income", "-12.34", 1234},
		{"depense_zero_reste_zero", "expense", "0", 0},
		{"revenu_zero_reste_zero", "income", "0", 0},
	}

	for _, tc := range cases {
		t.Run("create_"+tc.name, func(t *testing.T) {
			*call = recurringCall{}
			req := injectUser(post("/recurring", url.Values{
				"description": {"Op"},
				"amount":      {tc.amount},
				"dayOfMonth":  {"5"},
				"type":        {tc.opType},
				"accountId":   {intStr(accID)},
			}), mu(uid, "USER"))
			rr := httptest.NewRecorder()
			CreateRecurring(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("want 200, got %d", rr.Code)
			}
			if !call.called {
				t.Fatal("hookCreateRecurring n'a pas été appelé")
			}
			if call.amount != tc.want {
				t.Errorf("type=%s montant=%s : want %d centimes, got %d", tc.opType, tc.amount, tc.want, call.amount)
			}
		})

		t.Run("update_"+tc.name, func(t *testing.T) {
			*call = recurringCall{}
			req := injectUser(
				withParam(
					post("/recurring/"+intStr(recID), url.Values{
						"description": {"Op"},
						"amount":      {tc.amount},
						"dayOfMonth":  {"5"},
						"type":        {tc.opType},
					}),
					"id", intStr(recID),
				),
				mu(uid, "USER"),
			)
			rr := httptest.NewRecorder()
			UpdateRecurring(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("want 200, got %d", rr.Code)
			}
			if call.amount != tc.want {
				t.Errorf("type=%s montant=%s : want %d centimes, got %d", tc.opType, tc.amount, tc.want, call.amount)
			}
		})
	}
}

// TestRecurring_TransferAmountUntouched fixe le comportement complémentaire :
// un virement n'est pas concerné par l'ajustement de signe, son montant part
// tel quel vers la base (avec le compte destinataire câblé).
func TestRecurring_TransferAmountUntouched(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "s14_transfer@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	toID := createAcc(t, uid)
	call := captureRecurring(t)

	req := injectUser(post("/recurring", url.Values{
		"description": {"Virement"},
		"amount":      {"250"},
		"dayOfMonth":  {"10"},
		"type":        {"transfer"},
		"accountId":   {intStr(accID)},
		"toAccountId": {intStr(toID)},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	if call.amount != 25000 {
		t.Errorf("montant du virement : want 25000, got %d", call.amount)
	}
	if call.toAccountID == nil || *call.toAccountID != toID {
		t.Errorf("toAccountID : want %d, got %v", toID, call.toAccountID)
	}
}

// ── S-04 / FIN-3 : câblage du compte source lors d'une mise à jour ──────────

// TestCreateRecurring_UpdateWiresAccountID vérifie que le chemin POST
// /recurring en mode édition transmet bien le compte source choisi à
// db.UpdateRecurring. Remplacer cet argument par 0 (ou par une constante)
// doit faire rougir ce test : c'est exactement le bug corrigé par FIN-3.
func TestCreateRecurring_UpdateWiresAccountID(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "fin3_wire@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	accID2 := createAcc(t, uid)
	recID := createRec(t, uid, accID)
	call := captureRecurring(t)

	// L'utilisateur déplace la récurrente du compte accID vers accID2.
	req := injectUser(post("/recurring", url.Values{
		"id":          {intStr(recID)},
		"description": {"Loyer"},
		"amount":      {"500"},
		"dayOfMonth":  {"3"},
		"type":        {"expense"},
		"accountId":   {intStr(accID2)},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	if !call.called {
		t.Fatal("hookUpdateRecurring n'a pas été appelé")
	}
	if call.accountID != accID2 {
		t.Errorf("accountID transmis : want %d (compte choisi), got %d", accID2, call.accountID)
	}
}

// TestUpdateRecurring_KeepsSourceAccount fixe le contrat inverse du chemin PUT
// /recurring/{id} : il ne transporte pas de compte source, il doit donc passer
// 0 pour que db.UpdateRecurring conserve la valeur existante (COALESCE/NULLIF).
func TestUpdateRecurring_KeepsSourceAccount(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "fin3_keep@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)
	call := captureRecurring(t)

	req := injectUser(
		withParam(
			post("/recurring/"+intStr(recID), url.Values{
				"description": {"Loyer"},
				"amount":      {"500"},
				"dayOfMonth":  {"3"},
				"type":        {"expense"},
				// accountId volontairement absent : le PUT ne le transporte pas.
			}),
			"id", intStr(recID),
		),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateRecurring(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	if !call.called {
		t.Fatal("hookUpdateRecurring n'a pas été appelé")
	}
	if call.accountID != 0 {
		t.Errorf("accountID transmis : want 0 (compte inchangé), got %d", call.accountID)
	}
}

// ── S-36 : aller-retour du jeton de vérification d'email ────────────────────

// TestSendVerificationToken_RoundTrip vérifie que le jeton ENVOYÉ par mail est
// bien l'antécédent du condensat STOCKÉ en base : le lien reçu par
// l'utilisateur doit réellement valider son adresse. Les tests existants
// capturaient host et lang mais ignoraient le jeton (`func(_, _, host, lang)`),
// si bien qu'envoyer le condensat au lieu du secret ne cassait rien.
func TestSendVerificationToken_RoundTrip(t *testing.T) {
	setupHandlerTest(t)
	const email = "s36_roundtrip@example.com"
	uid := newUser(t, email, "ValidP@ss1!", "USER")

	var emailed string
	origSend := hookSendVerification
	hookSendVerification = func(_, token, _, _ string) error {
		emailed = token
		return nil
	}
	t.Cleanup(func() { hookSendVerification = origSend })

	if err := sendVerificationToken(uid, email, "fr"); err != nil {
		t.Fatalf("sendVerificationToken: %v", err)
	}
	if emailed == "" {
		t.Fatal("aucun jeton transmis à l'envoi de mail")
	}

	// Le condensat du jeton reçu doit retrouver l'utilisateur en base.
	found, err := db.GetUserByVerificationToken(hookHashToken(emailed))
	if err != nil {
		t.Fatalf("GetUserByVerificationToken: %v", err)
	}
	if found == nil {
		t.Fatal("le jeton envoyé par mail ne correspond à aucun utilisateur en base")
	}
	if found.ID != uid {
		t.Errorf("utilisateur retrouvé : want %d, got %d", uid, found.ID)
	}

	// Aller-retour complet : suivre le lien du mail doit vérifier l'adresse.
	req := httptest.NewRequest(http.MethodGet, "/verify-email?token="+emailed, nil)
	rr := httptest.NewRecorder()
	VerifyEmailPage(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("VerifyEmailPage: want 200, got %d", rr.Code)
	}

	after, err := db.GetUserByID(uid)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if !after.EmailVerified {
		t.Error("l'email devrait être vérifié après avoir suivi le lien reçu")
	}
}
