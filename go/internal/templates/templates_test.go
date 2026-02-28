package templates

import (
	"bytes"
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

// --- formatMoneyCompact ---

func TestFormatMoneyCompact_Small(t *testing.T) {
	if got := formatMoneyCompact(0, "EUR"); got != "0 EUR" {
		t.Errorf("want '0 EUR', got %q", got)
	}
	if got := formatMoneyCompact(999, "EUR"); got != "999 EUR" {
		t.Errorf("want '999 EUR', got %q", got)
	}
}

func TestFormatMoneyCompact_Kilo(t *testing.T) {
	if got := formatMoneyCompact(1500, "EUR"); got != "1.5k EUR" {
		t.Errorf("want '1.5k EUR', got %q", got)
	}
	if got := formatMoneyCompact(15000, "EUR"); got != "15k EUR" {
		t.Errorf("want '15k EUR', got %q", got)
	}
}

func TestFormatMoneyCompact_Mega(t *testing.T) {
	if got := formatMoneyCompact(1500000, "EUR"); got != "1.5M EUR" {
		t.Errorf("want '1.5M EUR', got %q", got)
	}
}

func TestFormatMoneyCompact_Negative(t *testing.T) {
	got := formatMoneyCompact(-1500, "EUR")
	if got != "-1.5k EUR" {
		t.Errorf("want '-1.5k EUR', got %q", got)
	}
}

func TestFormatMoneyCompact_EmptyCurrency(t *testing.T) {
	// Empty currency defaults to EUR
	got := formatMoneyCompact(100, "")
	if got != "100 EUR" {
		t.Errorf("want '100 EUR', got %q", got)
	}
}

// --- sub ---

func TestSub(t *testing.T) {
	if got := sub(10, 3); got != 7 {
		t.Errorf("sub(10, 3): want 7, got %v", got)
	}
	if got := sub(0, 5); got != -5 {
		t.Errorf("sub(0, 5): want -5, got %v", got)
	}
}

// --- absFunc ---

func TestAbsFunc_Positive(t *testing.T) {
	if got := absFunc(5.0); got != 5.0 {
		t.Errorf("absFunc(5): want 5, got %v", got)
	}
}

func TestAbsFunc_Negative(t *testing.T) {
	if got := absFunc(-3.14); got != 3.14 {
		t.Errorf("absFunc(-3.14): want 3.14, got %v", got)
	}
}

func TestAbsFunc_Zero(t *testing.T) {
	if got := absFunc(0); got != 0 {
		t.Errorf("absFunc(0): want 0, got %v", got)
	}
}

// --- formatBalance ---

func TestFormatBalance_Integer(t *testing.T) {
	if got := formatBalance(100.0); got != "100" {
		t.Errorf("want '100', got %q", got)
	}
}

func TestFormatBalance_Decimal(t *testing.T) {
	if got := formatBalance(100.5); got != "100.50" {
		t.Errorf("want '100.50', got %q", got)
	}
}

// --- formatMoney ---

func TestFormatMoney_Integer(t *testing.T) {
	if got := formatMoney(1000, "EUR"); got != "1 000 EUR" {
		t.Errorf("want '1 000 EUR', got %q", got)
	}
}

func TestFormatMoney_Decimal(t *testing.T) {
	if got := formatMoney(1000.50, "EUR"); got != "1 000,50 EUR" {
		t.Errorf("want '1 000,50 EUR', got %q", got)
	}
}

func TestFormatMoney_EmptyCurrency(t *testing.T) {
	got := formatMoney(100, "")
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
	if got := mult(3.0, 4.0); got != 12.0 {
		t.Errorf("mult(3,4): want 12, got %v", got)
	}
	if got := mult(0, 100); got != 0 {
		t.Errorf("mult(0,100): want 0, got %v", got)
	}
}

func TestAdd(t *testing.T) {
	if got := add(2.5, 3.5); got != 6.0 {
		t.Errorf("add(2.5,3.5): want 6, got %v", got)
	}
}

// --- ge / gt ---

func TestGe(t *testing.T) {
	if !ge(5.0, 5.0) {
		t.Error("ge(5,5): want true")
	}
	if !ge(6.0, 5.0) {
		t.Error("ge(6,5): want true")
	}
	if ge(4.0, 5.0) {
		t.Error("ge(4,5): want false")
	}
}

func TestGt(t *testing.T) {
	if !gt(6.0, 5.0) {
		t.Error("gt(6,5): want true")
	}
	if gt(5.0, 5.0) {
		t.Error("gt(5,5): want false")
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
	if got := formatWithSpaces(-1234567); got != "-1 234 567" {
		t.Errorf("formatWithSpaces(-1234567): want '-1 234 567', got %q", got)
	}
}

// --- formatFloat negative decimal part ---

func TestFormatFloat_Negative(t *testing.T) {
	// negative float: intPart=-1, decPart = (-1.5 - (-1)) * 100 = -50 → abs → 50
	if got := formatFloat(-1.5); got != "-1,50" {
		t.Errorf("formatFloat(-1.5): want '-1,50', got %q", got)
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
