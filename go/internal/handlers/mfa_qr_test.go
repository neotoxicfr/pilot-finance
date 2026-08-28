package handlers

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"strconv"
	"strings"
	"testing"

	"pilot-finance/internal/auth"
)

// qrTestURI produit une URI otpauth:// réaliste (même générateur que le
// handler) pour les tests de rendu.
func qrTestURI() string {
	return auth.GenerateTOTPURI("JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP", "qr@example.com")
}

// qrIsDark indique si le pixel (x,y) est un module noir.
func qrIsDark(img image.Image, x, y int) bool {
	r, _, _, _ := img.At(x, y).RGBA()
	return r < 0x8000
}

// assertScannableQR vérifie que le PNG est un symbole QR valide et scannable :
// dimensions 200×200, matrice carrée centrée, quiet zone blanche d'au moins
// 4 modules (exigence de la norme ISO/IEC 18004), les trois motifs de repérage
// 7×7 et les deux motifs de synchronisation. Retourne le nombre de modules et
// le pas de module en pixels.
func assertScannableQR(t *testing.T, raw []byte) (modules, module int) {
	t.Helper()

	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("le PNG ne décode pas: %v", err)
	}
	if got := img.Bounds(); got.Dx() != qrSize || got.Dy() != qrSize {
		t.Fatalf("dimensions = %v, attendu %dx%d", got, qrSize, qrSize)
	}

	// Boîte englobante des pixels noirs = étendue réelle de la matrice.
	minX, minY, maxX, maxY := qrSize, qrSize, -1, -1
	for y := 0; y < qrSize; y++ {
		for x := 0; x < qrSize; x++ {
			if !qrIsDark(img, x, y) {
				continue
			}
			minX, minY = min(minX, x), min(minY, y)
			maxX, maxY = max(maxX, x), max(maxY, y)
		}
	}
	if maxX < 0 {
		t.Fatal("image entièrement blanche : aucun module dessiné")
	}
	w, h := maxX-minX+1, maxY-minY+1
	if w != h {
		t.Fatalf("matrice non carrée: %dx%d", w, h)
	}

	// Pas de module : le motif de repérage haut-gauche fait 7 modules de large.
	run := 0
	for x := minX; x <= maxX && qrIsDark(img, x, minY); x++ {
		run++
	}
	if run%7 != 0 {
		t.Fatalf("motif de repérage haut-gauche incohérent (run=%d px)", run)
	}
	module = run / 7
	if module < 1 || w%module != 0 {
		t.Fatalf("pas de module incohérent: %d px (matrice %d px)", module, w)
	}
	modules = w / module
	// Un QR valide est carré, de taille 21+4k modules (versions 1 à 40).
	if modules < 21 || modules > 177 || modules%4 != 1 {
		t.Fatalf("nombre de modules incohérent: %d", modules)
	}

	// Quiet zone : au moins 4 modules blancs sur les quatre bords.
	quiet := min(min(minX, minY), min(qrSize-1-maxX, qrSize-1-maxY))
	if quiet < 4*module {
		t.Fatalf("quiet zone insuffisante: %d px = %.2f modules (min 4)",
			quiet, float64(quiet)/float64(module))
	}
	t.Logf("QR %d×%d modules, %d px/module, quiet zone %.1f modules, PNG %d octets",
		modules, modules, module, float64(quiet)/float64(module), len(raw))

	// Centre du module (m) en pixels, sur l'axe X comme sur l'axe Y.
	px := func(m int) int { return minX + m*module + module/2 }
	py := func(m int) int { return minY + m*module + module/2 }

	// Les trois motifs de repérage : anneau externe noir, anneau blanc, cœur 3×3.
	for _, c := range [][2]int{{0, 0}, {modules - 7, 0}, {0, modules - 7}} {
		for dy := 0; dy < 7; dy++ {
			for dx := 0; dx < 7; dx++ {
				// Distance de Tchebychev au centre du motif 7×7 :
				// anneau 3 noir, anneau 2 blanc, cœur 3×3 (anneaux 0-1) noir.
				ring := max(max(dx-3, 3-dx), max(dy-3, 3-dy))
				want := ring != 2
				if got := qrIsDark(img, px(c[0]+dx), py(c[1]+dy)); got != want {
					t.Fatalf("motif de repérage (%d,%d), module (%d,%d): noir=%v, attendu %v",
						c[0], c[1], dx, dy, got, want)
				}
			}
		}
	}

	// Motifs de synchronisation : ligne et colonne 6, modules alternés.
	for m := 8; m < modules-8; m++ {
		want := m%2 == 0
		if got := qrIsDark(img, px(m), py(6)); got != want {
			t.Fatalf("synchro horizontale, module %d: noir=%v, attendu %v", m, got, want)
		}
		if got := qrIsDark(img, px(6), py(m)); got != want {
			t.Fatalf("synchro verticale, module %d: noir=%v, attendu %v", m, got, want)
		}
	}
	return modules, module
}

