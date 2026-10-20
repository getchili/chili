package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/getchili/chili/compiler/natives"
)

// Test croisé : chaque programme de programTests est exécuté par la VM C
// (vm/) et doit produire exactement la même sortie que la VM de référence.
// Le test compile lui-même la VM C ; il est ignoré si aucun compilateur C
// (gcc, cc ou clang) n'est disponible.

const vmDir = "../../vm"

func buildCVM(t *testing.T) string {
	t.Helper()
	var cc string
	for _, name := range []string{"gcc", "cc", "clang"} {
		if p, err := exec.LookPath(name); err == nil {
			cc = p
			break
		}
	}
	if cc == "" {
		t.Skip("aucun compilateur C trouvé : test de la VM C ignoré")
	}
	exe := filepath.Join(t.TempDir(), "chili-vm")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	cmd := exec.Command(cc, "-std=c99", "-O2", "-Wall", "-Wextra", "-Werror", "-pedantic",
		"-I", filepath.Join(vmDir, "src"),
		"-o", exe,
		filepath.Join(vmDir, "src", "chili_vm.c"),
		filepath.Join(vmDir, "host", "main.c"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compilation de la VM C :\n%s", out)
	}
	return exe
}

// runCVM exécute src avec la VM C et renvoie (stdout, stderr, réussite).
func runCVM(t *testing.T, exe, src string) (string, string, bool) {
	t.Helper()
	prog, err := Compile([]byte(src))
	if err != nil {
		t.Fatalf("compilation :\n%v", err)
	}
	path := filepath.Join(t.TempDir(), "prog.chlb")
	if err := os.WriteFile(path, prog.Encode(), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr strings.Builder
	cmd := exec.Command(exe, path)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	return stdout.String(), stderr.String(), err == nil
}

func TestCVMPrograms(t *testing.T) {
	exe := buildCVM(t)
	for _, tt := range programTests {
		t.Run(tt.name, func(t *testing.T) {
			out, errOut, ok := runCVM(t, exe, tt.src)
			if !ok {
				t.Fatalf("échec de la VM C : %s", errOut)
			}
			if got := strings.Join(strings.Fields(out), " "); got != tt.want {
				t.Errorf("\n got: %s\nwant: %s", got, tt.want)
			}
		})
	}
}

func TestCVMRuntimeErrors(t *testing.T) {
	exe := buildCVM(t)
	tests := []struct{ src, want string }{
		{"fn main() { let z = 0\n print(1 / z) }", "division par zero"},
		{"fn main() { let z = 0\n print(1 % z) }", "modulo par zero"},
		{"fn f(n) { return f(n + 1) }\nfn main() { f(0) }", "trop d'appels imbriques"},
	}
	for _, tt := range tests {
		_, errOut, ok := runCVM(t, exe, tt.src)
		if ok || !strings.Contains(errOut, tt.want) {
			t.Errorf("%q : stderr %q, attendu un échec contenant %q", tt.src, errOut, tt.want)
		}
	}
}

func TestCVMRejectsCorruptFile(t *testing.T) {
	exe := buildCVM(t)
	prog, err := Compile([]byte("fn main() { print(1) }"))
	if err != nil {
		t.Fatal(err)
	}
	data := prog.Encode()
	data[len(data)-5] ^= 0xFF
	path := filepath.Join(t.TempDir(), "bad.chlb")
	os.WriteFile(path, data, 0o644)
	out, err := exec.Command(exe, path).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "CRC") {
		t.Errorf("fichier corrompu accepté : %s", out)
	}
}

// La table C (vm/src/chili_natives.h) doit suivre natives.go à l'identique.
func TestNativesTableInSync(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(vmDir, "src", "chili_natives.h"))
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`X\(\s*\w+,\s*"([^"]+)",\s*(\d+),`)
	matches := re.FindAllStringSubmatch(string(src), -1)
	if len(matches) != len(natives.Table) {
		t.Fatalf("%d natives en C, %d en Go", len(matches), len(natives.Table))
	}
	for i, m := range matches {
		arity, _ := strconv.Atoi(m[2])
		if n := natives.Table[i]; m[1] != n.Name || arity != n.Arity {
			t.Errorf("native %d : C = %s/%d, Go = %s/%d", i, m[1], arity, n.Name, n.Arity)
		}
	}
}
