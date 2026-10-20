# Luminosité, son, servos, capteurs et I2C

Avec `gpio.write`, une broche ne connaît que deux états : 0 V ou 3,3 V. Les fonctions de cette page vont plus loin : régler la luminosité d'une LED, jouer une note, positionner un servomoteur, mesurer une tension, dialoguer avec un écran ou un capteur.

> Ces fonctions demandent le firmware **0.4.0** ou plus récent : `chili flash`, ou le bandeau orange de Chili Lab.

| Fonction                  | Renvoie              | Effet                                                         |
|---------------------------|----------------------|---------------------------------------------------------------|
| `pwm.write(broche, v)`    | 0                    | luminosité / puissance de 0 (éteint) à 255 (à fond)           |
| `pwm.tone(broche, hz)`    | 0                    | joue un son de `hz` hertz (20 à 20 000) ; 0 = silence         |
| `servo.write(broche, a)`  | 0                    | place un servomoteur à l'angle `a` (0 à 180 degrés)           |
| `adc.read(broche)`        | 0 à 4095             | mesure la tension d'une entrée analogique                     |
| `adc.mv(broche)`          | millivolts           | la même mesure, en millivolts (0 à environ 3300)              |
| `i2c.begin(sda, scl)`     | 0                    | choisit les broches du bus I2C (par défaut SDA 21, SCL 22)    |
| `i2c.scan()`              | un tableau           | adresses des périphériques I2C qui répondent                  |
| `i2c.write(adr, données)` | 1 ou 0               | envoie des octets ; 1 si le périphérique a répondu            |
| `i2c.read(adr, n)`        | un tableau           | lit `n` octets (tableau vide si pas de réponse)               |

## 1. Luminosité : `pwm.write`

La broche s'allume et s'éteint très vite (5 000 fois par seconde). En réglant la part du temps où elle est allumée, on règle la luminosité d'une LED ou la vitesse d'un petit moteur (à travers un transistor). C'est la **PWM**.

```chili
pwm.write(2, 0)      // éteinte
pwm.write(2, 64)     // un quart
pwm.write(2, 255)    // à fond
```

Exemple complet : `respire.chl` (la LED de la carte s'allume et s'éteint en douceur).

- Pas besoin de `gpio.mode` avant : `pwm.write` règle la broche tout seul.
- **8 broches** au plus en PWM en même temps (son et servos compris).
- Pour revenir à `gpio.write` sur une broche, appelle d'abord `gpio.mode(broche, 1)`.

## 2. Son : `pwm.tone`

