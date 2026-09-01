package templates

import (
	"bytes"
	"errors"
	"html/template"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// goRoot returns the go/ directory (two levels up from internal/templates/).
func goRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "../..")
}

// --- formatUnitsCompact ---

func TestFormatUnitsCompact_Small(t *testing.T) {
	if got := formatUnitsCompact(0, "EUR", "fr"); got != "0 EUR" {
		t.Errorf("want '0 EUR', got %q", got)
	}
	if got := formatUnitsCompact(999, "EUR", "fr"); got != "999 EUR" {
		t.Errorf("want '999 EUR', got %q", got)
	}
}

// Séparateur décimal harmonisé sur formatFloat : virgule, plus point (audit S-26).
func TestFormatUnitsCompact_Kilo(t *testing.T) {
	if got := formatUnitsCompact(1500, "EUR", "fr"); got != "1,5k EUR" {
		t.Errorf("want '1,5k EUR', got %q", got)
	}
	if got := formatUnitsCompact(15000, "EUR", "fr"); got != "15k EUR" {
		t.Errorf("want '15k EUR', got %q", got)
	}
}

func TestFormatUnitsCompact_Mega(t *testing.T) {
	if got := formatUnitsCompact(1500000, "EUR", "fr"); got != "1,5M EUR" {
		t.Errorf("want '1,5M EUR', got %q", got)
	}
}

func TestFormatUnitsCompact_Negative(t *testing.T) {
	got := formatUnitsCompact(-1500, "EUR", "fr")
	if got != "-1,5k EUR" {
		t.Errorf("want '-1,5k EUR', got %q", got)
	}
}

func TestFormatUnitsCompact_EmptyCurrency(t *testing.T) {
	// Empty currency defaults to EUR
	got := formatUnitsCompact(100, "", "fr")
	if got != "100 EUR" {
		t.Errorf("want '100 EUR', got %q", got)
	}
}

// TestFormatUnitsCompact_JSMirror fige le contrat partagé avec le miroir JS
// compactMoney() de go/static/js/charts.js (audit S-26). Toute ligne modifiée
// ici doit l'être aussi dans charts.js : ces valeurs sont exactement celles que
// l'axe Y et le centre du camembert doivent afficher.
//
// Les valeurs évitent volontairement les demi-unités exactes (12,5 → « 12k » en
// Go, « 13k » en JS) : c'est la seule divergence résiduelle assumée entre les
// deux formateurs, Go arrondissant les égalités à l'entier pair et JS à
// l'entier supérieur. Elle est documentée dans charts.js.
func TestFormatUnitsCompact_JSMirror(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		want  string
	}{
		{"zero", 0, "0 EUR"},
		{"unites", 999, "999 EUR"},
		{"millier", 1000, "1,0k EUR"},
		{"millier decimal", 1234, "1,2k EUR"},
		{"palier 10k", 10000, "10k EUR"},
		{"dizaines de milliers", 49450, "49k EUR"},
		{"million", 1000000, "1,0M EUR"},
		{"negatif unites", -999, "-999 EUR"},
		{"negatif millier", -1234, "-1,2k EUR"},
		{"negatif palier 10k", -12400, "-12k EUR"},
		{"negatif million", -2500000, "-2,5M EUR"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatUnitsCompact(tc.value, "EUR", "fr"); got != tc.want {
				t.Errorf("formatUnitsCompact(%v): want %q, got %q", tc.value, tc.want, got)
			}
		})
	}
}

// --- sub ---

func TestSub(t *testing.T) {
	if got, err := sub(10, 3); err != nil || got != 7 {
		t.Errorf("sub(10, 3): want 7, got %v (err=%v)", got, err)
	}
	if got, err := sub(0, 5); err != nil || got != -5 {
		t.Errorf("sub(0, 5): want -5, got %v (err=%v)", got, err)
	}
}

// --- absFunc ---

func TestAbsFunc_Positive(t *testing.T) {
	if got, err := absFunc(5.0); err != nil || got != 5.0 {
		t.Errorf("absFunc(5): want 5, got %v (err=%v)", got, err)
	}
}

func TestAbsFunc_Negative(t *testing.T) {
	if got, err := absFunc(-3.14); err != nil || got != 3.14 {
		t.Errorf("absFunc(-3.14): want 3.14, got %v (err=%v)", got, err)
	}
}

