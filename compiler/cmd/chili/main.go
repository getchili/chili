// Commande chili : outil en ligne de commande du langage Chili.
package main

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getchili/chili/compiler/ast"
	"github.com/getchili/chili/compiler/bytecode"
	"github.com/getchili/chili/compiler/codegen"
	"github.com/getchili/chili/compiler/diag"
	"github.com/getchili/chili/compiler/firmware"
	"github.com/getchili/chili/compiler/lexer"
	"github.com/getchili/chili/compiler/natives"
	"github.com/getchili/chili/compiler/parser"
	"github.com/getchili/chili/compiler/refvm"
	"github.com/getchili/chili/compiler/sema"
)

// version de l'outil chili (le firmware embarqué a la sienne : chili flash).
const version = "0.1.0"

const usage = `usage : chili <commande> <fichier> [options]

commandes :
  tokens <f.chl>             affiche les tokens produits par le lexer
  parse  <f.chl>             affiche l'AST (S-expressions)
  check  <f.chl>             vérifie le programme (syntaxe et analyse sémantique)
  build  <f.chl> [-o f.chlb] compile en bytecode (par défaut : même nom, .chlb)
  dis    <f.chl|f.chlb>      affiche le bytecode désassemblé
  run    <f.chl|f.chlb>      exécute le programme sur le PC (GPIO simulés)

  version                    affiche la version de chili

carte (CIBLE = port série comme COM5, ou adresse IP / nom réseau ; détectée seule si absente) :
  check-port [PORT]          détecte les cartes ESP32 et affiche leur état
  flash [PORT]               installe le firmware Chili sur la carte
  push <f.chl|f.chlb> [CIBLE] envoie le programme, le lance et affiche sa sortie
  monitor [CIBLE]            affiche la sortie de la carte
  stop [CIBLE]               met le programme en pause (il repart au prochain démarrage)
  kill [CIBLE]               arrête le programme et son lancement au démarrage
  start [CIBLE]              relance le programme stocké sur la carte
  erase [CIBLE]              efface le programme de la carte
  wifi <réseau> <mot de passe> [PORT]  connecte la carte au WiFi (wifi off : coupe)
`

func main() {
	if len(os.Args) == 1 || len(os.Args) == 2 && (os.Args[1] == "help" || os.Args[1] == "--help" || os.Args[1] == "-h") {
		fmt.Print(usage)
		return
	}
	if len(os.Args) == 2 && (os.Args[1] == "version" || os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("chili %s (firmware ESP32 embarqué : %s, bytecode v%d)\n", version, firmware.Version(), bytecode.Version)
		return
	}
	if len(os.Args) >= 2 && boardCommand(os.Args[1], os.Args[2:]) {
		return
	}
	if len(os.Args) < 3 {
		usageExit()
	}
	cmd, path, rest := os.Args[1], os.Args[2], os.Args[3:]

	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if cmd != "build" && len(rest) > 0 {
		usageExit()
	}

	switch cmd {
	case "tokens":
		for _, t := range lexer.Tokenize(src) {
			fmt.Printf("%-6s %s\n", t.Pos, t)
		}
	case "parse":
		fmt.Println(mustParse(path, src))
	case "check":
		prog := mustParse(path, src)
		if _, err := sema.Check(prog); err != nil {
			fail(path, err)
		}
		fmt.Println("ok")
	case "build":
		out := strings.TrimSuffix(path, filepath.Ext(path)) + ".chlb"
		switch {
		case len(rest) == 2 && rest[0] == "-o":
			out = rest[1]
		case len(rest) != 0:
			usageExit()
		}
		data := mustCompile(path, src).Encode()
		if err := os.WriteFile(out, data, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("%s → %s (%d octets)\n", path, out, len(data))
	case "dis":
		bytecode.Disassemble(os.Stdout, load(path, src), natives.Name)
	case "run":
		runProgram(load(path, src))
	default:
		usageExit()
	}
}

func usageExit() {
	fmt.Fprint(os.Stderr, usage)
	os.Exit(2)
}

func mustParse(path string, src []byte) *ast.Program {
	prog, err := parser.Parse(src)
	if err != nil {
		fail(path, err)
	}
	return prog
}

func mustCompile(path string, src []byte) *bytecode.Program {
	prog, err := codegen.Compile(src)
	if err != nil {
		fail(path, err)
	}
	return prog
}

// load accepte un source .chl (compilé à la volée) ou un fichier .chlb.
func load(path string, data []byte) *bytecode.Program {
	if !strings.HasPrefix(string(data), bytecode.Magic) {
		return mustCompile(path, data)
	}
	prog, err := bytecode.Decode(data)
	if err == nil {
		err = bytecode.Verify(prog, natives.Arity)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s : %v\n", path, err)
		os.Exit(1)
	}
	return prog
}

// fail affiche les erreurs au format fichier:ligne:col: message et quitte.
func fail(path string, err error) {
	var errs diag.ErrorList
	if errors.As(err, &errs) {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "%s:%s\n", path, e)
		}
	} else {
		fmt.Fprintf(os.Stderr, "%s:%v\n", path, err)
	}
	os.Exit(1)
}

// runProgram exécute le programme avec des natives simulées sur PC.
func runProgram(prog *bytecode.Program) {
	start := time.Now()
	pins := map[int32]int32{}
	var vm *refvm.VM
	vm = refvm.New(prog, func(id int, v []refvm.Value) (refvm.Value, error) {
		zero := refvm.IntV(0)
		switch natives.Name(id) {
		case "print":
			fmt.Println(vm.Text(v[0]))
		case "delay":
			if v[0].I > 0 {
				time.Sleep(time.Duration(v[0].I) * time.Millisecond)
			}
		case "millis":
			return refvm.IntV(int32(time.Since(start).Milliseconds())), nil
		case "gpio.mode":
			fmt.Printf("[gpio] broche %d en mode %d\n", v[0].I, v[1].I)
		case "gpio.write":
			pins[v[0].I] = v[1].I
			fmt.Printf("[gpio] broche %d = %d\n", v[0].I, v[1].I)
		case "gpio.read":
			return refvm.IntV(pins[v[0].I]), nil
		case "random":
			r, err := natives.RandomRange(v[0].I, v[1].I, rand.Uint32())
			return refvm.IntV(r), err
		case "printc":
			if err := natives.CheckChar("printc", v[0].I); err != nil {
				return zero, err
			}
			fmt.Print(string(rune(v[0].I)))
		case "println":
			fmt.Println()
		default:
			return zero, fmt.Errorf("native %s non simulée", natives.Name(id))
		}
		return zero, nil
	})
	if err := vm.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
