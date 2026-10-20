# Le langage Chili

Chili est volontairement petit : des **entiers**, des **variables**, des **fonctions**, des **conditions** et des **boucles**. Cette page en fait le tour complet.

Pour tester un programme sans carte, sur le PC :

```powershell
chili check mon_programme.chl   # vérifie qu'il n'y a pas d'erreur
chili run mon_programme.chl     # l'exécute sur le PC (broches simulées)
```

## 1. Structure d'un programme

Un programme est un fichier texte en `.chl` qui contient des **fonctions** et des **variables globales**. Il doit avoir une fonction `main` : c'est elle qui est lancée au démarrage.

```chili
// blink.chl — fait clignoter la LED de la carte

let led = 2          // variable globale

fn main() {
    gpio.mode(led, 1)
    while true {
        gpio.write(led, 1)
        delay(500)
        gpio.write(led, 0)
        delay(500)
    }
}
```

- `//` commence un **commentaire** jusqu'à la fin de la ligne.
- **Pas de point-virgule** en fin de ligne : Chili les ajoute tout seul. On n'en met que pour écrire deux instructions sur une même ligne : `gpio.write(2, 1); delay(100)`.
- Les espaces et l'indentation sont libres (4 espaces est l'usage).

## 2. La règle d'or : l'accolade sur la même ligne

L'accolade ouvrante `{` doit être **sur la même ligne** que `fn`, `if`, `else` ou `while`. Et `else` doit être sur la même ligne que l'accolade fermante `}` qui le précède.

```chili
// ✅ correct
if x > 0 {
    print(x)
} else {
    print(0)
}

// ❌ incorrect
if x > 0
{
    print(x)
}
else {
    print(0)
}
```

Pourquoi : Chili ajoute un point-virgule invisible à la fin d'une ligne qui se termine par un nom, un nombre, `)` ou `}`. `if x > 0` suivi d'un retour à la ligne devient donc `if x > 0;`, ce qui n'a pas de sens.

## 3. Nombres

Chili n'a qu'**un seul type** : l'**entier signé 32 bits**, de **-2 147 483 648** à **2 147 483 647**. Pas de nombres à virgule, pas de texte (pour l'instant).

| Écriture      | Valeur  | Remarque                               |
|---------------|---------|----------------------------------------|
| `42`          | 42      | décimal                                |
| `0x2A`        | 42      | hexadécimal (`0x`)                     |
| `0b101010`    | 42      | binaire (`0b`)                         |
| `1_000_000`   | 1000000 | `_` sert de séparateur, il est ignoré  |
| `-5`          | -5      | moins unaire                           |
| `true`        | 1       |                                        |
| `false`       | 0       |                                        |

À savoir :

- **Débordement** : les calculs « font le tour ». `2147483647 + 1` donne `-2147483648`.
- **Division entière** : `7 / 2` vaut `3`, `-7 / 2` vaut `-3` (on tronque vers zéro). `-7 % 2` vaut `-1`.
- **Division par zéro** : erreur d'exécution, le programme s'arrête (la carte, elle, continue de fonctionner).
- **Vrai / faux** : `0` est faux, **toute autre valeur** est vraie. Les comparaisons renvoient `1` ou `0`.

## 4. Variables

On déclare une variable avec `let` et une valeur de départ (obligatoire) :

```chili
let vitesse = 100
let masque = 0xFF
```

On la modifie ensuite avec `=` ou un opérateur combiné :

```chili
vitesse = 200
vitesse += 10   // vitesse = vitesse + 10
vitesse -= 10
vitesse *= 2
vitesse /= 2
vitesse %= 7
```

Il n'y a pas de `++` ni de `--` : écris `i += 1`.

### Globales et locales

- Une variable déclarée **hors de toute fonction** est **globale** : toutes les fonctions la voient. Les globales sont initialisées dans l'ordre du fichier, avant `main`.
- Une variable déclarée **dans un bloc** `{ … }` est **locale** à ce bloc.
- On ne peut pas déclarer deux fois le même nom dans un même bloc. Dans un bloc imbriqué, on peut réutiliser un nom : la nouvelle variable **masque** l'ancienne jusqu'à la fin du bloc.

```chili
let compte = 0          // globale

fn ajoute() {
    compte += 1         // modifie la globale
}

fn main() {
    ajoute()
    print(compte)       // 1
    if true {
        let compte = 99 // nouvelle variable, locale au bloc
        print(compte)   // 99
    }
    print(compte)       // 1
}
```