func TestAbsFunc_Zero(t *testing.T) {
	if got, err := absFunc(0); err != nil || got != 0 {
		t.Errorf("absFunc(0): want 0, got %v (err=%v)", got, err)
	}
}

// --- formatBalance ---

func TestFormatBalance_Integer(t *testing.T) {
	if got := formatBalance(10000); got != "100" {
		t.Errorf("want '100', got %q", got)
	}
}

func TestFormatBalance_Decimal(t *testing.T) {
	if got := formatBalance(10050); got != "100.50" {
		t.Errorf("want '100.50', got %q", got)
	}
}

// --- formatUnits ---

func TestFormatUnits_Integer(t *testing.T) {
	if got := formatUnits(1000, "EUR", "fr"); got != "1 000 EUR" {
		t.Errorf("want '1 000 EUR', got %q", got)
	}
}

func TestFormatUnits_Decimal(t *testing.T) {
	if got := formatUnits(1000.50, "EUR", "fr"); got != "1 000,50 EUR" {
		t.Errorf("want '1 000,50 EUR', got %q", got)
	}
}

func TestFormatUnits_EmptyCurrency(t *testing.T) {
	got := formatUnits(100, "", "fr")
	if got != "100 EUR" {
		t.Errorf("want '100 EUR', got %q", got)
	}
}

// --- dict ---

func TestDict_EvenArgs(t *testing.T) {
	d := dict("key1", "val1", "key2", 42)
	if d["key1"] != "val1" {
		t.Errorf("dict[key1]: want 'val1', got %v", d["key1"])
	}
	if d["key2"] != 42 {
		t.Errorf("dict[key2]: want 42, got %v", d["key2"])
	}
}

func TestDict_OddArgs_ReturnsNil(t *testing.T) {
	d := dict("key1")
	if d != nil {
		t.Errorf("dict with odd args: want nil, got %v", d)
	}
}

func TestDict_NonStringKey_Skipped(t *testing.T) {
	d := dict(42, "val")
	if d == nil {
		t.Fatal("want empty map, got nil")
	}
	if len(d) != 0 {
		t.Errorf("non-string key should be skipped, got %v", d)
	}
}

// --- toJSON ---

func TestToJSON_Map(t *testing.T) {
	got := toJSON(map[string]int{"a": 1})
	if string(got) != `{"a":1}` {
		t.Errorf("toJSON: want '{\"a\":1}', got %q", got)
	}
}

func TestToJSON_Nil(t *testing.T) {
	// nil marshals to "null"
	got := toJSON(nil)
	if string(got) != "null" {
		t.Errorf("toJSON(nil): want 'null', got %q", got)
	}
}

// --- orFunc ---

func TestOrFunc_NilFirst(t *testing.T) {
	if got := orFunc(nil, "fallback"); got != "fallback" {
		t.Errorf("want 'fallback', got %v", got)
	}
}

func TestOrFunc_EmptyStringFirst(t *testing.T) {
	if got := orFunc("", "fallback"); got != "fallback" {
		t.Errorf("want 'fallback', got %v", got)
	}
}

func TestOrFunc_NonEmptyFirst(t *testing.T) {
	if got := orFunc("value", "fallback"); got != "value" {
		t.Errorf("want 'value', got %v", got)
	}
}

func TestOrFunc_Variadic(t *testing.T) {
	// All falsy except last
	if got := orFunc(nil, "", 0, "last"); got != "last" {
		t.Errorf("want 'last', got %v", got)
	}
	// Truthy in the middle
	if got := orFunc(nil, "mid", "end"); got != "mid" {
		t.Errorf("want 'mid', got %v", got)
	}
	// No args
	if got := orFunc(); got != nil {
		t.Errorf("want nil, got %v", got)
	}
	// All falsy → returns last
	if got := orFunc(false, 0, ""); got != "" {
		t.Errorf("want empty string (last), got %v", got)
	}
}

// TestOrFunc_Int64Zero vérifie qu'int64(0) est traité comme falsy.
func TestOrFunc_Int64Zero(t *testing.T) {
	if got := orFunc(int64(0), "fallback"); got != "fallback" {
		t.Errorf("orFunc(int64(0), fallback): want 'fallback', got %v", got)
	}
}

// TestOrFunc_Float64Zero vérifie que float64(0) est traité comme falsy.
func TestOrFunc_Float64Zero(t *testing.T) {
	if got := orFunc(float64(0), "fallback"); got != "fallback" {
		t.Errorf("orFunc(float64(0), fallback): want 'fallback', got %v", got)
	}
}

