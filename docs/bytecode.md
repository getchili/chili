# Format bytecode Chili — version 1

Fichier `.chlb` produit par `chili build`, chargé par la VM (série ou HTTP).
Tous les entiers multi-octets sont en **little-endian**.

Implémentation de référence : `compiler/bytecode` (encodage, vérification,
désassembleur) et `compiler/refvm` (interpréteur Go). La VM C doit se comporter
exactement comme `refvm`.

## 1. Structure du fichier

```
+--------------------------+
| en-tête (16 octets)      |
| table des fonctions      |  8 octets × num_funcs
| code                     |  code_size octets
| CRC32 (4 octets)         |  CRC-32 IEEE de tout ce qui précède
+--------------------------+
```

### En-tête

| Offset | Taille | Champ         | Description                                      |
|--------|--------|---------------|--------------------------------------------------|
| 0      | 4      | `magic`       | `"CHLB"` (0x43 0x48 0x4C 0x42)                   |
| 4      | 1      | `version`     | `1`                                              |
| 5      | 1      | `flags`       | `0` (réservé)                                    |
| 6      | 2      | `num_globals` | nombre de variables globales                     |
| 8      | 2      | `num_funcs`   | nombre d'entrées dans la table des fonctions     |
| 10     | 2      | `entry`       | index de la fonction d'entrée                    |
| 12     | 4      | `code_size`   | taille de la section code                        |

### Table des fonctions (8 octets par entrée)

| Offset | Taille | Champ        | Description                                      |
|--------|--------|--------------|--------------------------------------------------|
| 0      | 4      | `offset`     | début du code de la fonction dans la section code |
| 4      | 1      | `num_params` | nombre de paramètres                             |
| 5      | 1      | `num_locals` | nombre total de slots locaux (paramètres inclus) |
| 6      | 2      | `max_stack`  | profondeur maximale de la pile d'opérandes       |

Les fonctions sont stockées dans l'ordre croissant d'`offset` et sont contiguës :
le code de la fonction *i* va de `offset[i]` à `offset[i+1]` (ou `code_size`).
La première commence à 0.

### Fonction d'entrée

Le compilateur ajoute une fonction synthétique `<init>` (la dernière de la table,
désignée par `entry`) qui :

1. évalue les initialiseurs des globales dans l'ordre du source (`GSTORE`) ;
2. appelle `main` (`CALL`, puis `POP`) ;
3. exécute `HALT`.

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

Cas limites (identiques au C avec arithmétique modulo) :
`INT32_MIN / -1 = INT32_MIN`, `INT32_MIN % -1 = 0`. La VM C doit traiter ce cas
explicitement (c'est un comportement indéfini en C).

## 4. Vérification au chargement

Avant d'exécuter un programme reçu, la VM vérifie (cf. `bytecode.Verify`) :

- magic, version, taille du fichier et CRC32 ;
- `entry < num_funcs`, offsets croissants, première fonction à 0 ;
- pour chaque instruction : opcode connu, opérandes entièrement dans la fonction,
  slot < `num_locals`, globale < `num_globals`, fonction < `num_funcs`,
  native connue avec la bonne arité, cible de saut sur un début d'instruction
  de la même fonction ;
- chaque fonction se termine par `RET`, `JMP` ou `HALT`.

La profondeur de pile n'est pas vérifiée statiquement en v1 : la VM contrôle
`max_stack` à chaque `CALL` (débordement → erreur propre, pas de crash).
