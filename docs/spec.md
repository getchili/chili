# Spécification de Chili — v0

Langage généraliste minimal, syntaxe à accolades. Cible : VM bytecode sur microcontrôleur.

## 1. Exemple

```chili
// blink.chl
let led_pin = 2

fn main() {
    gpio.mode(led_pin, 1)
    while true {
        gpio.write(led_pin, 1)
        delay(500)
        gpio.write(led_pin, 0)
        delay(500)
    }
}
```

## 2. Lexique

- Encodage UTF-8, fichiers `.chl`.
- Commentaires : `// jusqu'à la fin de la ligne`.
- Identifiants : `[A-Za-z_][A-Za-z0-9_]*`.
- Entiers : décimal `42`, hexadécimal `0x2A`, binaire `0b101010`. `_` autorisé comme séparateur (`1_000_000`).
- Mots-clés : `fn let if else while return break continue true false`.
- Opérateurs : `+ - * / % & | ^ ~ << >> ! && || == != < <= > >= = += -= *= /= %= .`
- Délimiteurs : `( ) { } ,  ;`

### Points-virgules automatiques

Comme en Go : le lexer insère un `;` à la fin d'une ligne si le dernier token de la ligne est
un identifiant, un littéral, `true`, `false`, `return`, `break`, `continue`, `)` ou `}`.
On n'écrit donc jamais de `;` en pratique, sauf pour mettre plusieurs instructions sur une ligne.

Conséquence : l'accolade ouvrante doit être sur la même ligne (`if x {`, pas `if x\n{`).

## 3. Types

v0 : un seul type, l'**entier signé 32 bits** (`int`). Les booléens sont des entiers :
`false` = 0, `true` = 1, toute valeur non nulle est vraie. Débordement : arithmétique modulo 2³².
Division par zéro : erreur d'exécution (la VM arrête le programme).

Prévu plus tard : chaînes, tableaux fixes, flottants.

## 4. Grammaire (EBNF)

```ebnf
program     = { topDecl ";" } ;
topDecl     = fnDecl | letStmt ;
fnDecl      = "fn" IDENT "(" [ params ] ")" block ;
params      = IDENT { "," IDENT } ;

block       = "{" { statement ";" } "}" ;
statement   = letStmt | ifStmt | whileStmt | returnStmt
            | "break" | "continue" | block | simpleStmt ;
letStmt     = "let" IDENT "=" expr ;
ifStmt      = "if" expr block [ "else" ( ifStmt | block ) ] ;
whileStmt   = "while" expr block ;
returnStmt  = "return" [ expr ] ;
simpleStmt  = expr [ assignOp expr ] ;          (* l'expression de gauche doit être un IDENT *)
assignOp    = "=" | "+=" | "-=" | "*=" | "/=" | "%=" ;

expr        = binary ;                          (* voir la table de précédence *)
unary       = ( "-" | "!" | "~" ) unary | postfix ;
postfix     = primary [ "(" [ args ] ")" ] ;
args        = expr { "," expr } ;
primary     = INT | "true" | "false" | name | "(" expr ")" ;
name        = IDENT [ "." IDENT ] ;             (* "gpio.write" : nom qualifié de native *)
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

- Un programme est une suite de fonctions et de variables globales. Il doit définir `fn main()`.
- Les globales sont initialisées dans l'ordre du fichier, avant l'appel à `main`.
- `let` déclare une variable dans le bloc courant. Redéclarer un nom dans le même bloc est une erreur ;
  le masquage dans un bloc imbriqué est autorisé. Les paramètres font partie du bloc du corps :
  `fn f(a) { let a = 1 }` est une redéclaration.
- `let x = x + 1` : l'initialiseur est évalué avant la déclaration, il lit donc le `x` englobant.
- Les fonctions renvoient toujours un entier (0 si pas de `return` ou `return` sans valeur).
- Les fonctions peuvent être appelées avant leur définition dans le fichier. Récursion autorisée.
- Les noms qualifiés (`gpio.write`) désignent exclusivement des fonctions natives fournies par la VM.
  Les natives sans préfixe (`delay`, `print`) sont aussi possibles.

## 6. Fonctions natives (v0, provisoire)

| Nom              | Args           | Effet                                  |
|------------------|----------------|----------------------------------------|
| `print(v)`       | 1              | Affiche un entier (série / stdout)     |
| `delay(ms)`      | 1              | Pause en millisecondes                 |
| `millis()`       | 0              | Millisecondes depuis le démarrage      |
| `gpio.mode(p,m)` | 2              | 0 = entrée, 1 = sortie, 2 = pull-up    |
| `gpio.write(p,v)`| 2              | Écrit 0/1 sur une broche               |
| `gpio.read(p)`   | 1              | Lit une broche                         |

La table de référence est `compiler/natives/natives.go` : l'ID d'une native (encodé dans
`CALL_NATIVE`) est son index dans la table, et la VM C doit suivre le même ordre. On n'ajoute
qu'en fin de table pour ne pas casser les programmes déjà compilés.

Les noms des natives (`print`, `delay`…) et des modules (`gpio`) sont réservés : on ne peut pas
déclarer de variable ou de fonction portant ces noms.

## 7. Bytecode

Format figé dans [bytecode.md](bytecode.md) (version 1).
