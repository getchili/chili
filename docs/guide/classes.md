# Classes

Une **classe** regroupe des données et les fonctions qui s'en servent. Par exemple, une classe `Led` sait sur quelle broche est sa LED et sait l'allumer. On peut ensuite créer autant de LED qu'on veut, chacune avec sa broche : ce sont des **objets**.

Tu as déjà utilisé une classe sans le savoir : tout programme commence par `class Main`.

Cette page avance pas à pas. La référence complète (pour les curieux) est dans [docs/classes.md](../classes.md).

## 1. Une première classe

But : une classe `Led` qu'on crée avec un numéro de broche, et qui sait s'allumer pendant une durée.

```chili
class Led {
    let broche                      // un champ : chaque Led a le sien

    constructor(b) {                // appelé à la création de l'objet
        this.broche = b
        gpio.mode(b, 1)
    }

    function allume(duree) {        // une méthode
        gpio.write(this.broche, 1)
        delay(duree)
        gpio.write(this.broche, 0)
    }
}

class Main {
    function main() {
        let led = new Led(2)        // crée un objet Led sur la broche 2
        while true {
            led.allume(300)         // appelle sa méthode
            delay(300)
        }
    }
}
```

Ce qu'il faut retenir :

| Écriture                    | Sens                                                            |
|-----------------------------|-----------------------------------------------------------------|
| `class Led { … }`           | déclare la classe (au niveau global, nom avec une majuscule)    |
| `let broche`                | un **champ** : une variable que chaque objet possède (0 au départ, ou `let broche = 2`) |
| `constructor(b) { … }`      | le **constructeur** : exécuté quand on crée l'objet             |
| `function allume(duree)`    | une **méthode** : une fonction qui travaille sur l'objet        |
| `this.broche`               | le champ `broche` de **cet** objet (dans la classe uniquement)  |
| `new Led(2)`                | crée un objet ; les arguments vont au constructeur              |
| `led.allume(300)`           | appelle la méthode `allume` de l'objet `led`                    |

## 2. Plusieurs objets

Chaque objet a ses propres champs. Avec une LED externe sur la broche 4 :

```chili
class Led {
    let broche

    constructor(b) {
        this.broche = b
        gpio.mode(b, 1)
    }

    function allume(duree) {
        gpio.write(this.broche, 1)
        delay(duree)
        gpio.write(this.broche, 0)
    }
}

class Main {
    function main() {
        let bleue = new Led(2)
        let rouge = new Led(4)
        while true {
            bleue.allume(200)
            rouge.allume(200)
        }
    }
}
```

## 3. Les champs sont privés

Un champ ne se lit et ne s'écrit qu'avec `this.`, **dans la classe**. De l'extérieur, on passe par une méthode :

```chili
class Compteur {
    let valeur = 0

    function ajoute() {
        this.valeur += 1
    }

    function lire() {
        return this.valeur
    }
}

class Main {
    function main() {
        let c = new Compteur()
        c.ajoute()
        c.ajoute()
        print(c.lire())     // 2  (écrire c.valeur est refusé)
    }
}
```

Dans une méthode, on appelle une autre méthode du même objet avec `this.` : `this.ajoute()`. Sans `this.`, `ajoute()` désignerait une fonction globale.

## 4. L'héritage : `extends`

Une classe peut **hériter** d'une autre : elle récupère ses champs et ses méthodes, et peut en ajouter ou en remplacer.

```chili
class Animal {
    function cri() {
        return 0
    }
}

class Chat extends Animal {
    function cri() {        // remplace la méthode de Animal
        return 1
    }
}

class Main {
    function main() {
        let a = new Animal()
        let b = new Chat()
        print(a.cri())      // 0
        print(b.cri())      // 1
    }
}
```

- Une méthode remplacée doit garder le **même nombre de paramètres**.
- `super.cri()` appelle la version de la classe parente.
- Si le constructeur de la classe parente prend des paramètres, celui de la classe fille doit l'appeler avec `super(…)`. Une classe fille sans constructeur reprend celui de son parent.

## 5. Les classes abstraites : `abstract`

Une classe **abstraite** sert de modèle : on ne peut pas en créer d'objet, seulement des classes filles. Elle peut **imposer** des méthodes avec `abstract function` (sans corps) : chaque classe fille doit les écrire.

C'est l'idée de l'exemple `motifs.chl` : un `Motif` est une façon de faire clignoter la LED. Il en existe plusieurs, mais tous savent `jouer()`.

```chili
class Led {
    let broche

    constructor(b) {
        this.broche = b
        gpio.mode(b, 1)
    }

    function allume(duree) {
        gpio.write(this.broche, 1)
        delay(duree)
        gpio.write(this.broche, 0)
    }
}

abstract class Motif {
    let led

    constructor(led) {
        this.led = led
    }

    abstract function jouer()       // chaque motif dira comment il joue

    function repeter(fois) {        // commune à tous les motifs
        let i = 0
        while i < fois {
            this.jouer()
            i += 1
        }
    }
}

class Clignote extends Motif {
    function jouer() {
        this.led.allume(300)
        delay(300)
    }
}

class Battement extends Motif {
    function jouer() {
        this.led.allume(80)
        delay(120)
        this.led.allume(80)
        delay(700)
    }
}

class Main {
    function main() {
        let led = new Led(2)
        while true {
            let m = new Clignote(led)
            if random(1, 2) == 2 {
                m = new Battement(led)
            }
            m.repeter(2)
        }
    }
}
```