// TestOrFunc_NonZeroNumerics vérifie que les nombres non nuls sont truthy.
func TestOrFunc_NonZeroNumerics(t *testing.T) {
	if got := orFunc(int64(5), "fallback"); got != int64(5) {
		t.Errorf("orFunc(int64(5)): want int64(5), got %v", got)
	}
	if got := orFunc(float64(2.5), "fallback"); got != float64(2.5) {
		t.Errorf("orFunc(float64(2.5)): want 2.5, got %v", got)
	}
	if got := orFunc(7, "fallback"); got != 7 {
		t.Errorf("orFunc(int 7): want 7, got %v", got)
	}
}

// TestOrFunc_BoolTrue vérifie que true est truthy.
func TestOrFunc_BoolTrue(t *testing.T) {
	if got := orFunc(true, "fallback"); got != true {
		t.Errorf("orFunc(true): want true, got %v", got)
	}
}

// TestOrFunc_DefaultTypeTruthy vérifie qu'un type non géré (ex. slice) est truthy.
func TestOrFunc_DefaultTypeTruthy(t *testing.T) {
	val := []string{"x"}
	got := orFunc(val, "fallback")
	gotSlice, ok := got.([]string)
	if !ok || len(gotSlice) != 1 || gotSlice[0] != "x" {
		t.Errorf("orFunc(slice): want the slice returned, got %v", got)
	}
}

// --- Init, Render, RenderPartial ---

func TestInit_ValidDir(t *testing.T) {
	root := goRoot()
	if err := Init(filepath.Join(root, "templates")); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(pageTemplates) == 0 {
		t.Error("Init should load at least one template")
	}
}

func TestRender_UnknownTemplate(t *testing.T) {
	// Ensure Init has been called
	root := goRoot()
	_ = Init(filepath.Join(root, "templates"))

	var buf bytes.Buffer
	err := Render(&buf, "nonexistent-page-xyz.html", nil)
	if err == nil {
		t.Error("want error for unknown template")
	}
}

func TestRenderPartial_UnknownTemplate(t *testing.T) {
	root := goRoot()
	_ = Init(filepath.Join(root, "templates"))

	var buf bytes.Buffer
	err := RenderPartial(&buf, "nonexistent-xyz.html", "someblock", nil)
	if err == nil {
		t.Error("want error for unknown template in RenderPartial")
	}
}

func TestRenderPartial_UnknownBlock(t *testing.T) {
	root := goRoot()
	if err := Init(filepath.Join(root, "templates")); err != nil {
		t.Fatalf("Init: %v", err)
	}

	var buf bytes.Buffer
	// "login.html" exists but "nonexistent-block" does not
	err := RenderPartial(&buf, "login.html", "nonexistent-block-xyz", nil)
	if err == nil {
		t.Error("want error for unknown block in RenderPartial")
	}
}

// --- mult / add ---

func TestMult(t *testing.T) {
	if got, err := mult(3.0, 4.0); err != nil || got != 12.0 {
		t.Errorf("mult(3,4): want 12, got %v (err=%v)", got, err)
	}
	if got, err := mult(0, 100); err != nil || got != 0 {
		t.Errorf("mult(0,100): want 0, got %v (err=%v)", got, err)
	}
}

func TestAdd(t *testing.T) {
	if got, err := add(2.5, 3.5); err != nil || got != 6.0 {
		t.Errorf("add(2.5,3.5): want 6, got %v (err=%v)", got, err)
	}
}

// --- ge / gt ---

func TestGe(t *testing.T) {
	cases := []struct {
		a, b interface{}
		want bool
	}{
		{5.0, 5.0, true},
		{6.0, 5.0, true},
		{4.0, 5.0, false},
		// Comparaison d'un montant stocke (int64 centimes) a zero : le signe ne
		// depend pas de l'unite, mais le type ne doit plus etre reinterprete
		// (audit S-40).
		{int64(-1500), 0, false},
		{int64(1500), 0, true},
	}
	for _, tc := range cases {
		got, err := ge(tc.a, tc.b)
		if err != nil || got != tc.want {
			t.Errorf("ge(%v,%v): want %v, got %v (err=%v)", tc.a, tc.b, tc.want, got, err)
		}
	}
}

