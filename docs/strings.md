# Chaînes, tableaux et boucle `for` — version 1

Ajout au langage de base (`spec.md`) et aux classes (`classes.md`). Nécessite le
bytecode v3 et le firmware 0.3.0.

## 1. Exemple

```chili
let notes = [262, 294, 330, 349]

class Main {
    function main() {
        let nom = "Chili"
        print("Bonjour " + nom + " !")
        print("il y a " + len(notes) + " notes")

        for n in notes {
            print(n)
        }

        let carres = array(5)
        for i in 0..len(carres) {
            carres[i] = i * i
        }
        carres[1] += 10

        for c in "abc" {
            print(c)            // 97, 98, 99 : les octets de la chaîne
        }
    }
}
```

## 2. Valeurs

Une valeur Chili est l'une des quatre sortes suivantes. La VM connaît la sorte de chaque valeur
pendant l'exécution (valeurs **typées à l'exécution**) : il n'y a pas de déclaration de type
dans le source.

| Sorte    | Exemple             | Vraie dans un `if` ?          |
|----------|---------------------|-------------------------------|
| entier   | `42`, `true`        | si différent de 0             |
| chaîne   | `"bonjour"`         | toujours                      |
| tableau  | `[1, 2]`, `array(8)`| toujours                      |
| objet    | `new Led(2)`        | toujours                      |

`0` reste la valeur « rien » (variable pas encore initialisée, champ par défaut, fonction
sans `return`). `obj == 0` vaut donc vrai seulement si `obj` est l'entier 0.

Une opération appliquée à une sorte qui ne convient pas (`"a" - 1`, `tableau.f()`,
`5[0]`…) est une **erreur d'exécution** qui arrête le programme avec un message clair.

## 3. Chaînes

### Littéraux

```chili
"bonjour"
"ligne 1\nligne 2"
"guillemet : \" et barre : \\"
"\x41"          // "A"
"\u{e9}t\u{e9}" // "été"
"été"           // le source est en UTF-8 : on peut aussi écrire directement les accents
```

Échappements : `\n` `\t` `\r` `\0` `\"` `\\` `\xHH` (un octet) `\u{H…}` (caractère Unicode,
encodé en UTF-8). Une chaîne tient sur une seule ligne et fait au plus 65 535 octets.

### Opérations

| Écriture          | Résultat                                                              |
|-------------------|-----------------------------------------------------------------------|
| `a + b`           | concaténation si `a` ou `b` est une chaîne ; un entier est converti en texte décimal (`"x" + 5` → `"x5"`) |
| `len(s)`          | nombre d'**octets** (`len("été")` vaut 5)                            |
| `s[i]`            | octet n° `i`, de 0 à 255 (`"A"[0]` vaut 65)                           |
| `a == b`, `a != b`| compare le **contenu**                                                |
| `<` `<=` `>` `>=` | ordre lexicographique des octets (`"abc" < "abd"`)                    |
| `for c in s`      | parcourt les octets                                                   |

- Les chaînes sont **immuables** : `s[0] = 65` est une erreur d'exécution. Pour obtenir une
  autre chaîne, on en construit une nouvelle (`+`, `str.sub`, `str.char`).
- `+` entre une chaîne et un objet ou un tableau est une erreur.
- Un indice hors de `[0, len(s)[` est une erreur d'exécution. Pas d'indice négatif.

### Natives de chaînes

| Native                   | Effet                                                          |
|--------------------------|----------------------------------------------------------------|
| `str.sub(s, debut, fin)` | octets `[debut, fin[` ; les bornes sont ramenées dans `[0, len(s)]` |
| `str.find(s, motif)`     | position de la première occurrence de `motif`, ou -1           |
| `str.char(c)`            | chaîne d'un caractère Unicode (`str.char(233)` → `"é"`)        |
| `str.int(s)`             | entier décimal lu dans `s` (`"-42"` → -42) ; 0 si `s` n'est pas un nombre |

`print(v)` affiche les entiers et les chaînes ; un tableau s'affiche `<tableau 3>` et un objet
`<Chien>`.

## 4. Tableaux

### Création

```chili
let vide = []
let notes = [262, 294, 330]
let melange = [1, "deux", new Led(3), [4, 5]]
let buffer = array(16)        // 16 cases valant 0
```

Un littéral peut s'étendre sur plusieurs lignes :

```chili
let motif = [
    1, 0, 1,
    0, 1, 0
]
```

### Opérations

| Écriture        | Effet                                                      |
|-----------------|------------------------------------------------------------|
| `t[i]`          | lit la case `i`                                            |
| `t[i] = v`      | écrit la case `i` (`+=`, `-=`… aussi)                      |
| `len(t)`        | nombre de cases                                            |
| `a == b`        | vrai si `a` et `b` désignent le **même** tableau           |
| `for v in t`    | parcourt les cases                                         |

- La taille est fixée à la création : pas d'ajout ni de suppression.
- Les cases peuvent contenir n'importe quelle valeur (tableaux de tableaux = 2D).
- Un tableau est une référence, comme un objet : `let b = a` puis `b[0] = 9` modifie aussi `a`.
- Indice hors de `[0, len(t)[` ou taille de `array(n)` négative : erreur d'exécution.
- Les tableaux et les chaînes vivent dans la même mémoire que les objets et sont récupérés par
  le ramasse-miettes.

## 5. Boucle `for`

```chili
for v in expr { … }        // parcourt un tableau (les cases) ou une chaîne (les octets)
for i in debut..fin { … }  // i = debut, debut + 1, …, fin - 1
```

- `debut..fin` exclut `fin` : `for i in 0..3` donne 0, 1, 2. Si `debut >= fin`, la boucle ne
  tourne pas. Les deux bornes sont évaluées **une seule fois**, avant la boucle, et doivent
  être des entiers.
- La variable de boucle est une nouvelle variable locale, visible seulement dans le corps.
  La modifier dans le corps ne change pas le déroulement de la boucle.
- `break` et `continue` fonctionnent comme dans `while` ; `continue` passe à l'élément suivant.
- `for v in t` lit `len(t)` à chaque tour ; la case est lue au début de chaque tour.

## 6. Mots réservés

- Nouveaux mots-clés : `for`, `in`.
- Nouvelles fonctions intégrées : `len(x)` et `array(n)`. Leurs noms sont réservés, comme
  ceux des natives.
- Nouveau module natif : `str`.

## 7. Grammaire (ajouts à `spec.md` §4)

```ebnf
statement   = … | forStmt ;
forStmt     = "for" IDENT "in" expr [ ".." expr ] block ;
simpleStmt  = expr [ assignOp expr ] ;      (* à gauche : IDENT, this.champ ou x[i] *)

postfix     = primary { "." IDENT [ "(" [ args ] ")" ] | "(" [ args ] ")" | "[" expr "]" } ;
primary     = … | STRING | "[" [ expr { "," expr } ] "]" ;
```

Dans un littéral de tableau, les fins de ligne sont ignorées.

## 8. Bytecode (v3)

Voir `bytecode.md` : table des chaînes constantes, valeurs typées, opcodes `DUP2`, `PUSHS`,
`NEWARR`, `MKARR`, `GETI`, `SETI`, `LEN`.