// TestQREncodePNG_Geometry — le QR d'enrôlement nominal est structurellement
// valide et respecte la quiet zone.
func TestQREncodePNG_Geometry(t *testing.T) {
	raw, err := qrEncodePNG(qrTestURI(), qrSize)
	if err != nil {
		t.Fatalf("qrEncodePNG: %v", err)
	}
	assertScannableQR(t, raw)
}

// TestQREncodePNG_Versions — la géométrie reste valide quelle que soit la
// longueur de l'adresse e-mail, donc quelle que soit la version du QR choisie.
// Garde-fou contre une quiet zone qui s'effondrerait sur certaines versions,
// ou un pas de module trop petit pour être scanné.
func TestQREncodePNG_Versions(t *testing.T) {
	for _, n := range []int{1, 5, 10, 20, 30, 40, 50, 60, 70} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			email := strings.Repeat("a", n) + "@example.com"
			raw, err := qrEncodePNG(
				auth.GenerateTOTPURI("JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP", email), qrSize)
			if err != nil {
				t.Fatalf("qrEncodePNG: %v", err)
			}
			modules, module := assertScannableQR(t, raw)
			// En deçà de 2 px par module le QR devient difficile à scanner une
			// fois affiché (l'<img> du template fait 192 px).
			if module < 2 {
				t.Errorf("pas de module trop petit: %d px pour %d modules", module, modules)
			}
		})
	}
}

// TestQREncodePNG_Deterministic — deux encodages de la même URI donnent le même
// PNG (aucune source d'aléa dans le rendu).
func TestQREncodePNG_Deterministic(t *testing.T) {
	uri := qrTestURI()
	a, err := qrEncodePNG(uri, qrSize)
	if err != nil {
		t.Fatalf("qrEncodePNG: %v", err)
	}
	b, err := qrEncodePNG(uri, qrSize)
	if err != nil {
		t.Fatalf("qrEncodePNG: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Error("deux encodages de la même URI diffèrent")
	}
}

// TestQREncodePNG_InvalidURI — otp.NewKeyFromURL échoue → erreur remontée.
func TestQREncodePNG_InvalidURI(t *testing.T) {
	// Caractère de contrôle : url.Parse refuse.
	if _, err := qrEncodePNG("otpauth://totp/\x7f", qrSize); err == nil {
		t.Error("attendu une erreur pour une URI invalide")
	}
}

// TestQREncodePNG_SizeTooSmall — la matrice ne tient pas dans la taille
// demandée : key.Image (barcode.Scale) échoue → erreur remontée.
func TestQREncodePNG_SizeTooSmall(t *testing.T) {
	if _, err := qrEncodePNG(qrTestURI(), 2*qrQuietZone+1); err == nil {
		t.Error("attendu une erreur pour une taille trop petite")
	}
}

// TestQREncodePNG_PNGEncodeError — branche d'erreur de l'encodage PNG.
func TestQREncodePNG_PNGEncodeError(t *testing.T) {
	orig := hookPNGEncode
	hookPNGEncode = func(w io.Writer, m image.Image) error { return errTest }
	t.Cleanup(func() { hookPNGEncode = orig })

	if _, err := qrEncodePNG(qrTestURI(), qrSize); err == nil {
		t.Error("attendu une erreur d'encodage PNG")
	}
}
