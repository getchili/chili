# Format bytecode Chili — version 3

Fichier `.chlb` produit par `chili build`, chargé par la VM (série ou HTTP).
Tous les entiers multi-octets sont en **little-endian**.

Implémentation de référence : `compiler/bytecode` (encodage, vérification,
désassembleur) et `compiler/refvm` (interpréteur Go). La VM C doit se comporter
exactement comme `refvm`.

Historique :

- **v2** ajoute les classes (§6) : table des classes, table des noms et quatre opcodes.
- **v3** ajoute les valeurs typées, les chaînes et les tableaux (§7) : table des chaînes
  constantes, sept opcodes, nouvelle signature des natives.

Un firmware ne charge que sa propre version de bytecode ; `chili push` détecte un firmware
trop ancien et demande de le mettre à jour (`chili flash`).

## 1. Structure du fichier

```
+--------------------------+
| en-tête (22 octets)      |
| table des fonctions      |  10 octets × num_funcs
| table des classes        |  8 octets + 4 × num_methods, pour chaque classe
| table des noms           |  pour chaque nom : longueur (u8) + octets
| table des chaînes        |  pour chaque chaîne : longueur (u16) + octets
| code                     |  code_size octets
| CRC32 (4 octets)         |  CRC-32 IEEE de tout ce qui précède
+--------------------------+
```

### En-tête

| Offset | Taille | Champ         | Description                                      |
|--------|--------|---------------|--------------------------------------------------|
| 0      | 4      | `magic`       | `"CHLB"` (0x43 0x48 0x4C 0x42)                   |
| 4      | 1      | `version`     | `3`                                              |
| 5      | 1      | `flags`       | `0` (réservé)                                    |
| 6      | 2      | `num_globals` | nombre de variables globales                     |
| 8      | 2      | `num_funcs`   | nombre d'entrées dans la table des fonctions     |
| 10     | 2      | `entry`       | index de la fonction d'entrée                    |
| 12     | 4      | `code_size`   | taille de la section code                        |
| 16     | 2      | `num_classes` | nombre de classes                                |
| 18     | 2      | `num_names`   | nombre d'entrées dans la table des noms          |
| 20     | 2      | `num_strings` | nombre de chaînes constantes                     |

### Table des fonctions (10 octets par entrée)

| Offset | Taille | Champ        | Description                                      |
|--------|--------|--------------|--------------------------------------------------|
| 0      | 4      | `offset`     | début du code de la fonction dans la section code |
| 4      | 1      | `num_params` | nombre de paramètres                             |
| 5      | 1      | `num_locals` | nombre total de slots locaux (paramètres inclus) |
| 6      | 2      | `max_stack`  | profondeur maximale de la pile d'opérandes       |
| 8      | 2      | `name`       | index du nom de la fonction dans la table des noms |

Les noms servent au désassembleur et aux messages d'erreur (`Main.main`, `Led.allume`,
`Led.constructor`, `Led.new`, `<init>`…).

Les fonctions sont stockées dans l'ordre croissant d'`offset` et sont contiguës :
le code de la fonction *i* va de `offset[i]` à `offset[i+1]` (ou `code_size`).
La première commence à 0.

### Table des chaînes

`num_strings` entrées : la longueur sur 2 octets (0 à 65 535) puis les octets de la chaîne
(UTF-8, sans zéro final). L'instruction `PUSHS i` empile la chaîne n° `i`. Les chaînes
constantes ne sont jamais copiées dans la mémoire des objets.

### Fonction d'entrée

Le compilateur ajoute une fonction synthétique `<init>` (la dernière de la table,
désignée par `entry`) qui :

