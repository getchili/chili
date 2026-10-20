# Spécification de Chili — v1.1

Langage généraliste minimal, syntaxe à accolades. Cible : VM bytecode sur microcontrôleur.

Les classes (`class`, `extends`, `abstract`, `new`, `this`, `super`) sont décrites dans
[classes.md](classes.md). Les chaînes, les tableaux et la boucle `for` sont décrits dans
[strings.md](strings.md). Le langage complet nécessite le bytecode v3 et le firmware 0.3.0.

## 1. Exemple

```chili
// blink.chl
let led_pin = 2

class Main {
    function main() {
        gpio.mode(led_pin, 1)
        while true {
            gpio.write(led_pin, 1)
            delay(500)
            gpio.write(led_pin, 0)
            delay(500)
        }
    }
}
```

## 2. Lexique

- Encodage UTF-8, fichiers `.chl`.
- Commentaires : `// jusqu'à la fin de la ligne`.
- Identifiants : `[A-Za-z_][A-Za-z0-9_]*`.
- Entiers : décimal `42`, hexadécimal `0x2A`, binaire `0b101010`. `_` autorisé comme séparateur (`1_000_000`).
- Chaînes : `"texte"`, avec les échappements `\n \t \r \0 \" \\ \xHH \u{…}` (voir strings.md).
- Mots-clés : `function let if else while for in return break continue true false`
  et, pour les classes, `class abstract extends constructor this super new`.
  `fn` (ancienne syntaxe) est refusé : « 'fn' n'existe plus : écris 'function' ».
- Opérateurs : `+ - * / % & | ^ ~ << >> ! && || == != < <= > >= = += -= *= /= %= . ..`
- Délimiteurs : `( ) { } [ ] ,  ;`

### Points-virgules automatiques

Comme en Go : le lexer insère un `;` à la fin d'une ligne si le dernier token de la ligne est
un identifiant, un littéral (nombre ou chaîne), `true`, `false`, `this`, `super`, `return`, `break`,
`continue`, `)`, `]` ou `}`.
On n'écrit donc jamais de `;` en pratique, sauf pour mettre plusieurs instructions sur une ligne.

Conséquence : l'accolade ouvrante doit être sur la même ligne (`if x {`, pas `if x\n{`).

## 3. Types

Quatre sortes de valeurs, connues de la VM pendant l'exécution (pas de déclaration de type) :
l'**entier signé 32 bits**, la **chaîne**, le **tableau** et l'**objet** (voir strings.md §2 et
classes.md). `0` sert de « rien ». Les booléens sont des entiers : `false` = 0, `true` = 1 ; un
entier non nul est vrai, une chaîne, un tableau ou un objet est toujours vrai. Débordement :
arithmétique modulo 2³². Division par zéro, opération sur une sorte qui ne convient pas
(`"a" - 1`) : erreur d'exécution (la VM arrête le programme).

Prévu plus tard : flottants.

## 4. Grammaire (EBNF)

```ebnf
program     = { topDecl ";" } ;
topDecl     = classDecl | funcDecl | letStmt ;  (* classDecl : voir classes.md *)
funcDecl    = "function" IDENT "(" [ params ] ")" block ;
params      = IDENT { "," IDENT } ;

block       = "{" { statement ";" } "}" ;
statement   = letStmt | ifStmt | whileStmt | forStmt | returnStmt
            | "break" | "continue" | block | simpleStmt ;
letStmt     = "let" IDENT "=" expr ;
ifStmt      = "if" expr block [ "else" ( ifStmt | block ) ] ;
whileStmt   = "while" expr block ;
forStmt     = "for" IDENT "in" expr [ ".." expr ] block ;
returnStmt  = "return" [ expr ] ;
simpleStmt  = expr [ assignOp expr ] ;          (* à gauche : IDENT, this.champ ou x[i] *)
assignOp    = "=" | "+=" | "-=" | "*=" | "/=" | "%=" ;

expr        = binary ;                          (* voir la table de précédence *)
unary       = ( "-" | "!" | "~" ) unary | postfix ;
postfix     = primary { "." IDENT [ "(" [ args ] ")" ] | "(" [ args ] ")" | "[" expr "]" } ;
args        = expr { "," expr } ;
primary     = INT | STRING | "true" | "false" | IDENT | "this" | "super"
            | "new" IDENT "(" [ args ] ")" | "(" expr ")"
            | "[" [ expr { "," expr } ] "]" ;          (* fins de ligne ignorées entre [ et ] *)
                                                (* "gpio.write(…)" : native qualifiée ;
                                                   "obj.m(…)" : appel de méthode ;
                                                   "this.x" : champ — voir classes.md *)
```

### Précédence (de la plus faible à la plus forte)

| Niveau | Opérateurs           | Associativité |
|--------|----------------------|---------------|
| 1      | `\|\|`               | gauche        |
| 2      | `&&`                 | gauche        |
| 3      | `==` `!=`            | gauche        |
| 4      | `<` `<=` `>` `>=`    | gauche        |
| 5      | `\|`                 | gauche        |
| 6      | `^`                  | gauche        |
| 7      | `&`                  | gauche        |
| 8      | `<<` `>>`            | gauche        |
| 9      | `+` `-`              | gauche        |
| 10     | `*` `/` `%`          | gauche        |
| 11     | unaires `-` `!` `~`  | droite        |