### Noms

Un nom commence par une lettre ou `_`, puis lettres, chiffres ou `_` (sans accents) : `led`, `led_rouge`, `temps2`.

Noms **interdits** :

- les mots-clés : `fn let if else while return break continue true false` ;
- les noms des fonctions natives : `print`, `delay`, `millis`, et le module `gpio`.

## 5. Opérateurs

Du plus prioritaire au moins prioritaire :

| Priorité | Opérateurs               | Signification                                       |
|----------|--------------------------|-----------------------------------------------------|
| 1        | `-x` `!x` `~x`           | opposé, non logique (`!0` = 1), inversion des bits  |
| 2        | `*` `/` `%`              | multiplication, division entière, reste             |
| 3        | `+` `-`                  | addition, soustraction                              |
| 4        | `<<` `>>`                | décalage de bits à gauche / à droite                |
| 5        | `&`                      | ET bit à bit                                        |
| 6        | `^`                      | OU exclusif bit à bit                               |
| 7        | `\|`                     | OU bit à bit                                        |
| 8        | `<` `<=` `>` `>=`        | comparaisons                                        |
| 9        | `==` `!=`                | égal, différent                                     |
| 10       | `&&`                     | ET logique                                          |
| 11       | `\|\|`                   | OU logique                                          |

- `&&` et `||` s'arrêtent dès que le résultat est connu : dans `b != 0 && a / b > 2`, la division n'est pas faite si `b` vaut 0. Ils renvoient `1` ou `0`.
- En cas de doute, mets des parenthèses : `(a & 0x0F) == 3`.

Exemples :

```chili
print(1 << 4)            // 16
print(0b1100 & 0b1010)   // 8  (0b1000)
print(0b1100 | 0b1010)   // 14 (0b1110)
print(5 > 3 && 2 > 1)    // 1
print(!5)                // 0
```

## 6. Conditions : `if` / `else`

```chili
if temperature > 30 {
    gpio.write(ventilo, 1)
} else if temperature < 25 {
    gpio.write(ventilo, 0)
} else {
    // rien à faire
}
```

- Pas besoin de parenthèses autour de la condition (elles sont permises : `if (x > 0) {`).
- Les accolades sont **obligatoires**, même pour une seule instruction.

## 7. Boucles : `while`