func TestGt(t *testing.T) {
	cases := []struct {
		a, b interface{}
		want bool
	}{
		{6.0, 5.0, true},
		{5.0, 5.0, false},
		{int64(1), 0, true},
		{int64(0), 0, false},
	}
	for _, tc := range cases {
		got, err := gt(tc.a, tc.b)
		if err != nil || got != tc.want {
			t.Errorf("gt(%v,%v): want %v, got %v (err=%v)", tc.a, tc.b, tc.want, got, err)
		}
	}
}

// --- eqFunc / neFunc ---

func TestEqFunc(t *testing.T) {
	if !eqFunc("a", "a") {
		t.Error("eqFunc(a,a): want true")
	}
	if eqFunc("a", "b") {
		t.Error("eqFunc(a,b): want false")
	}
}

func TestNeFunc(t *testing.T) {
	if !neFunc("a", "b") {
		t.Error("neFunc(a,b): want true")
	}
	if neFunc("x", "x") {
		t.Error("neFunc(x,x): want false")
	}
}

// --- formatWithSpaces negative ---

func TestFormatWithSpaces_Negative(t *testing.T) {
	if got := groupDigits(-1234567, " "); got != "-1 234 567" {
		t.Errorf("groupDigits(-1234567, espace): want '-1 234 567', got %q", got)
	}
}

// --- formatFloat negative decimal part ---

func TestFormatFloat_Negative(t *testing.T) {
	// negative float: intPart=-1, decPart = (-1.5 - (-1)) * 100 = -50 → abs → 50
	if got := formatDecimal(-1.5, " ", ","); got != "-1,50" {
		t.Errorf("formatDecimal(-1.5): want '-1,50', got %q", got)
	}
}

// --- toJSON unencodable value ---

func TestToJSON_UnencodableValue(t *testing.T) {
	// math.Inf(1) causes json.Encoder.Encode to fail → returns "null"
	got := toJSON(math.Inf(1))
	if string(got) != "null" {
		t.Errorf("toJSON(Inf): want 'null', got %q", got)
	}
}

// --- Render success path ---

func TestRender_Success(t *testing.T) {
	// Inject a minimal template directly into pageTemplates (same package access)
	tmpl := template.New("").Funcs(FuncMap)
	if _, err := tmpl.New("base.html").Parse(`{{template "content" .}}`); err != nil {
		t.Fatalf("parse base: %v", err)
	}
	if _, err := tmpl.New("test-render.html").Parse(`{{define "content"}}hello{{end}}`); err != nil {
		t.Fatalf("parse page: %v", err)
	}
	pageTemplates["test-render.html"] = tmpl
	defer delete(pageTemplates, "test-render.html")

	var buf bytes.Buffer
	if err := Render(&buf, "test-render.html", nil); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if buf.String() != "hello" {
		t.Errorf("want 'hello', got %q", buf.String())
	}
}

// --- Init error paths ---

func makeMinimalTemplateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{"layouts", "components", "pages"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}
	return dir
}

func TestInit_InvalidBaseTemplate(t *testing.T) {
	dir := makeMinimalTemplateDir(t)
	// Write an invalid layout template — causes tmpl.New(baseName).Parse to fail
	os.WriteFile(filepath.Join(dir, "layouts", "base.html"), []byte("{{.Broken"), 0644) //nolint:errcheck
	// At least one page so the per-page loop runs
	os.WriteFile(filepath.Join(dir, "pages", "index.html"), []byte(`{{define "content"}}ok{{end}}`), 0644) //nolint:errcheck

	if err := Init(dir); err == nil {
		t.Error("want error for invalid base template syntax")
	}
}

func TestInit_InvalidPageTemplate(t *testing.T) {
	dir := makeMinimalTemplateDir(t)
	// Valid base template
	os.WriteFile(filepath.Join(dir, "layouts", "base.html"), []byte(`{{block "content" .}}{{end}}`), 0644) //nolint:errcheck
	// Invalid page template — causes tmpl.New(pageName).Parse to fail
	os.WriteFile(filepath.Join(dir, "pages", "bad.html"), []byte("{{.Broken"), 0644) //nolint:errcheck

	if err := Init(dir); err == nil {
		t.Error("want error for invalid page template syntax")
	}
}

// --- Init glob/readfile error paths (via hooks) ---

