# Chili — langage de programmation pour microcontrôleurs (ESP32 d'abord)

## Le projet

Chili est un langage de programmation créé pour écrire des programmes ESP32. On peut envoyer un nouveau programme à la carte (WiFi ou série) **sans reflasher le firmware**.

- Nom du langage : **Chili** (jeu de mots chip + chili 🌶️). Le nom « ChiliESP » a été envisagé, mais on garde « Chili » tout court pour ne pas lier le langage à une seule carte ni à la marque Espressif.
- Extension des fichiers source : `.chl`
- Runtimes par carte : `chili-esp32` d'abord, puis éventuellement `chili-pico`, `chili-stm32`… (même logique que les ports de MicroPython)

## Architecture retenue

**Compilateur sur PC + VM bytecode sur la carte.**

```
programme.chl ──[compilateur Go, sur PC]──> bytecode ──[WiFi/série]──> VM C sur ESP32
```

- **Compilateur** en **Go** : lexer → parser (récursif descendant écrit à la main) → AST → analyse sémantique → génération de bytecode.
- **VM** en **C** : VM à pile, légère (quelques dizaines de Ko), intégrée dans un projet PlatformIO pour l'ESP32. Elle doit d'abord être testable sur PC.
- **Fonctions natives** exposées par la VM : `gpio`, `delay`, `led.set`, `wifi`, timers…
- **Chargement à chaud** : endpoint HTTP sur l'ESP32, et CLI côté PC (`chili push`).

Alternatives écartées :
- Transpileur vers C++ : plus simple, mais impose de recompiler et reflasher à chaque modification.
- Interpréteur complet embarqué (façon MicroPython) : trop lourd en RAM, avec un GC complexe.

## Structure du dépôt proposée

```
chili/
├── CLAUDE.md
├── compiler/        # Go : lexer, parser, AST, codegen, CLI `chili`
├── vm/              # C : VM portable (testable sur PC)
├── runtimes/
│   └── esp32/       # Projet PlatformIO : VM + natives + serveur de chargement
├── examples/        # Programmes .chl (blink.chl, etc.)
└── docs/            # Spec du langage et du bytecode
```

## Plan de réalisation