`&&` et `||` sont court-circuités et renvoient 0 ou 1.

## 5. Sémantique

- Un programme est une suite de classes, de fonctions et de variables globales. Il doit définir
  `class Main` avec une méthode `main()` sans paramètre.
- Au démarrage : les globales sont initialisées dans l'ordre du fichier, puis la VM crée un objet
  `new Main()` et appelle sa méthode `main()`.
- `let` déclare une variable dans le bloc courant. Redéclarer un nom dans le même bloc est une erreur ;
  le masquage dans un bloc imbriqué est autorisé. Les paramètres font partie du bloc du corps :
  `function f(a) { let a = 1 }` est une redéclaration.
- `let x = x + 1` : l'initialiseur est évalué avant la déclaration, il lit donc le `x` englobant.
- Les fonctions renvoient toujours un entier (0 si pas de `return` ou `return` sans valeur).
- Les fonctions peuvent être appelées avant leur définition dans le fichier. Récursion autorisée.
- Les fonctions et les classes se déclarent uniquement au niveau global.
- Les noms qualifiés (`gpio.write`) désignent exclusivement des fonctions natives fournies par la VM.
  Les natives sans préfixe (`delay`, `print`) sont aussi possibles.

## 6. Fonctions natives

| Nom              | Args           | Effet                                  |
|------------------|----------------|----------------------------------------|
| `print(v)`       | 1              | Affiche une valeur (entier, chaîne…) sur une ligne |
| `delay(ms)`      | 1              | Pause en millisecondes                 |
| `millis()`       | 0              | Millisecondes depuis le démarrage      |
| `gpio.mode(p,m)` | 2              | 0 = entrée, 1 = sortie, 2 = pull-up, 3 = pull-down |
| `gpio.write(p,v)`| 2              | Écrit 0/1 sur une broche               |
| `gpio.read(p)`   | 1              | Lit une broche                         |
| `random(min,max)`| 2              | Entier aléatoire dans [min, max], bornes incluses (générateur matériel sur ESP32) |
| `printc(c)`      | 1              | Affiche le caractère Unicode de code c (UTF-8), sans retour à la ligne |
| `println()`      | 0              | Termine la ligne                                                        |
| `str.sub(s,a,b)` | 3              | Sous-chaîne `[a, b[` (strings.md §3)   |
| `str.find(s,m)`  | 2              | Position de `m` dans `s`, ou -1        |
| `str.char(c)`    | 1              | Chaîne d'un caractère Unicode          |
| `str.int(s)`     | 1              | Entier décimal lu dans `s` (0 si invalide) |

| `pwm.write(p,v)` | 2              | PWM 5 kHz, rapport cyclique `v` de 0 à 255 |
| `pwm.tone(p,hz)` | 2              | Signal carré de `hz` Hz (20 à 20 000), 0 = arrêt |
| `servo.write(p,a)` | 2            | Servo : impulsion de 500 à 2500 µs à 50 Hz pour `a` de 0 à 180 |
| `adc.read(p)`    | 1              | Entrée analogique, de 0 à 4095         |
| `adc.mv(p)`      | 1              | Entrée analogique, en millivolts (calibrée) |
| `i2c.begin(sda,scl)` | 2          | Broches du bus I2C (défaut : 21, 22), 100 kHz |
| `i2c.scan()`     | 0              | Tableau des adresses (1 à 126) qui répondent |
| `i2c.write(adr,d)` | 2            | Envoie `d` (chaîne ou tableau d'octets, 128 max) ; 1 si ACK, sinon 0 |
| `i2c.read(adr,n)` | 2             | Lit `n` octets (128 max) ; tableau des octets reçus |

Les natives `pwm.*`, `servo.*`, `adc.*` et `i2c.*` demandent le firmware 0.4.0 (même bytecode v3).
Une broche en PWM (`pwm`, `servo`) le reste jusqu'au prochain `gpio.mode` sur elle ; 8 broches
au plus en même temps.

Fonctions intégrées (compilées en instructions, ce ne sont pas des natives) : `len(x)`
(longueur d'une chaîne ou d'un tableau) et `array(n)` (tableau de `n` zéros).

La table de référence est `compiler/natives/natives.go` : l'ID d'une native (encodé dans
`CALL_NATIVE`) est son index dans la table, et la VM C doit suivre le même ordre. On n'ajoute
qu'en fin de table pour ne pas casser les programmes déjà compilés.

Les noms des natives (`print`, `delay`…), des modules (`gpio`, `str`, `pwm`, `servo`, `adc`, `i2c`) et des fonctions intégrées
(`len`, `array`) sont réservés : on ne peut pas déclarer de variable ou de fonction portant ces noms.

## 7. Bytecode

Format figé dans [bytecode.md](bytecode.md) (version 3).