func TestInit_GlobLayoutError(t *testing.T) {
	orig := globFn
	defer func() { globFn = orig }()
	count := 0
	globFn = func(p string) ([]string, error) {
		count++
		if count == 1 {
			return nil, errors.New("glob-layout-error")
		}
		return filepath.Glob(p)
	}
	if err := Init("/any"); err == nil || err.Error() != "glob-layout-error" {
		t.Errorf("want glob-layout-error, got %v", err)
	}
}

func TestInit_GlobComponentError(t *testing.T) {
	orig := globFn
	defer func() { globFn = orig }()
	count := 0
	globFn = func(p string) ([]string, error) {
		count++
		if count == 2 {
			return nil, errors.New("glob-component-error")
		}
		return filepath.Glob(p)
	}
	if err := Init("/any"); err == nil || err.Error() != "glob-component-error" {
		t.Errorf("want glob-component-error, got %v", err)
	}
}

func TestInit_GlobPageError(t *testing.T) {
	orig := globFn
	defer func() { globFn = orig }()
	count := 0
	globFn = func(p string) ([]string, error) {
		count++
		if count == 3 {
			return nil, errors.New("glob-page-error")
		}
		return filepath.Glob(p)
	}
	if err := Init("/any"); err == nil || err.Error() != "glob-page-error" {
		t.Errorf("want glob-page-error, got %v", err)
	}
}

func TestInit_ReadBaseFileError(t *testing.T) {
	orig := osReadFile
	defer func() { osReadFile = orig }()
	osReadFile = func(name string) ([]byte, error) {
		return nil, errors.New("read-base-error")
	}
	// Need at least one page so the per-page loop runs, and at least one base file
	dir := makeMinimalTemplateDir(t)
	os.WriteFile(filepath.Join(dir, "layouts", "base.html"), []byte(`ok`), 0644) //nolint:errcheck
	os.WriteFile(filepath.Join(dir, "pages", "index.html"), []byte(`ok`), 0644)  //nolint:errcheck

	err := Init(dir)
	if err == nil {
		t.Error("want error from osReadFile on base file")
	}
}

// --- toNumber : conversion SANS unite (audit S-40) ---
//
// toFloat64 divisait les int64 par 100 (« un int64 est un montant en
// centimes ») et retournait 0 sur tout autre type. Les deux comportements sont
// remplacés : la valeur numérique est rendue telle quelle, et un type non
// numérique est une erreur qui interrompt le rendu du template.

func TestToNumber_Int(t *testing.T) {
	got, err := toNumber(42)
	if err != nil || got != 42.0 {
		t.Errorf("toNumber(int 42): want 42.0, got %v (err=%v)", got, err)
	}
}

func TestToNumber_Int_Negative(t *testing.T) {
	got, err := toNumber(-10)
	if err != nil || got != -10.0 {
		t.Errorf("toNumber(int -10): want -10.0, got %v (err=%v)", got, err)
	}
}

func TestToNumber_Float64(t *testing.T) {
	got, err := toNumber(1234.56)
	if err != nil || got != 1234.56 {
		t.Errorf("toNumber(float64): want 1234.56, got %v (err=%v)", got, err)
	}
}

// Un int64 n'est PLUS divisé par 100 : l'unité appartient au formateur, pas au
// type dynamique.
func TestToNumber_Int64_NoUnitConversion(t *testing.T) {
	got, err := toNumber(int64(12345))
	if err != nil || got != 12345.0 {
		t.Errorf("toNumber(int64 12345): want 12345.0, got %v (err=%v)", got, err)
	}
}

func TestToNumber_UnsupportedTypes(t *testing.T) {
	cases := []struct {
		name  string
		value interface{}
	}{
		{"chaine", "not a number"},
		{"nil", nil},
		{"bool", true},
		{"pointeur", new(int)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := toNumber(tc.value)
			if err == nil {
				t.Fatalf("toNumber(%v): want erreur, got %v", tc.value, got)
			}
			if got != 0 {
				t.Errorf("toNumber(%v): valeur de repli attendue 0, got %v", tc.value, got)
			}
		})
	}
}