1. Définir la syntaxe minimale : variables, `if`, `while`, fonctions, appels natifs. Pas d'objets ni de GC au début.
2. Écrire le lexer et le parser en Go, avec des tests unitaires.
3. Concevoir le jeu d'instructions bytecode : `PUSH`, `LOAD`, `STORE`, `ADD`/`SUB`/…, `JMP`, `JZ`, `CALL`, `CALL_NATIVE`, `RET`.
4. Écrire le compilateur AST → bytecode en Go.
5. Écrire la VM en C, testée sur PC, puis portée sur l'ESP32 avec PlatformIO.
6. Ajouter la table des fonctions natives (GPIO, timers, LED).
7. Mettre en place le chargement OTA du bytecode (endpoint HTTP sur l'ESP32 et `chili push` côté PC).
8. Plus tard : event loop (`on button_press { … }`), coroutines, serveur LSP, coloration Tree-sitter.

## Décisions prises

- Orientation : langage **généraliste minimal** (un DSL pourra venir par-dessus plus tard).
- Syntaxe à **accolades**, points-virgules insérés automatiquement en fin de ligne (comme Go). Spec : `docs/spec.md`.
- Un seul type en v0 : entier signé 32 bits.

## État

- Étapes 1 et 2 faites : spec, lexer et parser Go avec tests (`cd compiler; go test ./...`).
- Analyse sémantique faite (`compiler/sema`) : résolution des noms et portées, arité, `main`, `break`/`continue`. Elle produit `sema.Info` (slots locaux, index des globales et fonctions, ID des natives) pour le futur générateur de code.
- Table des natives de référence : `compiler/natives/natives.go` (ID = index, n'ajouter qu'en fin de table).
- Bytecode v1 figé (`docs/bytecode.md`) : paquet `compiler/bytecode` (encodage, CRC32, `Verify`, désassembleur), générateur `compiler/codegen`, VM de référence Go `compiler/refvm` (modèle exact pour la VM C).
- CLI : `go build -o ../chili.exe ./cmd/chili` puis `chili tokens|parse|check|build|dis|run fichier`. `run` exécute sur PC avec GPIO simulés.
- Objectif d'autonomie : l'utilisateur final n'installe rien d'autre que `chili.exe` (firmware ESP32 embarqué, flash en Go façon esptool, push/monitor par USB série). Protocole série : `docs/protocol.md`.
- `chili check-port [PORT]` fait (`compiler/device`) : liste les ports, reconnaît les puces USB des ESP32, interroge le firmware Chili puis le bootloader ROM (modèle de puce, MAC).
- VM C (`vm/src`, C99, sans malloc) : testée contre `refvm` par `compiler/codegen/cvm_test.go` (compile la VM avec gcc si présent ; gcc = WinLibs via winget). Table des natives C `vm/src/chili_natives.h`, synchronisée avec Go par test.
- Firmware ESP32 (`runtimes/esp32`, PlatformIO + Arduino, ESP32 classique) : programme en LittleFS lancé au boot, protocole sur USB et TCP 7070 (WiFi), mDNS `chili-xxxxxx.local`.
- `build.ps1` : compile le firmware, le copie dans `compiler/firmware/esp32` (embarqué via go:embed) puis construit `chili.exe`.
- Commandes carte : `check-port`, `flash` (esptool réimplémenté en Go, ROM sans stub, 460800 bauds, vérif MD5), `push`, `monitor`, `start|stop|erase`, `wifi`. Testées sur une vraie carte le 2026-10-20 (ESP32-D0WD-V3, CH340, COM4) : check-port, flash (28 s, MD5 ok), push et monitor fonctionnent. WiFi pas encore testé.
- Le terminal de l'utilisateur est Windows PowerShell 5.1 : donner les commandes sans `&&` (utiliser `;` et `if ($?) { … }`).

## Points encore ouverts

- Protocole de chargement (série / HTTP).

## Références

- *Crafting Interpreters* (Robert Nystrom), gratuit en ligne
- *Writing an Interpreter in Go* / *Writing a Compiler in Go* (Thorsten Ball)
- Langages existants à étudier : MicroPython, Berry (Tasmota), Toit, Lua/NodeMCU

## Préférences de travail

- Communication directe, sans blabla.
- Fichiers de code complets plutôt que des extraits partiels.
- Instructions numérotées pour les procédures complexes.
- Le moins d'étapes de setup possible.
- Installateur Windows : `installer/chili.iss` (Inno Setup, par utilisateur, PATH + icônes + extension VS Code), produit `dist/ChiliSetup-x.y.z.exe` via `build.ps1 -Installer`.
- Application de bureau **Chili Lab** : dépôt séparé `../chili-lab` (github.com/getchili/chili-lab), importe ce module via `replace ../chili/compiler`. Le module Go s'appelle `github.com/getchili/chili/compiler`. `chili board` a été retiré du CLI ; `chili` seul = aide. device.Conn diffuse les lignes à des abonnés (Subscribe).
- Firmware 0.1.3 : états running/stopped/done/error/empty, autorun (kill le coupe, stop non), id programme = CRC du contenu.
- Langage v0.2 (2026-10-20) : `function` remplace `fn` ; point d'entrée `class Main { function main() }` ; classes complètes (docs/classes.md) : champs privés via `this.x`, constructor, extends, abstract, super, new, appel de méthode dynamique. Bytecode v2 (table des classes, table des noms, ALLOC/GETF/SETF/CALLM), VM C avec ramasse-miettes conservatif (CHILI_HEAP_WORDS = 4096). Firmware 0.2.0, testé sur la carte (motifs.chl, 30 000 objets en 2,3 s).
- Tests : `compiler/codegen/legacy_test.go` convertit les anciens programmes de test (fn/main) à la volée.
- Langage v0.3 (2026-10-20) : chaînes, tableaux fixes, `for v in x` et `for i in a..b` (docs/strings.md). Valeurs **typées à l'exécution** (INT/STR/ARR/OBJ) : `refvm.Value` en Go, `chili_value` en C ; GC précis (plus conservatif). Bytecode v3 (table des chaînes, PUSHS, DUP2, NEWARR, MKARR, GETI, SETI, LEN), en-tête 22 octets. Natives avec signature (`Sig` : i/s/v, vérifiée par la VM) ; `str.*` fournies par la VM, pas la plateforme ; `len`/`array` = fonctions intégrées (opcodes). Tas C 8192 mots (32 Ko) + tags 2 bits/mot. Firmware 0.3.0 (VM de vérification allouée par malloc le temps d'un `load`). Le lexer ignore un BOM UTF-8.
- Firmware 0.4.0 (2026-10-20) : natives matériel `pwm.write`, `pwm.tone`, `servo.write` (LEDC, canaux pairs seulement = 8 broches, chacune sa fréquence), `adc.read`/`adc.mv` (ADC2 refusé si WiFi actif), `i2c.begin/scan/write/read` (Wire, 21/22 par défaut, 128 octets max). Bytecode inchangé (v3). Simulation PC : `compiler/cmd/chili/sim.go` et `vm/host/main.c`. API C pour natives : `chili_new_arr`, `chili_arr_*`, `chili_bytes`, `chili_check_range`. Guide : docs/guide/materiel.md.
