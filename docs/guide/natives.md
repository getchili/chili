# Fonctions natives

Les fonctions natives sont fournies par la carte. On les appelle comme n'importe quelle fonction. Leurs noms sont réservés : on ne peut pas appeler une variable `delay` ou `print`.

| Fonction             | Arguments          | Renvoie            | Effet                                          |
|----------------------|--------------------|--------------------|------------------------------------------------|
| `print(v)`           | un nombre          | 0                  | affiche le nombre sur une ligne                |
| `delay(ms)`          | millisecondes      | 0                  | attend `ms` millisecondes                      |
| `millis()`           | aucun              | millisecondes      | temps écoulé depuis le démarrage de la carte   |
| `random(min, max)`   | deux bornes        | un nombre          | nombre au hasard entre `min` et `max` inclus   |
| `printc(code)`       | code de caractère  | 0                  | affiche un caractère, sans retour à la ligne   |
| `println()`          | aucun              | 0                  | termine la ligne                               |
| `gpio.mode(p, m)`    | broche, mode (0-3) | 0                  | règle une broche en entrée ou en sortie        |
| `gpio.write(p, v)`   | broche, valeur     | 0                  | met une sortie à 1 (3,3 V) ou à 0 (0 V)        |
| `gpio.read(p)`       | broche             | 0 ou 1             | lit l'état d'une broche                        |

Les extraits ci-dessous se placent dans `function main()` (ou dans une autre fonction). Les exemples complets commencent par `let` ou `class`.

## `print(v)`

Affiche un nombre entier, suivi d'un retour à la ligne. La sortie apparaît dans le terminal pendant `chili push`, avec `chili monitor`, ou dans la console de Chili Lab.

```chili
print(42)
print(millis())
```

Chili ne gère pas encore le texte : `print("bonjour")` n'est pas possible.

## `delay(ms)`

Met le programme en pause pendant `ms` millisecondes (1000 = 1 seconde). Une valeur nulle ou négative ne fait rien.

```chili
delay(500)    // une demi-seconde
delay(60_000) // une minute
```

Pendant un `delay`, la carte continue de répondre aux commandes (`chili stop`, `chili push`…).

## `millis()`

Renvoie le nombre de millisecondes écoulées depuis le démarrage de la carte. Utile pour mesurer une durée ou faire plusieurs choses à des rythmes différents sans bloquer avec `delay`.

```chili
let debut = millis()
// ... travail ...
print(millis() - debut)   // durée en ms
```

Faire une action toutes les secondes sans bloquer le reste :

```chili
let prochain = millis() + 1000
while true {
    if millis() - prochain >= 0 {
        print(millis())
        prochain += 1000
    }
    // ... le reste de la boucle continue de tourner ...
    delay(10)
}
```

Le compteur dépasse la limite des entiers 32 bits au bout d'environ 24 jours et repasse en négatif. Compare toujours avec une **différence** (`millis() - prochain >= 0`) plutôt que `millis() >= prochain` : la différence reste juste même au passage de la limite.

## `printc(code)` et `println()`

`print` n'affiche que des nombres. Pour écrire du **texte**, on affiche les caractères un par un avec `printc`, en donnant leur code (65 = `A`, 97 = `a`, 48 = `0`, 33 = `!`…), puis `println()` termine la ligne.

```chili
printc(72)    // H
printc(105)   // i
printc(33)    // !
println()     // affiche « Hi! » sur une ligne
```

Les accents et autres caractères Unicode marchent aussi : `printc(233)` affiche `é`. Un code invalide (négatif, 0…) arrête le programme avec une erreur.

L'exemple `password.chl` s'en sert pour afficher un mot de passe tiré au hasard.

> `printc` et `println` demandent le firmware **0.2.1** ou plus récent (`chili flash`, ou le bandeau orange de Chili Lab).

## `random(min, max)`

Renvoie un nombre entier tiré au hasard entre `min` et `max`, **bornes comprises**. Sur l'ESP32, le hasard vient d'un générateur matériel : chaque démarrage donne une suite différente.

```chili
let de = random(1, 6)     // 1, 2, 3, 4, 5 ou 6
let pile = random(0, 1)   // 0 ou 1
delay(random(100, 500))   // attente variable
```

Si `max` est plus petit que `min`, le programme s'arrête avec l'erreur `random : le maximum (1) est plus petit que le minimum (6)`.

L'exemple `de.chl` est un dé électronique : à chaque appui sur BOOT, la LED clignote de 1 à 6 fois.