// Chaque fonction exposée au template doit PROPAGER l'erreur, dans les deux
// positions d'opérande : c'est ce qui fait échouer bruyamment un mauvais
// câblage au lieu d'afficher « 0 EUR ».
func TestArithmetic_PropagatesTypeError(t *testing.T) {
	bad := "pas un nombre"
	binaires := map[string]func(a, b interface{}) (float64, error){
		"mult": mult,
		"add":  add,
		"sub":  sub,
	}
	for name, fn := range binaires {
		t.Run(name, func(t *testing.T) {
			if v, err := fn(bad, 1); err == nil {
				t.Errorf("%s(bad, 1): want erreur, got %v", name, v)
			}
			if v, err := fn(1, bad); err == nil {
				t.Errorf("%s(1, bad): want erreur, got %v", name, v)
			}
		})
	}
	comparaisons := map[string]func(a, b interface{}) (bool, error){
		"ge": ge,
		"gt": gt,
	}
	for name, fn := range comparaisons {
		t.Run(name, func(t *testing.T) {
			if v, err := fn(bad, 1); err == nil {
				t.Errorf("%s(bad, 1): want erreur, got %v", name, v)
			}
			if v, err := fn(1, bad); err == nil {
				t.Errorf("%s(1, bad): want erreur, got %v", name, v)
			}
		})
	}
	t.Run("abs", func(t *testing.T) {
		if v, err := absFunc(bad); err == nil {
			t.Errorf("absFunc(bad): want erreur, got %v", v)
		}
	})
}

// --- formatFloat: edge cases ---

func TestFormatFloat_NegativeZero(t *testing.T) {
	// math.Signbit(-0.0) is true, but abs is 0 → should produce "-0,00"
	got := formatDecimal(math.Copysign(0, -1), " ", ",")
	if got != "-0,00" {
		t.Errorf("formatDecimal(-0.0): want '-0,00', got %q", got)
	}
}

func TestFormatFloat_RoundingOverflow(t *testing.T) {
	// Test the rounding overflow path: when decPart rounds up to 100
	// 99.999 → absVal=99.999, intPart=99, decPart=round(0.999*100)=100 → intPart becomes 100
	got := formatDecimal(99.999, " ", ",")
	if got != "100,00" {
		t.Errorf("formatDecimal(99.999): want '100,00', got %q", got)
	}
}

func TestFormatFloat_PositiveDecimal(t *testing.T) {
	got := formatDecimal(1234.56, " ", ",")
	if got != "1 234,56" {
		t.Errorf("formatDecimal(1234.56): want '1 234,56', got %q", got)
	}
}

func TestFormatFloat_Zero(t *testing.T) {
	got := formatDecimal(0.0, " ", ",")
	if got != "0,00" {
		t.Errorf("formatDecimal(0.0): want '0,00', got %q", got)
	}
}

func TestInit_ReadPageFileError(t *testing.T) {
	dir := makeMinimalTemplateDir(t)
	os.WriteFile(filepath.Join(dir, "layouts", "base.html"), []byte(`ok`), 0644) //nolint:errcheck
	os.WriteFile(filepath.Join(dir, "pages", "index.html"), []byte(`ok`), 0644)  //nolint:errcheck

	orig := osReadFile
	defer func() { osReadFile = orig }()
	baseRead := false
	osReadFile = func(name string) ([]byte, error) {
		if !baseRead {
			baseRead = true
			return []byte(`ok`), nil // laisser le base file réussir
		}
		return nil, errors.New("read-page-error")
	}

	err := Init(dir)
	if err == nil {
		t.Error("want error from osReadFile on page file")
	}
}

// --- Localisation de la couche monétaire (audit S-23) ---
//
// Le serveur figeait la notation française alors que PILOT_FMT.currency, côté
// navigateur, localise via Intl : un compte en anglais lisait « 1 234,56 EUR »
// rendu par le serveur à côté de « 1,234.56 EUR » rendu par le JS.

func TestMoneySeparators(t *testing.T) {
	cases := []struct {
		locale, thousands, decimal string
	}{
		{"fr", " ", ","},
		{"fr-FR", " ", ","},
		{"en", ",", "."},
		{"en-US", ",", "."},
		{"", " ", ","}, // locale absente : repli français, comportement historique
	}
	for _, tc := range cases {
		t.Run(tc.locale, func(t *testing.T) {
			th, dec := moneySeparators(tc.locale)
			if th != tc.thousands || dec != tc.decimal {
				t.Errorf("moneySeparators(%q): want (%q,%q), got (%q,%q)", tc.locale, tc.thousands, tc.decimal, th, dec)
			}
		})
	}
}