Ce qui se passe :

1. `Clignote` et `Battement` n'ont pas de constructeur : ils reprennent celui de `Motif`, donc `new Clignote(led)` range `led` dans le champ `this.led`.
2. `m.repeter(2)` exécute la méthode `repeter` de `Motif`, qui appelle `this.jouer()`.
3. `this.jouer()` choisit la version de la **vraie classe** de l'objet : celle de `Clignote` ou celle de `Battement`. C'est tout l'intérêt : `repeter` est écrite une seule fois.

L'exemple complet `motifs.chl` ajoute un troisième motif, `Morse`, qui a ses propres champs et méthodes. Pour l'essayer :

```powershell
chili push motifs.chl
```

Le terminal affiche le numéro du motif tiré à chaque tour (`1`, `2` ou `3`).

## 6. Bon à savoir

- **Classes, fonctions et `let` globaux** se déclarent au niveau du fichier, dans l'ordre qu'on veut.
- Un objet est une valeur comme une autre : on peut le ranger dans une variable, un champ, le passer en paramètre ou le renvoyer.
- **`0` veut dire « pas d'objet »**. Un champ qui doit contenir un objet vaut 0 tant qu'on ne lui a rien donné.
- `a == b` est vrai si `a` et `b` désignent **le même** objet.
- La mémoire est **libérée toute seule** : un objet qu'on ne peut plus atteindre (aucune variable ni aucun champ ne le désigne) est récupéré automatiquement quand la place manque.
- La carte a une place limitée pour les objets (16 Ko, soit environ 2 000 objets à 1 champ gardés en même temps). Créer beaucoup d'objets **temporaires** ne pose pas de problème ; en **garder** des milliers à la fois, si.

## Erreurs fréquentes

| Message affiché | Cause | Correction |
|---|---|---|
| `les champs sont privés : écris this.broche dans la classe, ou passe par une méthode` | `led.broche` hors de la classe | ajoute une méthode, par exemple `function broche() { return this.broche }` |
| `'broche' n'est pas déclaré` | `broche` sans `this.` dans une méthode | écris `this.broche` |
| `fonction 'allume' non déclarée : pour appeler la méthode, écris this.allume(…)` | méthode appelée sans `this.` | écris `this.allume(…)` |
| `'Led' est une classe : pour créer un objet, écris new Led(…)` | `Led(2)` sans `new` | écris `new Led(2)` |
| `new Led attend 1 argument(s) (son constructeur), 0 fourni(s)` | mauvais nombre d'arguments à `new` | donne les arguments du constructeur |
| `Motif est abstraite : on ne peut pas en créer d'objet, crée plutôt une classe fille` | `new` sur une `abstract class` | crée une classe fille (`class Clignote extends Motif`) et fais `new Clignote(…)` |
| `la classe Clignote doit définir la méthode abstraite jouer() héritée de Motif (ou être déclarée abstract)` | méthode imposée oubliée | ajoute `function jouer() { … }` dans `Clignote` |
| `une méthode abstraite n'a pas de corps` | `abstract function jouer() { … }` | enlève `{ … }`, ou enlève `abstract` |
| `le constructeur de Rouge doit appeler super(...) : Led attend 1 paramètre(s)` | le parent a un constructeur avec paramètres | commence le constructeur par `super(4)` (par exemple) |
| `B.f redéfinit A.f : elle doit avoir 1 paramètre(s), pas 0` | méthode remplacée avec un autre nombre de paramètres | garde les mêmes paramètres que dans la classe parente |
| `aucune classe n'a de méthode 'eteint'` | méthode inexistante ou mal écrite | vérifie le nom |
| `la classe Main n'a pas de champ 'x'` | `this.x` sans `let x` dans la classe | déclare le champ : `let x` |

Erreurs à l'exécution (le programme s'arrête, la carte continue) :

| Message | Cause | Correction |
|---|---|---|
| `appel de allume() sur une valeur qui n'est pas un objet` | la variable vaut encore `0` : l'objet n'a jamais été créé | crée-le avec `new` avant de l'utiliser |
| `memoire des objets pleine (4096 mots)` | trop d'objets gardés en même temps | garde moins d'objets ; réutilise-les au lieu d'en créer sans cesse |

> Rappel : les programmes avec la nouvelle syntaxe ont besoin du **firmware 0.2.0** ou plus récent. Si `chili push` dit que le firmware gère le bytecode v1, lance `chili flash` (ou clique sur le bandeau orange de Chili Lab).

---

Suite : [Fonctions natives](natives.md)