Chili n'a qu'une boucle, `while` (il n'y a pas de `for`) :

```chili
let i = 0
while i < 10 {
    print(i)
    i += 1
}
```

Boucle infinie (le cas typique d'un programme de microcontrôleur) :

```chili
while true {
    // ...
}
```

### `break` et `continue`

- `break` sort immédiatement de la boucle.
- `continue` passe directement au tour suivant.

```chili
let i = 0
while true {
    i += 1
    if i % 2 == 0 {
        continue     // saute les nombres pairs
    }
    if i > 9 {
        break        // on s'arrête après 9
    }
    print(i)         // 1 3 5 7 9
}
```

## 8. Fonctions

```chili
fn carre(n) {
    return n * n
}

fn moyenne(a, b) {
    return (a + b) / 2
}

fn main() {
    print(carre(7))        // 49
    print(moyenne(10, 20)) // 15
}
```

- Une fonction prend zéro, un ou plusieurs paramètres, séparés par des virgules.
- Elle **renvoie toujours un entier** : la valeur de `return`, ou `0` s'il n'y a pas de `return` (ou un `return` seul).
- On peut l'appeler **avant** l'endroit où elle est écrite dans le fichier.
- La **récursion** est permise (une fonction qui s'appelle elle-même), dans la limite de la mémoire de la carte.
- `main` ne prend pas de paramètres.
- Une instruction seule doit être un appel de fonction : `carre(3)` est permis (le résultat est ignoré), `x + 1` tout seul ne l'est pas.

```chili
fn fib(n) {
    if n < 2 {
        return n
    }
    return fib(n - 1) + fib(n - 2)
}
```

## 9. Fonctions natives

Les fonctions fournies par la carte (`print`, `delay`, `millis`, `gpio.mode`, `gpio.write`, `gpio.read`) s'appellent comme les autres. Elles sont décrites dans [Fonctions natives](natives.md).

## 10. Erreurs fréquentes

`chili check` (et `chili push`, qui vérifie avant d'envoyer) affiche les erreurs sous la forme `fichier:ligne:colonne: message`. Voici les plus courantes.

| Message affiché | Cause | Correction |
|---|---|---|
| `b.chl:1:10: '{' attendu, trouvé fin de ligne` | accolade `{` mise à la ligne suivante | remonte `{` sur la ligne du `fn` / `if` / `while` |
| `n.chl:5:5: expression attendue, trouvé 'else'` | `else` sur une autre ligne que `}` | écris `} else {` sur une seule ligne |
| `a.chl:2:11: 'x' n'est pas déclaré` | variable non déclarée, ou faute de frappe | ajoute `let x = …` avant, ou corrige le nom |
| `t.chl:2:5: fonction 'foo' non déclarée` | fonction inexistante ou mal écrite | vérifie le nom ; définis-la avec `fn foo()` |
| `c.chl:3:9: 'a' est déjà déclaré dans cette portée (variable locale, ligne 2)` | deux `let a` dans le même bloc | la 2ᵉ fois, écris `a = 2` sans `let` |
| `v.chl:2:10: '=' attendu, trouvé fin de ligne` | `let x` sans valeur | donne une valeur de départ : `let x = 0` |
| `d.chl:2:5: delay attend 1 argument(s), 2 fourni(s)` | mauvais nombre d'arguments | voir [natives.md](natives.md) pour les arguments attendus |
| `j.chl:2:10: le module 'gpio' n'a pas de fonction 'blink'` | fonction native qui n'existe pas | fonctions disponibles : `gpio.mode`, `gpio.write`, `gpio.read` |
| `i.chl:2:9: 'delay' est une fonction native, ce nom est réservé` | variable nommée comme une native | choisis un autre nom : `let attente = 3` |
| `e.chl:1:1: le programme doit définir fn main()` | pas de fonction `main` | ajoute `fn main() { … }` |
| `x.chl:1:4: main ne doit pas avoir de paramètres (1 trouvés)` | `fn main(a)` | écris `fn main()` |
| `f.chl:2:5: 'break' en dehors d'une boucle` | `break` / `continue` hors d'un `while` | mets-le dans une boucle, ou utilise `return` |
| `w.chl:3:7: expression attendue, trouvé '+'` | `x++` | écris `x += 1` |
| `z.chl:2:12: ')' attendu, trouvé fin de ligne` | parenthèse non fermée | ferme la parenthèse sur la même ligne |
| `z5.chl:3:1: '}' attendu, trouvé fin de fichier` | accolade fermante oubliée | ajoute le `}` manquant (l'indentation aide à le trouver) |
| `z3.chl:2:14: fin d'instruction attendue, trouvé identifiant 'print'` | deux instructions sur une ligne | va à la ligne, ou sépare-les par `;` |
| `s.chl:2:13: caractère inattendu '"'` | texte entre guillemets | Chili ne gère pas encore le texte : `print` n'affiche que des nombres |
| `l.chl:2:7: on ne peut affecter qu'une variable` | `5 = 3`, ou `=` à gauche d'autre chose qu'un nom | à gauche de `=`, mets un nom de variable |
| `z2.chl:2:5: expression inutilisée : seul un appel de fonction peut servir d'instruction` | souvent un `for`, qui n'existe pas | utilise `while` |

### Erreurs d'exécution

Certaines erreurs n'apparaissent qu'au moment où le programme tourne. Le programme s'arrête et le message s'affiche :

- sur le PC (`chili run`) :
  `erreur d'exécution dans main (pc 0008) : division par zéro`
- sur la carte (`chili push` ou `chili monitor`) :
  `[chili] erreur : division par zero (fonction f0, pc 0008)`

Les plus courantes :

| Message | Cause |
|---|---|
| `division par zéro` / `modulo par zéro` | `/` ou `%` avec un diviseur qui vaut 0 |
| `trop d'appels imbriqués (récursion trop profonde ?)` | une fonction s'appelle elle-même sans fin |
| `gpio.mode : broche 99 invalide` | numéro de broche qui n'existe pas sur l'ESP32 |
| `gpio.mode : la broche 34 ne peut pas etre une sortie` | broches 34 à 39 : entrée seulement |
| `gpio.mode : mode 4 inconnu (0 a 3)` | mode différent de 0, 1, 2, 3 |

Corrige le programme et renvoie-le avec `chili push`.

---

Suite : [Fonctions natives](natives.md)
