package db

import (
	"os/exec"
	"strings"
	"testing"
)

// modernc.org/sqlite n'est validé qu'avec la version EXACTE de
// modernc.org/libc déclarée dans son propre go.mod (doc.go de sqlite,
// « Fragile modernc.org/libc dependency »). Un `go get -u ./...` ou un bump
// groupé de Dependabot monte libc seule sans que rien ne casse à la
// compilation : ce test compare la version retenue avec celle qu'exige sqlite.
func TestLibcMatchesSQLite(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("toolchain go absente du PATH")
	}
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command(goBin, args...).Output()
		if err != nil {
			t.Fatalf("go %s: %v", strings.Join(args, " "), err)
		}
		return strings.TrimSpace(string(out))
	}

	selected := run("list", "-m", "-f", "{{.Version}}", "modernc.org/libc")
	var required string
	for _, line := range strings.Split(run("mod", "graph"), "\n") {
		from, to, ok := strings.Cut(line, " ")
		if ok && strings.HasPrefix(from, "modernc.org/sqlite@") && strings.HasPrefix(to, "modernc.org/libc@") {
			required = strings.TrimPrefix(to, "modernc.org/libc@")
		}
	}
	if required == "" {
		t.Fatal("aucune dépendance modernc.org/sqlite → modernc.org/libc dans go mod graph")
	}
	if selected != required {
		t.Errorf("modernc.org/libc %s retenue, mais modernc.org/sqlite exige %s : `go get modernc.org/libc@%s`", selected, required, required)
	}
}
