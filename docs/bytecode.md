# Format bytecode Chili — version 2

Fichier `.chlb` produit par `chili build`, chargé par la VM (série ou HTTP).
Tous les entiers multi-octets sont en **little-endian**.

Implémentation de référence : `compiler/bytecode` (encodage, vérification,
désassembleur) et `compiler/refvm` (interpréteur Go). La VM C doit se comporter
exactement comme `refvm`.

La version 2 ajoute les classes (§5) : table des classes, table des noms et quatre
opcodes. Un firmware qui ne gère que la v1 refuse ces programmes ; `chili push`
le détecte et demande de mettre le firmware à jour (`chili flash`).

## 1. Structure du fichier

```
+--------------------------+
| en-tête (20 octets)      |
| table des fonctions      |  10 octets × num_funcs
| table des classes        |  8 octets + 4 × num_methods, pour chaque classe
| table des noms           |  pour chaque nom : longueur (u8) + octets
| code                     |  code_size octets
| CRC32 (4 octets)         |  CRC-32 IEEE de tout ce qui précède
+--------------------------+
```

### En-tête

| Offset | Taille | Champ         | Description                                      |
|--------|--------|---------------|--------------------------------------------------|
| 0      | 4      | `magic`       | `"CHLB"` (0x43 0x48 0x4C 0x42)                   |
| 4      | 1      | `version`     | `2`                                              |
| 5      | 1      | `flags`       | `0` (réservé)                                    |
| 6      | 2      | `num_globals` | nombre de variables globales                     |
| 8      | 2      | `num_funcs`   | nombre d'entrées dans la table des fonctions     |
| 10     | 2      | `entry`       | index de la fonction d'entrée                    |
| 12     | 4      | `code_size`   | taille de la section code                        |
| 16     | 2      | `num_classes` | nombre de classes                                |
| 18     | 2      | `num_names`   | nombre d'entrées dans la table des noms          |

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

### Fonction d'entrée

Le compilateur ajoute une fonction synthétique `<init>` (la dernière de la table,
désignée par `entry`) qui :