## Broches GPIO

Les broches GPIO sont les pattes numérotées de la carte (`D2`, `GPIO2`, `IO2` ou juste `2` selon la sérigraphie). Le numéro à utiliser dans Chili est le **numéro GPIO**.

### `gpio.mode(broche, mode)`

À appeler **une fois**, au début du programme, pour chaque broche utilisée.

| Mode | Nom                    | Usage                                                                 |
|------|------------------------|-----------------------------------------------------------------------|
| `0`  | entrée                 | lire un signal qui est toujours à 0 ou 1 (capteur, autre carte)       |
| `1`  | sortie                 | piloter une LED, un relais, un buzzer…                                |
| `2`  | entrée avec pull-up    | bouton branché entre la broche et **GND** : lit 1 relâché, 0 appuyé   |
| `3`  | entrée avec pull-down  | bouton branché entre la broche et **3,3 V** : lit 0 relâché, 1 appuyé |

### `gpio.write(broche, valeur)`

Sur une broche en sortie (mode 1) : `0` met la broche à 0 V, toute autre valeur la met à 3,3 V.

### `gpio.read(broche)`

Renvoie `0` ou `1` selon la tension sur la broche.

### Quelles broches utiliser sur un ESP32

| Broches                         | Remarque                                                              |
|---------------------------------|-----------------------------------------------------------------------|
| **2**                           | LED intégrée sur la plupart des cartes                                |
| **0**                           | bouton **BOOT** intégré (lit 0 quand on appuie)                        |
| 4, 5, 13, 14, 16 à 19, 21 à 23, 25 à 27, 32, 33 | libres, entrée ou sortie : à préférer                  |
| 34 à 39                         | **entrée seulement**, et sans pull-up/pull-down interne (modes 2 et 3 sans effet) |
| 6 à 11                          | **ne pas utiliser** : reliées à la mémoire flash de la carte          |
| 1 et 3                          | port USB série : les utiliser coupe la liaison avec le PC             |
| 12 et 15                        | utilisables, mais influencent le démarrage : à éviter pour débuter    |

Quand le programme est arrêté (`chili stop`, `chili kill`, nouveau `push`), toutes les broches qu'il avait mises en sortie sont remises à 0 puis en entrée : une LED ou un relais s'éteint.

## Exemple : LED

Brancher une LED externe : broche **GPIO 4** → résistance 220 Ω → patte longue de la LED ; patte courte → **GND**.

```chili
let led = 4

class Main {
    function main() {
        gpio.mode(led, 1)       // sortie
        while true {
            gpio.write(led, 1)  // allumée
            delay(200)
            gpio.write(led, 0)  // éteinte
            delay(800)
        }
    }
}
```

## Exemple : bouton

Le bouton **BOOT** de la carte est déjà branché sur la broche 0, entre la broche et GND. C'est l'exemple `bouton.chl` :

```chili
let bouton = 0
let led = 2

class Main {
    function main() {
        gpio.mode(bouton, 2) // entrée avec pull-up
        gpio.mode(led, 1)    // sortie

        while true {
            if gpio.read(bouton) == 0 {
                gpio.write(led, 1)
            } else {
                gpio.write(led, 0)
            }
            delay(10) // 100 lectures par seconde suffisent
        }
    }
}
```

Avec un bouton externe : un fil du bouton sur **GPIO 4**, l'autre sur **GND**, et `let bouton = 4`. Pas besoin de résistance grâce au mode 2.

## Exemple : compter les appuis

Pour détecter **un appui** (et pas « tant que le bouton est enfoncé »), on compare avec l'état précédent. C'est l'exemple `compteur.chl` :

```chili
let bouton = 0

class Main {
    function main() {
        gpio.mode(bouton, 2)

        let appuis = 0
        let avant = 1 // état précédent du bouton

        while true {
            let etat = gpio.read(bouton)
            if avant == 1 && etat == 0 {
                appuis += 1
                print(appuis)
            }
            avant = etat
            delay(20) // évite de compter les rebonds du bouton
        }
    }
}
```

## Sur le PC (`chili run`)

`chili run` simule les broches : `gpio.mode` et `gpio.write` affichent ce qu'ils font, et `gpio.read` renvoie la dernière valeur écrite sur la broche (0 au départ).

```
[gpio] broche 2 en mode 1
[gpio] broche 2 = 1
[gpio] broche 2 = 0
```

---

Suite : [Commandes](commandes.md)