func TestCurrencyDecimals(t *testing.T) {
	if got := currencyDecimals("JPY"); got != 0 {
		t.Errorf("le yen ne subdivise pas : want 0, got %d", got)
	}
	if got := currencyDecimals("EUR"); got != 2 {
		t.Errorf("currencyDecimals(EUR): want 2, got %d", got)
	}
}

func TestFormatUnits_Locales(t *testing.T) {
	cases := []struct {
		name             string
		amount           float64
		currency, locale string
		want             string
	}{
		{"decimal fr", 1000.50, "EUR", "fr", "1 000,50 EUR"},
		{"decimal en", 1000.50, "EUR", "en", "1,000.50 EUR"},
		{"negatif en", -1000.50, "EUR", "en", "-1,000.50 EUR"},
		// JPY : aucune décimale, même sur un montant fractionnaire.
		{"yen entier", 1500.0, "JPY", "fr", "1 500 JPY"},
		{"yen arrondi", 1500.6, "JPY", "en", "1,501 JPY"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatUnits(tc.amount, tc.currency, tc.locale); got != tc.want {
				t.Errorf("formatUnits(%v,%q,%q): want %q, got %q", tc.amount, tc.currency, tc.locale, tc.want, got)
			}
		})
	}
}

// TestFormatCents fige l'unité du formateur des montants STOCKÉS : l'argument
// est un nombre de centimes, toujours divisé par 100 (audit S-40).
//
// Pendant l'audit, un test avait été écrit faux exactement ici : son auteur
// attendait que le littéral 1234567 soit lu en centimes alors que sa valeur non
// typée arrivait en `int`, donc traitée comme des unités par l'ancien formateur
// générique. Le paramètre int64 rend cette ambiguïté impossible.
func TestFormatCents(t *testing.T) {
	cases := []struct {
		name             string
		cents            int64
		currency, locale string
		want             string
	}{
		{"centimes fr", 1234567, "EUR", "fr", "12 345,67 EUR"},
		{"centimes en", 1234567, "EUR", "en", "12,345.67 EUR"},
		{"montant rond", 100000, "EUR", "fr", "1 000 EUR"},
		{"negatif", -125050, "EUR", "fr", "-1 250,50 EUR"},
		{"zero", 0, "EUR", "fr", "0 EUR"},
		{"devise vide", 5000, "", "fr", "50 EUR"},
		// Le yen est stocké comme les autres devises (×100) mais n'affiche
		// aucune décimale.
		{"yen", 150060, "JPY", "en", "1,501 JPY"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatCents(tc.cents, tc.currency, tc.locale); got != tc.want {
				t.Errorf("formatCents(%d,%q,%q): want %q, got %q", tc.cents, tc.currency, tc.locale, tc.want, got)
			}
		})
	}
}

func TestFormatUnitsCompact_Locales(t *testing.T) {
	if got := formatUnitsCompact(1500.0, "EUR", "en"); got != "1.5k EUR" {
		t.Errorf("compact en: want '1.5k EUR', got %q", got)
	}
	if got := formatUnitsCompact(-2500000.0, "EUR", "en"); got != "-2.5M EUR" {
		t.Errorf("compact en negatif: want '-2.5M EUR', got %q", got)
	}
}

func TestGroupDigits_Locales(t *testing.T) {
	if got := groupDigits(1234567, ","); got != "1,234,567" {
		t.Errorf("groupDigits en: want '1,234,567', got %q", got)
	}
	if got := groupDigits(123, " "); got != "123" {
		t.Errorf("groupDigits court: want '123', got %q", got)
	}
}

// TestReplaceFunc couvre la substitution des marqueurs i18n (audit S-20) :
// la même clé est consommée côté serveur et côté JS, le rendu doit être
// identique.
func TestReplaceFunc(t *testing.T) {
	if got := replaceFunc("Supprime aussi {n} opération(s)", "{n}", 3); got != "Supprime aussi 3 opération(s)" {
		t.Errorf("replaceFunc nombre: got %q", got)
	}
	if got := replaceFunc("Comptes : {list}", "{list}", "A, B"); got != "Comptes : A, B" {
		t.Errorf("replaceFunc chaine: got %q", got)
	}
	if got := replaceFunc("sans marqueur", "{n}", 1); got != "sans marqueur" {
		t.Errorf("replaceFunc absent: got %q", got)
	}
}