1. évalue les initialiseurs des globales dans l'ordre du source (`GSTORE`) ;
2. crée l'objet principal (`CALL Main.new`, cf. §5) ;
3. appelle sa méthode `main` (`CALL Main.main`, l'objet étant le paramètre 0, puis `POP`) ;
4. exécute `HALT`.

La VM met toutes les globales à 0, puis exécute `entry` avec une pile vide.

## 2. Modèle d'exécution

- Toutes les valeurs sont des **entiers signés 32 bits**. Arithmétique modulo 2³².
- Chaque appel crée un cadre (*frame*) : `num_locals` slots, puis la pile d'opérandes.
- **Appel** : l'appelant empile les arguments de gauche à droite puis exécute `CALL f`.
  Les `num_params` valeurs du sommet deviennent les slots 0..n-1 ; les slots
  restants sont mis à 0.
- **Retour** : `RET` dépile la valeur de retour, détruit le cadre et empile
  la valeur chez l'appelant. Une fonction sans `return` renvoie 0.
- Un `RET` dans la fonction d'entrée, ou `HALT`, termine le programme.

## 3. Jeu d'instructions

Opcode sur 1 octet, suivi de ses opérandes. Les sauts sont **relatifs à l'adresse
de l'instruction suivante** (i16).

| Code | Mnémonique    | Opérandes        | Pile                | Effet                                   |
|------|---------------|------------------|---------------------|-----------------------------------------|
| 0x00 | `HALT`        | —                | —                   | arrête le programme                     |
| 0x01 | `PUSH`        | i32              | → v                 |                                         |
| 0x02 | `PUSH8`       | i8               | → v                 | petite constante (-128..127)            |
| 0x03 | `POP`         | —                | v →                 |                                         |
| 0x10 | `LOAD`        | u8 slot          | → v                 | lit une locale                          |
| 0x11 | `STORE`       | u8 slot          | v →                 | écrit une locale                        |
| 0x12 | `GLOAD`       | u16 index        | → v                 | lit une globale                         |
| 0x13 | `GSTORE`      | u16 index        | v →                 | écrit une globale                       |
| 0x20 | `ADD`         | —                | a b → a+b           |                                         |
| 0x21 | `SUB`         | —                | a b → a-b           |                                         |
| 0x22 | `MUL`         | —                | a b → a*b           |                                         |
| 0x23 | `DIV`         | —                | a b → a/b           | tronque vers 0 ; b = 0 → erreur         |
| 0x24 | `MOD`         | —                | a b → a%b           | signe de a ; b = 0 → erreur             |
| 0x25 | `AND`         | —                | a b → a&b           | bit à bit                               |
| 0x26 | `OR`          | —                | a b → a\|b          | bit à bit                               |
| 0x27 | `XOR`         | —                | a b → a^b           |                                         |
| 0x28 | `SHL`         | —                | a b → a<<(b&31)     |                                         |
| 0x29 | `SHR`         | —                | a b → a>>(b&31)     | décalage **arithmétique**               |
| 0x2A | `NEG`         | —                | a → -a              |                                         |
| 0x2B | `NOT`         | —                | a → (a==0)          | non logique                             |
| 0x2C | `BNOT`        | —                | a → ~a              |                                         |
| 0x30 | `EQ`          | —                | a b → 0/1           |                                         |
| 0x31 | `NE`          | —                | a b → 0/1           |                                         |
| 0x32 | `LT`          | —                | a b → 0/1           |                                         |
| 0x33 | `LE`          | —                | a b → 0/1           |                                         |
| 0x34 | `GT`          | —                | a b → 0/1           |                                         |
| 0x35 | `GE`          | —                | a b → 0/1           |                                         |
| 0x40 | `JMP`         | i16              | —                   | saut inconditionnel                     |
| 0x41 | `JZ`          | i16              | v →                 | saute si v == 0                         |
| 0x42 | `JNZ`         | i16              | v →                 | saute si v != 0                         |
| 0x50 | `CALL`        | u16 fn           | args → r            | appelle une fonction de la table        |
| 0x51 | `CALL_NATIVE` | u16 id, u8 argc  | args → r            | appelle une native (cf. `natives.go`)   |
| 0x52 | `RET`         | —                | r →                 | retour de fonction                      |
| 0x60 | `ALLOC`       | u16 classe       | → obj               | crée un objet, champs à 0 (§5)          |
| 0x61 | `GETF`        | u8 champ         | obj → v             | lit un champ                            |
| 0x62 | `SETF`        | u8 champ         | obj v →             | écrit un champ                          |
| 0x63 | `CALLM`       | u16 nom, u8 argc | obj args → r        | appel de méthode (liaison dynamique)    |

Cas limites (identiques au C avec arithmétique modulo) :
`INT32_MIN / -1 = INT32_MIN`, `INT32_MIN % -1 = 0`. La VM C doit traiter ce cas
explicitement (c'est un comportement indéfini en C).

## 4. Vérification au chargement

Avant d'exécuter un programme reçu, la VM vérifie (cf. `bytecode.Verify`) :

- magic, version, taille du fichier et CRC32 ;
- `entry < num_funcs`, offsets croissants, première fonction à 0 ;
- pour chaque classe : nom valide, parent valide (ou `0xFFFF`), méthodes triées par
  nom, chaque méthode désignant une fonction existante qui a au moins un paramètre ;
- pour chaque instruction : opcode connu, opérandes entièrement dans la fonction,
  slot < `num_locals`, globale < `num_globals`, fonction < `num_funcs`,
  native connue avec la bonne arité, `ALLOC` sur une classe existante et non
  abstraite, `CALLM` avec un nom existant, cible de saut sur un début d'instruction
  de la même fonction ;
- chaque fonction se termine par `RET`, `JMP` ou `HALT`.

La profondeur de pile n'est pas vérifiée statiquement : la VM contrôle
`max_stack` à chaque `CALL` (débordement → erreur propre, pas de crash).

## 5. Classes et objets (v2)

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
  et empile son *handle*. Si la mémoire des objets est pleine, la VM lance le
  ramasse-miettes puis réessaie ; en cas d'échec : erreur d'exécution
  « mémoire des objets pleine ».
- **`GETF i` / `SETF i`** : lit ou écrit le champ d'indice `i` de l'objet. Erreur
  d'exécution si la valeur n'est pas un objet ou si `i` dépasse ses champs.
- **`CALLM sel, argc`** : l'objet est sous les `argc` arguments. La VM cherche `sel`
  dans la table de méthodes de la **classe réelle** de l'objet, vérifie que la fonction
  trouvée a `argc + 1` paramètres, puis l'appelle comme `CALL` (l'objet devient le
  slot 0). Erreurs d'exécution : valeur qui n'est pas un objet, méthode absente de
  la classe, mauvais nombre d'arguments.

### Représentation des objets (VM C)

- Les objets vivent dans une mémoire dédiée de `CHILI_HEAP_WORDS` mots de 32 bits
  (**4096** par défaut, soit 16 Ko). Un objet occupe un mot d'en-tête (taille du bloc
  sur les 16 bits de poids faible, numéro de classe + 1 sur les 16 bits de poids fort),
  puis un mot par champ. Allocation : premier bloc libre assez grand, découpé si besoin.
- Un objet est désigné par l'entier ***handle*** `0x4F000000 | offset`, où `offset`
  est la position de son en-tête dans cette mémoire. `0` n'est jamais un objet :
  il sert de « pas d'objet ». `==` compare les handles (même objet).
- **Ramasse-miettes conservatif mark-and-sweep**, lancé quand une allocation échoue :
  - racines : toute la pile de la VM (globales, locales et valeurs temporaires de tous
    les cadres) ;
  - marquage : toute valeur qui a la forme d'un handle et désigne le début d'un objet
    vivant compte comme une référence (un entier qui ressemble par hasard à un handle
    garde simplement l'objet en vie plus longtemps), puis les champs des objets
    marqués, jusqu'à stabilité ;
  - balayage : les objets non marqués sont libérés et les blocs libres voisins fusionnés.
- `refvm` (Go) applique les mêmes règles avec une mémoire des objets bien plus grande :
  un programme qui garde beaucoup d'objets vivants peut tourner avec `chili run` et
  manquer de mémoire sur la carte.