Un **buzzer passif** (celui qui n'émet rien quand on le branche simplement sur 3,3 V) joue la note qu'on lui envoie :

```chili
pwm.tone(25, 440)    // un la
delay(500)
pwm.tone(25, 0)      // silence
```

| Note | do  | ré  | mi  | fa  | sol | la  | si  | do (aigu) |
|------|-----|-----|-----|-----|-----|-----|-----|-----------|
| Hz   | 262 | 294 | 330 | 349 | 392 | 440 | 494 | 523       |

Exemple complet : `buzzer.chl` joue « Au clair de la lune » à partir de deux tableaux (les notes et leurs durées).

## 3. Servomoteur : `servo.write`

Un servomoteur (type SG90) tourne jusqu'à un angle précis et y reste.

```chili
servo.write(13, 0)     // tout à gauche
servo.write(13, 90)    // au milieu
servo.write(13, 180)   // tout à droite
```

Branchement : fil **orange** (signal) sur la broche, **rouge** sur **5V** (ou VIN), **marron** sur **GND**. Exemple complet : `servo.chl`.

## 4. Mesurer une tension : `adc.read` et `adc.mv`

Les entrées analogiques mesurent une tension entre 0 et 3,3 V : `adc.read` renvoie un nombre de **0** (0 V) à **4095** (3,3 V), `adc.mv` la tension en millivolts.

```chili
let valeur = adc.read(34)        // 0 à 4095
let tension = adc.mv(34)         // en mV
pwm.write(2, valeur * 255 / 4095) // ramené à l'échelle 0..255
```

Exemple complet : `potentiometre.chl` (un potentiomètre règle la luminosité de la LED). Branchement : broche du milieu du potentiomètre sur **GPIO 34**, les deux autres sur **3V3** et **GND**.

Quelles broches :

| Broches                  | Remarque                                                        |
|--------------------------|-----------------------------------------------------------------|
| **32 à 39**              | à préférer : marchent toujours                                  |
| 0, 2, 4, 12 à 15, 25 à 27 | seulement quand le WiFi est coupé                              |
| les autres               | pas d'entrée analogique                                         |

Une broche qui n'est reliée à rien donne une valeur au hasard : c'est normal.

## 5. Bus I2C : écrans et capteurs

L'**I2C** relie plusieurs modules (écran OLED, capteur de température BME280, horloge…) avec seulement deux fils : **SDA** et **SCL**. Chaque module a une **adresse**, par exemple `0x3C` pour la plupart des petits écrans OLED.

Branchement par défaut : **SDA → GPIO 21**, **SCL → GPIO 22**, **VCC → 3V3**, **GND → GND**. Autres broches : `i2c.begin(sda, scl)` au début du programme.

```chili
// Quels modules sont branchés ?
for adresse in i2c.scan() {
    print(adresse)              // 60 pour un écran en 0x3C
}

// Envoyer des octets : un tableau de nombres de 0 à 255, ou un texte
if !i2c.write(0x3C, [0x00, 0xAF]) {
    print("l'écran ne répond pas")
}

// Lire : envoyer le numéro du registre, puis lire les octets
i2c.write(0x76, [0xD0])
let id = i2c.read(0x76, 1)      // [0x60] pour un BME280
```

- 128 octets au plus par `i2c.write` ou `i2c.read`.
- Ce qu'il faut envoyer dépend du module : sa fiche technique (*datasheet*) donne ses registres.

Exemple complet : `i2c.chl` cherche les modules branchés et lit un registre du premier.

## 6. Sur le PC (`chili run`)

Tout est simulé et affiché :

```
[pwm] broche 2 = 128/255
[pwm] broche 25 : son 440 Hz
[servo] broche 13 = 90° (impulsion 1500 µs)
[i2c] scan : aucun périphérique (simulation)
```

`adc.read` renvoie une valeur qui monte puis descend toute seule (de 0 à 4095 en 2 secondes), comme un potentiomètre qu'on tourne. Aucun module I2C ne répond : `i2c.scan()` renvoie `[]`, `i2c.write` renvoie 0.

## 7. Erreurs fréquentes

| Message | Cause |
|---|---|
| `pwm.write : valeur 300 hors de 0..255` | la luminosité va de 0 à 255 |
| `pwm.tone : frequence 5 Hz hors de 20..20000 (0 pour couper le son)` | note trop grave ou trop aiguë |
| `servo.write : angle 200 hors de 0..180` | l'angle va de 0 à 180 |
| `pwm.write : deja 8 broches en PWM, c'est le maximum` | trop de broches en PWM en même temps |
| `adc.read : la broche 5 n'a pas d'entree analogique (utilise 32 a 39)` | broche sans entrée analogique |
| `adc.read : la broche 4 ne marche pas avec le WiFi actif (utilise 32 a 39)` | broches 0, 2, 4, 12 à 15, 25 à 27 avec le WiFi |
| `i2c.write : les donnees doivent etre une chaine ou un tableau d'octets (0 a 255)` | une valeur du tableau dépasse 255, ou ce n'est pas un tableau |
| `native n.13 inconnue de ce firmware : mets-le a jour avec chili flash` | firmware plus ancien que 0.4.0 |
| `pwm.write : la broche 1 sert au port USB (TX), l'utiliser couperait la liaison avec le PC` | broches 1 et 3 (port USB) et 6 à 11 (mémoire flash) : choisis-en une autre |

---

Suite : [Commandes](commandes.md)
