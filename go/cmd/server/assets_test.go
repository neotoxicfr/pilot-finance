package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Une empreinte par fichier, qui ne change qu'avec le contenu de CE fichier.
func TestComputeAssetVersions_PerFile(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"static/css", "static/js", "static/img"} {
		if err := os.MkdirAll(filepath.Join(dir, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p, s string) {
		if err := os.WriteFile(filepath.Join(dir, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("static/css/a.css", "a{}")
	write("static/js/b.js", "b()")
	write("static/img/c.png", "png")
	t.Chdir(dir)

	v1 := computeAssetVersions()
	if len(v1) != 2 || len(v1["css/a.css"]) != 8 || len(v1["js/b.js"]) != 8 {
		t.Fatalf("attendu 2 empreintes de 8 caractères (css+js seulement), got %v", v1)
	}
	write("static/css/a.css", "a{color:red}")
	v2 := computeAssetVersions()
	if v2["css/a.css"] == v1["css/a.css"] {
		t.Error("l'empreinte de a.css doit changer avec son contenu")
	}
	if v2["js/b.js"] != v1["js/b.js"] {
		t.Error("l'empreinte de b.js ne doit pas bouger quand seul a.css change")
	}
}
