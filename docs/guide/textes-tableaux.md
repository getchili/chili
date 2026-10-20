# Textes, tableaux et boucle `for`

Depuis le firmware **0.3.0**, Chili sait manipuler du **texte** (les chaînes), des **listes de valeurs** (les tableaux) et parcourir les deux avec la boucle **`for`**.

> Ces programmes demandent le firmware **0.3.0** ou plus récent. Si `chili push` te dit que le firmware est trop ancien, mets-le à jour avec `chili flash` (ou le bandeau orange de Chili Lab).

## 1. Le texte (les chaînes)

Un texte s'écrit entre guillemets droits `"…"` :

```chili
class Main {
    function main() {
        let nom = "Chili"
        print("Bonjour " + nom + " !")   // Bonjour Chili !
    }
}
```

### Assembler du texte avec `+`

`+` colle deux textes bout à bout. Si l'un des deux côtés est un nombre, il est converti en texte :

```chili
let t = 23
print("Température : " + t + " °C")   // Température : 23 °C
print("total = " + (2 + 3))           // total = 5
print("total = " + 2 + 3)             // total = 23 (!)
```

Attention au dernier cas : Chili calcule de gauche à droite, donc `"total = " + 2` donne déjà du texte, auquel on colle `3`. Mets des parenthèses autour des calculs.

### Caractères spéciaux

| Écriture   | Donne                                   |
|------------|-----------------------------------------|
| `\n`       | un retour à la ligne                    |
| `\t`       | une tabulation                          |
| `\"`       | un guillemet `"`                        |
| `\\`       | une barre `\`                           |
| `\u{e9}`   | le caractère Unicode `e9`, ici `é`      |

Les accents peuvent aussi s'écrire directement : `"été"`. Une chaîne tient sur une seule ligne.

### Longueur, lettres, comparaisons

```chili
let mot = "salut"
print(len(mot))        // 5 : nombre de lettres
print(mot[0])          // 115 : code de la 1re lettre ('s')
print(mot == "salut")  // 1 : même texte
print("abc" < "abd")   // 1 : ordre alphabétique
```

- `len(texte)` donne la longueur, et `texte[i]` le **code** de la lettre n° `i` (en commençant à 0). Ce code est un nombre : 65 pour `A`, 97 pour `a`, 48 pour `0`, 32 pour l'espace.
- Pour les lettres accentuées, `len` et `[i]` comptent les **octets** : `len("été")` vaut 5, car `é` occupe 2 octets.
- On ne peut pas modifier une lettre d'un texte (`mot[0] = 83` est une erreur). On fabrique un nouveau texte à la place.

### Fonctions de texte

| Fonction                  | Renvoie                                                   | Exemple                               |
|---------------------------|-----------------------------------------------------------|---------------------------------------|
| `str.sub(t, debut, fin)`  | le morceau de `t` de `debut` (inclus) à `fin` (exclu)     | `str.sub("bonjour", 0, 3)` → `"bon"`  |
| `str.find(t, motif)`      | la position de `motif` dans `t`, ou -1 s'il n'y est pas   | `str.find("bonjour", "jour")` → 3     |
| `str.char(code)`          | le caractère qui a ce code                                 | `str.char(65)` → `"A"`                |
| `str.int(t)`              | le nombre écrit dans `t` (0 si ce n'est pas un nombre)     | `str.int("42") + 1` → 43              |

## 2. Les tableaux

Un tableau est une **liste de valeurs** numérotées à partir de 0.

```chili
let notes = [262, 294, 330, 349]   // 4 valeurs

class Main {
    function main() {
        print(notes[0])     // 262 : la première
        print(len(notes))   // 4
        notes[1] = 300      // remplace la 2e valeur
        notes[2] += 5
    }
}
```

- `[a, b, c]` crée un tableau avec ces valeurs. `[]` crée un tableau vide.
- `array(n)` crée un tableau de `n` cases qui valent toutes 0 : `let mesures = array(10)`.
- `t[i]` lit la case `i`, `t[i] = v` l'écrit. Les cases vont de `0` à `len(t) - 1` : en dehors, le programme s'arrête avec l'erreur `indice 4 hors limites (taille 4)`.
- La taille d'un tableau ne change pas après sa création.
- Une case peut contenir n'importe quoi : un nombre, un texte, un objet, un autre tableau.

Un long tableau peut s'écrire sur plusieurs lignes :

```chili
let motif = [
    1, 0, 1,
    0, 1, 0
]
```

Un tableau de tableaux forme une grille :

```chili
let grille = [[1, 2, 3], [4, 5, 6]]
print(grille[1][0])   // 4 : ligne 1, colonne 0
```

### Un tableau est partagé

Comme un objet, un tableau rangé dans deux variables reste **le même** tableau :

```chili
let a = [1, 2, 3]
let b = a
b[0] = 99
print(a[0])   // 99 : a et b désignent le même tableau
```

## 3. La boucle `for`

`for` répète un bloc pour chaque valeur d'une suite. Deux formes :

```chili
for v in tableau { … }     // chaque case du tableau (ou chaque lettre d'un texte)
for i in debut..fin { … }  // i = debut, debut + 1, …, fin - 1
```

```chili
let notes = [262, 294, 330]

class Main {
    function main() {
        for n in notes {
            print(n)            // 262, puis 294, puis 330
        }

        for i in 0..3 {
            print(i)            // 0, 1, 2 (pas 3)
        }

        for i in 0..len(notes) {
            notes[i] = notes[i] * 2   // pour modifier les cases, on passe par l'indice
        }
    }
}
```

- `debut..fin` **n'inclut pas** `fin` : `0..len(t)` parcourt exactement toutes les cases de `t`.
- La variable (`n`, `i`) n'existe que dans la boucle. La modifier ne change pas le nombre de tours.
- `break` et `continue` marchent comme dans `while`.
- `for c in "abc"` donne les **codes** des lettres : 97, 98, 99.

## 4. Exemple complet : message en morse

L'exemple `morse.chl` fait clignoter un message en morse sur la LED de la carte. Il utilise un tableau de textes pour les codes des lettres, et deux boucles `for` : une sur les lettres du message, une sur les points et les traits.

```powershell
chili push examples\morse.chl
```

```
Message : SOS CHILI
... --- ... / -.-. .... .. .-.. ..
```

## 5. Mémoire

Les textes fabriqués pendant le programme (avec `+`, `str.sub`…) et les tableaux occupent la même mémoire que les objets : **32 Ko** sur la carte. Ce qui ne sert plus est récupéré automatiquement. Un programme qui garde beaucoup de données en même temps peut s'arrêter avec `memoire des objets pleine` ; `chili run` sur le PC a beaucoup plus de mémoire et ne montre pas forcément le problème.

## 6. Erreurs fréquentes

| Message | Cause |
|---|---|
| `'-' impossible entre une chaine et un entier` | calcul sur un texte : seul `+` marche avec du texte |
| `'+' impossible entre une chaine et un tableau` | on ne peut coller au texte que du texte ou des nombres |
| `indice 3 hors limites (taille 3)` | case qui n'existe pas : les cases vont de 0 à `len(t) - 1` |
| `les chaines ne sont pas modifiables` | `texte[i] = …` : fabrique un nouveau texte à la place |
| `indexation impossible sur un entier` | `x[0]` alors que `x` est un nombre |
| `len() impossible sur un entier` | `len(x)` ou `for v in x` alors que `x` est un nombre |
| `gpio.write : l'argument 1 doit etre un entier (recu : une chaine)` | un texte passé à une fonction qui attend un nombre |

---

Suite : [Classes](classes.md)