1. évalue les initialiseurs des globales dans l'ordre du source (`GSTORE`) ;
2. crée l'objet principal (`CALL Main.new`, cf. §6) ;
3. appelle sa méthode `main` (`CALL Main.main`, l'objet étant le paramètre 0, puis `POP`) ;
4. exécute `HALT`.

La VM met toutes les globales à l'entier 0, puis exécute `entry` avec une pile vide.

## 2. Valeurs

Chaque valeur porte sa **sorte** (*tag*), connue de la VM pendant l'exécution :

| Sorte    | Contenu                                                  |
|----------|----------------------------------------------------------|
| `INT`    | entier signé 32 bits, arithmétique modulo 2³²            |
| `STR`    | chaîne d'octets immuable (constante ou dans le tas)      |
| `ARR`    | tableau de valeurs de taille fixe (dans le tas)          |
| `OBJ`    | objet d'une classe (dans le tas)                         |

`STR`, `ARR` et `OBJ` sont des **références**. Les globales, les locales, les champs et les
cases de tableau valent l'entier 0 tant qu'on ne leur a rien affecté.

**Vérité** (`JZ`, `JNZ`, `NOT`) : un `INT` est vrai s'il est non nul ; une référence est
toujours vraie.

**Texte d'une valeur** (concaténation, `print`) : `INT` → décimal (`-42`) ; `STR` → ses octets ;
`ARR` → `<tableau N>` ; `OBJ` → `<NomDeClasse>`. La concaténation n'accepte que `INT` et `STR`.

**Noms des sortes** dans les messages d'erreur : `entier`, `chaîne`, `tableau`, `objet`.

## 3. Modèle d'exécution

- Chaque appel crée un cadre (*frame*) : `num_locals` slots, puis la pile d'opérandes.
- **Appel** : l'appelant empile les arguments de gauche à droite puis exécute `CALL f`.
  Les `num_params` valeurs du sommet deviennent les slots 0..n-1 ; les slots
  restants sont mis à l'entier 0.
- **Retour** : `RET` dépile la valeur de retour, détruit le cadre et empile
  la valeur chez l'appelant. Une fonction sans `return` renvoie 0.
- Un `RET` dans la fonction d'entrée, ou `HALT`, termine le programme.

## 4. Jeu d'instructions

Opcode sur 1 octet, suivi de ses opérandes. Les sauts sont **relatifs à l'adresse
de l'instruction suivante** (i16). La colonne « Sortes » indique ce que l'instruction
accepte ; toute autre sorte provoque une erreur d'exécution.

| Code | Mnémonique    | Opérandes        | Pile                | Sortes / effet                                   |
|------|---------------|------------------|---------------------|--------------------------------------------------|
| 0x00 | `HALT`        | —                | —                   | arrête le programme                              |
| 0x01 | `PUSH`        | i32              | → v                 | entier                                           |
| 0x02 | `PUSH8`       | i8               | → v                 | petit entier (-128..127)                         |
| 0x03 | `POP`         | —                | v →                 |                                                  |
| 0x04 | `PUSHS`       | u16 index        | → s                 | chaîne constante n° index                        |
| 0x05 | `DUP2`        | —                | a b → a b a b       | duplique les deux valeurs du sommet              |
| 0x10 | `LOAD`        | u8 slot          | → v                 | lit une locale                                   |
| 0x11 | `STORE`       | u8 slot          | v →                 | écrit une locale                                 |
| 0x12 | `GLOAD`       | u16 index        | → v                 | lit une globale                                  |
| 0x13 | `GSTORE`      | u16 index        | v →                 | écrit une globale                                |
| 0x20 | `ADD`         | —                | a b → a+b           | `INT`+`INT` ; concaténation si l'un est `STR` et l'autre `INT` ou `STR` |
| 0x21 | `SUB`         | —                | a b → a-b           | `INT`                                            |
| 0x22 | `MUL`         | —                | a b → a*b           | `INT`                                            |
| 0x23 | `DIV`         | —                | a b → a/b           | `INT` ; tronque vers 0 ; b = 0 → erreur          |
| 0x24 | `MOD`         | —                | a b → a%b           | `INT` ; signe de a ; b = 0 → erreur              |
| 0x25 | `AND`         | —                | a b → a&b           | `INT`, bit à bit                                 |
| 0x26 | `OR`          | —                | a b → a\|b          | `INT`, bit à bit                                 |
| 0x27 | `XOR`         | —                | a b → a^b           | `INT`                                            |
| 0x28 | `SHL`         | —                | a b → a<<(b&31)     | `INT`                                            |
| 0x29 | `SHR`         | —                | a b → a>>(b&31)     | `INT`, décalage **arithmétique**                 |
| 0x2A | `NEG`         | —                | a → -a              | `INT`                                            |
| 0x2B | `NOT`         | —                | a → 0/1             | toutes : 1 si a est faux (§2)                    |
| 0x2C | `BNOT`        | —                | a → ~a              | `INT`                                            |
| 0x30 | `EQ`          | —                | a b → 0/1           | toutes (voir ci-dessous)                         |
| 0x31 | `NE`          | —                | a b → 0/1           | toutes : `NOT EQ`                                |
| 0x32 | `LT`          | —                | a b → 0/1           | `INT`/`INT` ou `STR`/`STR` (ordre des octets)    |
| 0x33 | `LE`          | —                | a b → 0/1           | idem                                             |
| 0x34 | `GT`          | —                | a b → 0/1           | idem                                             |
| 0x35 | `GE`          | —                | a b → 0/1           | idem                                             |
| 0x40 | `JMP`         | i16              | —                   | saut inconditionnel                              |
| 0x41 | `JZ`          | i16              | v →                 | saute si v est faux                              |
| 0x42 | `JNZ`         | i16              | v →                 | saute si v est vrai                              |
| 0x50 | `CALL`        | u16 fn           | args → r            | appelle une fonction de la table                 |
| 0x51 | `CALL_NATIVE` | u16 id, u8 argc  | args → r            | appelle une native (§5)                          |
| 0x52 | `RET`         | —                | r →                 | retour de fonction                               |
| 0x60 | `ALLOC`       | u16 classe       | → obj               | crée un objet, champs à 0 (§6)                   |
| 0x61 | `GETF`        | u8 champ         | obj → v             | `OBJ` : lit un champ                             |
| 0x62 | `SETF`        | u8 champ         | obj v →             | `OBJ` : écrit un champ                           |
| 0x63 | `CALLM`       | u16 nom, u8 argc | obj args → r        | `OBJ` : appel de méthode (liaison dynamique)     |
| 0x64 | `NEWARR`      | —                | n → t               | `INT` ≥ 0 : tableau de n cases valant 0          |
| 0x65 | `MKARR`       | u16 n            | v1 … vn → t         | tableau `[v1, …, vn]`                            |
| 0x66 | `GETI`        | —                | x i → v             | `ARR` : case i ; `STR` : octet i (0..255) ; i `INT` |
| 0x67 | `SETI`        | —                | t i v →             | `ARR` uniquement ; i `INT`                       |
| 0x68 | `LEN`         | —                | x → n               | `ARR` : nombre de cases ; `STR` : nombre d'octets |

**Égalité** (`EQ`) : deux `INT` sont égaux s'ils ont la même valeur ; deux `STR` s'ils ont le
même contenu ; deux `ARR` ou deux `OBJ` s'ils désignent le même tableau / objet. Deux valeurs
de sortes différentes ne sont jamais égales (`EQ` ne provoque jamais d'erreur).

**Indices** (`GETI`, `SETI`) : `i` doit être un `INT` dans `[0, len[`. Sinon : erreur
d'exécution « indice i hors limites (taille n) ». `SETI` sur une `STR` : erreur « les chaînes
ne sont pas modifiables ».

Cas limites de l'arithmétique (identiques au C avec arithmétique modulo) :
`INT32_MIN / -1 = INT32_MIN`, `INT32_MIN % -1 = 0`. La VM C doit traiter ce cas
explicitement (c'est un comportement indéfini en C).

### Messages d'erreur de sorte

| Situation                         | Message                                                  |
|-----------------------------------|----------------------------------------------------------|
| `SUB`…`BNOT` sur une non-`INT`    | `'-' impossible sur un(e) <sorte>` (opérateur du source) |
| `ADD` invalide                    | `'+' impossible entre <sorte> et <sorte>`                |
| `LT`… invalide                    | `'<' impossible entre <sorte> et <sorte>`                |
| `GETF`/`SETF` sur une non-`OBJ`   | `accès à un champ sur une valeur qui n'est pas un objet` |
| `CALLM` sur une non-`OBJ`         | `appel de m() sur une valeur qui n'est pas un objet`     |
| `GETI`/`SETI`/`LEN` invalide      | `<sorte> : ni un tableau ni une chaîne` / `indice non entier` |

La VM C, faute d'Unicode garanti sur toutes les consoles, écrit ses messages sans accents ;
le sens doit rester le même que dans `refvm`.

## 5. Natives

`CALL_NATIVE id, argc` appelle la native n° `id` de `compiler/natives/natives.go`.
Chaque native a une **signature** : une lettre par paramètre.

| Lettre | Sorte acceptée          |
|--------|-------------------------|
| `i`    | `INT`                   |
| `s`    | `STR`                   |
| `v`    | n'importe laquelle      |

Avant l'appel, la VM vérifie chaque argument selon la signature ; sinon erreur
d'exécution « `gpio.write` : l'argument 2 doit être un entier (reçu : chaîne) ». La native
renvoie une valeur (l'entier 0 par défaut).

Les arguments restent sur la pile pendant l'appel : une native peut allouer (une chaîne,
par exemple) sans risque que le ramasse-miettes libère ses arguments.

Les natives `str.*` sont fournies par la VM elle-même (`chili_vm.c` et `refvm`), pas par la
plateforme.

## 6. Classes et objets (v2)

### Table des classes

Pour chaque classe, un en-tête de 8 octets suivi de sa table de méthodes :

| Offset | Taille | Champ         | Description                                         |
|--------|--------|---------------|-----------------------------------------------------|
| 0      | 2      | `name`        | index du nom de la classe                           |
| 2      | 2      | `parent`      | index de la classe parente, `0xFFFF` si aucune      |
| 4      | 1      | `num_fields`  | nombre de champs, **champs hérités compris**        |
| 5      | 1      | `flags`       | bit 0 : classe abstraite                            |
| 6      | 2      | `num_methods` | nombre d'entrées de la table de méthodes            |
| 8      | 4 × n  | méthodes      | `selector` (u16, index du nom) puis `fn` (u16)      |

La table de méthodes est **complète** (méthodes héritées comprises, déjà résolues
selon les redéfinitions) et **triée par `selector`** croissant : la VM y cherche une
méthode par dichotomie. Les champs hérités occupent les premiers indices ; ceux de la
classe viennent ensuite.

### Table des noms

`num_names` entrées : un octet de longueur (0 à 255) puis les octets du nom (UTF-8).
Elle contient les noms des fonctions, des classes et des méthodes. Un même nom de
méthode (`jouer`) a un seul index, partagé par toutes les classes : c'est le
*selector* utilisé par `CALLM`.

### Fonctions générées par le compilateur

| Nom             | Paramètres                         | Rôle                                                    |
|-----------------|------------------------------------|---------------------------------------------------------|
| `C.m`           | `this`, puis ceux de `m`           | méthode `m` de la classe `C` ; `this` est le slot 0     |
| `C.constructor` | `this`, puis ceux du constructeur  | corps du constructeur                                   |
| `C.new`         | ceux du constructeur               | `ALLOC C`, initialise les champs (ceux des parents d'abord), appelle le constructeur, renvoie l'objet |

`new C(a, b)` se compile en `CALL C.new`. `super(a)` appelle `P.constructor` avec
`this` ; `super.m(a)` appelle `P.m` directement (`CALL`, sans liaison dynamique).
Les appels `obj.m(…)` et `this.m(…)` utilisent `CALLM`.

### Instructions

- **`ALLOC c`** : crée un objet de la classe `c` (non abstraite), tous ses champs à 0,
  et empile la référence. Si la mémoire des objets est pleine, la VM lance le
  ramasse-miettes puis réessaie ; en cas d'échec : erreur d'exécution
  « mémoire des objets pleine ». Même règle pour toute allocation (tableaux, chaînes).
- **`GETF i` / `SETF i`** : lit ou écrit le champ d'indice `i` de l'objet.
- **`CALLM sel, argc`** : l'objet est sous les `argc` arguments. La VM cherche `sel`
  dans la table de méthodes de la **classe réelle** de l'objet, vérifie que la fonction
  trouvée a `argc + 1` paramètres, puis l'appelle comme `CALL` (l'objet devient le
  slot 0). Erreurs d'exécution : valeur qui n'est pas un objet, méthode absente de
  la classe, mauvais nombre d'arguments.

## 7. Chaînes, tableaux, `for` (v3)

Voir `strings.md` pour le langage.

### Compilation

| Source                 | Bytecode                                                          |
|------------------------|-------------------------------------------------------------------|
| `"abc"`                | `PUSHS k`                                                         |
| `[a, b, c]`            | `a b c MKARR 3`                                                   |
| `array(n)` / `len(x)`  | `n NEWARR` / `x LEN`                                              |
| `t[i]`                 | `t i GETI`                                                        |
| `t[i] = v`             | `t i v SETI`                                                      |
| `t[i] += v`            | `t i DUP2 GETI v ADD SETI`                                        |

`for v in x { corps }` utilise deux locales cachées, `c` (la collection) et `k` (l'indice) :

```
        x ; STORE c ; PUSH8 0 ; STORE k
debut:  LOAD k ; LOAD c ; LEN ; LT ; JZ fin
        LOAD c ; LOAD k ; GETI ; STORE v
        corps
suite:  LOAD k ; PUSH8 1 ; ADD ; STORE k ; JMP debut      ; cible de continue
fin:                                                       ; cible de break
```

`for v in a..b { corps }` utilise deux locales cachées, `k` (compteur) et `f` (fin) :

```
        a ; STORE k ; b ; STORE f
debut:  LOAD k ; LOAD f ; LT ; JZ fin
        LOAD k ; STORE v
        corps
suite:  LOAD k ; PUSH8 1 ; ADD ; STORE k ; JMP debut
fin:
```

### Représentation (VM C)

- Une valeur (`chili_value`) est un entier 32 bits accompagné de sa sorte (`tag`).
  Pour une référence, l'entier est l'offset du bloc dans le tas ; pour une chaîne
  constante, c'est `0x80000000 | index` (bit de poids fort à 1).
- Le tas compte `CHILI_HEAP_WORDS` mots de 32 bits (**8192** par défaut, soit 32 Ko). Chaque
  mot a en plus une sorte sur 2 bits (table `heap_tags`), utile pour les champs et les
  cases de tableau.
- Un bloc commence par un mot d'en-tête : taille du bloc en mots (en-tête compris) sur les
  16 bits de poids faible, et sur les 16 bits de poids fort :

  | Valeur      | Bloc                                                         |
  |-------------|--------------------------------------------------------------|
  | `0`         | libre                                                        |
  | `1 … 255`   | objet de la classe `valeur - 1` ; puis un mot par champ      |
  | `0xFFFE`    | chaîne ; puis la longueur en octets, puis les octets (4 par mot) |
  | `0xFFFF`    | tableau ; puis un mot par case (taille = taille du bloc - 1) |

- **Ramasse-miettes mark-and-sweep précis**, lancé quand une allocation échoue :
  - racines : toutes les valeurs de la pile de la VM dont la sorte est une référence
    (globales, locales et valeurs temporaires de tous les cadres) ;
  - marquage : les champs et cases marqués comme références des blocs marqués, jusqu'à
    stabilité (le contenu d'une chaîne n'est jamais parcouru) ;
  - balayage : les blocs non marqués sont libérés et les blocs libres voisins fusionnés.
- Pendant une instruction qui alloue (`ADD` de chaînes, `NEWARR`, `MKARR`, `ALLOC`, natives),
  les opérandes restent sur la pile jusqu'à la fin de l'allocation.
- `refvm` (Go) applique les mêmes règles avec le ramasse-miettes de Go et sans limite de
  mémoire : un programme qui garde beaucoup de données vivantes peut tourner avec
  `chili run` et manquer de mémoire sur la carte.

## 8. Vérification au chargement

Avant d'exécuter un programme reçu, la VM vérifie (cf. `bytecode.Verify`) :

- magic, version, taille du fichier et CRC32 ;
- `entry < num_funcs`, offsets croissants, première fonction à 0 ;
- pour chaque classe : nom valide, parent valide (ou `0xFFFF`), méthodes triées par
  nom, chaque méthode désignant une fonction existante qui a au moins un paramètre ;
- pour chaque instruction : opcode connu, opérandes entièrement dans la fonction,
  slot < `num_locals`, globale < `num_globals`, fonction < `num_funcs`,
  native connue avec la bonne arité, `ALLOC` sur une classe existante et non
  abstraite, `CALLM` avec un nom existant, `PUSHS` avec une chaîne existante,
  cible de saut sur un début d'instruction de la même fonction ;
- chaque fonction se termine par `RET`, `JMP` ou `HALT`.

La profondeur de pile n'est pas vérifiée statiquement : la VM contrôle
`max_stack` à chaque `CALL` (débordement → erreur propre, pas de crash).