// --- Garde-fou d'unité monétaire au niveau du RENDU (audit S-40) ---
//
// C'est le test qui matérialise le finding : la même valeur, branchée sur le
// mauvais formateur, ne produit plus un chiffre faux mais une erreur qui
// interrompt le rendu.
//
// Avant, formatMoney(interface{}) déduisait l'unité du type dynamique. Sur la
// clé « Amount » de la liste des opérations récurrentes — qui mélange les
// versements d'intérêts calculés et les opérations stockées — 3086 centimes
// rendait « 30,86 EUR » et 3086.0 euros rendait « 3 086,00 EUR ». Deux
// affichages plausibles, ×100 d'écart, aucune alerte.
func TestFormatCents_RejectsUnitsFloat(t *testing.T) {
	tmpl := template.Must(template.New("t").Funcs(FuncMap).Parse(`{{formatCents .Amount "EUR" "fr"}}`))

	// Câblage correct : centimes int64.
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]interface{}{"Amount": int64(3086)}); err != nil {
		t.Fatalf("centimes int64: erreur inattendue: %v", err)
	}
	if got := buf.String(); got != "30,86 EUR" {
		t.Errorf("centimes int64: want '30,86 EUR', got %q", got)
	}

	// Mauvais câblage : le MÊME montant exprimé en euros. L'ancien formateur
	// générique affichait « 3 086,00 EUR » sans broncher.
	buf.Reset()
	if err := tmpl.Execute(&buf, map[string]interface{}{"Amount": 3086.0}); err == nil {
		t.Errorf("euros float64 sur formatCents: want erreur, got rendu %q", buf.String())
	}
}

// Symétrique : un montant stocké branché sur le formateur des agrégats est
// refusé lui aussi. Sans ce garde, 3086 centimes se seraient affichés
// « 3 086 EUR ».
func TestFormatUnits_RejectsCentsInt64(t *testing.T) {
	tmpl := template.Must(template.New("t").Funcs(FuncMap).Parse(`{{formatUnits .Total "EUR" "fr"}}`))

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]interface{}{"Total": 3086.0}); err != nil {
		t.Fatalf("euros float64: erreur inattendue: %v", err)
	}
	if got := buf.String(); got != "3 086 EUR" {
		t.Errorf("euros float64: want '3 086 EUR', got %q", got)
	}

	buf.Reset()
	if err := tmpl.Execute(&buf, map[string]interface{}{"Total": int64(3086)}); err == nil {
		t.Errorf("centimes int64 sur formatUnits: want erreur, got rendu %q", buf.String())
	}
}

// formatBalance protège l'input éditable de la ligne de compte de la même
// façon : il ne prend que des centimes.
func TestFormatBalance_RejectsUnitsFloat(t *testing.T) {
	tmpl := template.Must(template.New("t").Funcs(FuncMap).Parse(`{{formatBalance .Balance}}`))

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]interface{}{"Balance": int64(1234567)}); err != nil {
		t.Fatalf("centimes int64: erreur inattendue: %v", err)
	}
	if got := buf.String(); got != "12345.67" {
		t.Errorf("formatBalance: want '12345.67', got %q", got)
	}

	buf.Reset()
	if err := tmpl.Execute(&buf, map[string]interface{}{"Balance": 12345.67}); err == nil {
		t.Errorf("euros float64 sur formatBalance: want erreur, got rendu %q", buf.String())
	}
}

// Une valeur non numérique dans une expression arithmétique interrompt le rendu
// au lieu de valoir 0 : c'est la disparition du `default: return 0`.
func TestArithmetic_AbortsRenderOnNonNumeric(t *testing.T) {
	tmpl := template.Must(template.New("t").Funcs(FuncMap).Parse(`{{mult .V 12}}`))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]interface{}{"V": "n/a"}); err == nil {
		t.Errorf("mult sur une chaine: want erreur, got rendu %q", buf.String())
	}
}

// Les entiers non monétaires (pagination de /admin/audit) restent rendus tels
// quels : toNumber ne réinterprète plus rien.
func TestArithmetic_PageNumbers(t *testing.T) {
	tmpl := template.Must(template.New("t").Funcs(FuncMap).Parse(`{{sub .Page 1}}|{{add .Page 1}}|{{gt .Page 1}}`))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]interface{}{"Page": 3}); err != nil {
		t.Fatalf("pagination: erreur inattendue: %v", err)
	}
	if got := buf.String(); got != "2|4|true" {
		t.Errorf("pagination: want '2|4|true', got %q", got)
	}
}
