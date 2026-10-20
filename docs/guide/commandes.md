# Les commandes de `chili`

Toutes les commandes se tapent dans un terminal (PowerShell ou cmd) :

```
chili <commande> [fichier] [cible]
```

`chili help` (ou `chili` tout seul) affiche la liste. Pour tout faire à la souris, il y a l'application [Chili Lab](chili-lab.md).

**La cible** (facultative) indique à quelle carte parler :

- un **port série** : `COM4` ;
- une **adresse IP** : `192.168.1.42` ;
- un **nom réseau** : `chili-a1b2c3.local`.

Sans cible, `chili` cherche tout seul la carte branchée en USB. Avec plusieurs cartes branchées, ou en WiFi, indique la cible.

| Commande                      | Rôle                                                           |
|-------------------------------|----------------------------------------------------------------|
| [`check`](#chili-check)       | vérifie un programme sans l'exécuter                           |
| [`build`](#chili-build)       | compile en bytecode `.chlb`                                    |
| [`dis`](#chili-dis)           | affiche le bytecode                                            |
| [`run`](#chili-run)           | exécute un programme sur le PC                                 |
| [`check-port`](#chili-check-port) | détecte les cartes et affiche leur état                    |
| [`flash`](#chili-flash)       | installe le firmware Chili sur la carte                        |
| [`push`](#chili-push)         | envoie un programme à la carte et le lance                     |
| [`monitor`](#chili-monitor)   | affiche la sortie de la carte                                  |
| [`stop`](#chili-stop)         | met le programme en pause (il repart au prochain démarrage)    |
| [`kill`](#chili-kill)         | arrête le programme durablement                                |
| [`start`](#chili-start)       | relance le programme stocké                                    |
| [`erase`](#chili-erase)       | efface le programme de la carte                                |
| [`wifi`](#chili-wifi)         | connecte la carte au WiFi                                      |
| [`version`](#chili-version)   | affiche la version                                             |
| [`help`](#chili-help)         | affiche la liste des commandes                                 |

---

## Sur le PC (sans carte)

### `chili check`

Vérifie la syntaxe et le sens du programme (variables déclarées, nombre d'arguments…), sans rien exécuter.

```powershell
chili check blink.chl
```

```
ok
```

En cas d'erreur, le fichier, la ligne et la colonne sont indiqués :

```
blink.chl:7:5: 'ledd' n'est pas déclaré
```

Voir les [erreurs fréquentes](langage.md#10-erreurs-fréquentes).

### `chili build`

Compile le programme en **bytecode** (fichier `.chlb`), le format exécuté par la carte. Tu n'en as pas besoin d'habitude : `chili push` compile tout seul.

```powershell
chili build blink.chl
```

```
blink.chl → blink.chlb (111 octets)
```

Choisir le nom du fichier produit : `chili build blink.chl -o clignote.chlb`.

### `chili dis`

Affiche le bytecode produit, instruction par instruction (pour les curieux). Accepte un `.chl` ou un `.chlb`.

```powershell
chili dis blink.chl
```

```
; 2 globale(s), 2 fonction(s), entrée = <init>, 75 octets de code

fn main  ; params=0 locals=0 max_stack=2
  0000  GLOAD       0
  0003  PUSH8       1
  0005  CALL_NATIVE gpio.mode/2
  0009  POP
  ...
```

### `chili run`

Exécute le programme sur le PC. Les broches sont simulées : leur état s'affiche. Pratique pour tester la logique sans carte. **Ctrl+C** pour arrêter.

```powershell
chili run fib.chl
```

```
0
1
1
2
3
...
75025
58
```

Avec des broches :

```
[gpio] broche 2 en mode 1
[gpio] broche 2 = 1
[gpio] broche 2 = 0
```

Une erreur d'exécution arrête le programme :

```
erreur d'exécution dans main (pc 0008) : division par zéro
```

---

## Avec la carte

### `chili check-port`

Liste les ports série, reconnaît les cartes ESP32 et interroge chacune. C'est **la commande à lancer en premier** quand quelque chose ne va pas.

```powershell
chili check-port
```

Carte avec le firmware Chili :

```
Ports série :
  COM4     WCH CH340 (1A86:7523)  <- carte ESP32 probable

COM4 :
  Firmware Chili : 0.1.3 (bytecode v1)
  Puce           : ESP32-D0WD-V3, 251 Ko de mémoire libre
  WiFi           : non configuré (chili wifi <réseau> <mot de passe>)
  Programme      : 111 octets (crc 5e0c1f3a)
  État           : en cours d'exécution
```

Carte neuve, sans firmware Chili :

```
COM4 :
  Firmware Chili : absent
  Puce           : ESP32  (MAC 08:3a:f2:12:34:56)

  La carte est prête à recevoir le firmware Chili :
    chili flash COM4
```

Valeurs possibles de **État** :

| Affichage                                     | Signification                                  |
|-----------------------------------------------|------------------------------------------------|
| `en cours d'exécution`                        | le programme tourne                            |
| `en pause (chili stop), repartira au prochain démarrage` | après `chili stop`                  |
| `arrêté (chili kill), ne se relance pas au démarrage…`   | après `chili kill`                  |
| `terminé (le programme est arrivé au bout)`   | `main` s'est terminée normalement (firmware 0.1.3+) |
| `arrêté sur une erreur`                       | erreur d'exécution (voir `chili monitor`)      |

Si la carte n'est pas reconnue (puce USB inconnue), force l'examen d'un port : `chili check-port COM4`.

`check-port` ne redémarre pas une carte qui a déjà le firmware Chili. Sur une carte sans firmware, il la redémarre pour lire le modèle de puce.

### `chili flash`

Installe le firmware Chili (inclus dans `chili.exe`) sur la carte. À faire **une fois par carte**, puis après chaque mise à jour de Chili. Dure environ 30 secondes.

```powershell
chili flash
```

```
Installation du firmware Chili 0.1.3 sur COM4.
Ne débranche pas la carte pendant l'opération.

  [##############################] 100 %  firmware.bin

Firmware écrit et vérifié en 28 s. Redémarrage de la carte…
Firmware Chili 0.1.3 opérationnel sur ESP32-D0WD-V3.

Envoie ton premier programme :
  chili push examples\blink.chl COM4
```

- Plusieurs cartes branchées : précise le port, `chili flash COM5`.
- `flash` se fait **uniquement par USB**.
- Si la carte « ne passe pas en mode programmation automatiquement » : maintiens **BOOT**, appuie une fois sur **EN**/**RST**, relâche **BOOT**, relance la commande.

### `chili push`

Compile le programme, l'envoie, l'enregistre sur la carte et le lance. Ensuite, la sortie de la carte s'affiche.

```powershell
chili push blink.chl
```

```
Envoi de blink.chl (111 octets) vers COM4… ok (180 ms)
Programme enregistré sur la carte et lancé. Il redémarrera tout seul à chaque mise sous tension.
Sortie de la carte — Ctrl+C pour quitter (le programme continue de tourner).
```

- **Ctrl+C** ferme l'affichage ; le programme continue sur la carte.
- Le nouveau programme **remplace** l'ancien.
- Si le programme contient une erreur, rien n'est envoyé : l'erreur s'affiche comme avec `chili check`.
- Par WiFi : `chili push blink.chl 192.168.1.42` (voir [WiFi](wifi.md)).
- Un `.chlb` produit par `chili build` s'envoie de la même façon.

### `chili monitor`

Affiche la sortie de la carte (les `print`) sans rien envoyer. **Ctrl+C** pour quitter.

```powershell
chili monitor
```

```
Sortie de la carte (COM4) — Ctrl+C pour quitter.

1
2
3
```

Messages de Chili qui peuvent apparaître :

| Message                              | Signification                                   |
|--------------------------------------|-------------------------------------------------|
| `[chili] la carte a redémarré`       | la carte vient de démarrer (bouton EN, coupure) |
| `[chili] programme terminé`          | `main` s'est terminée normalement               |
| `[chili] erreur : division par zero (fonction f0, pc 0008)` | erreur d'exécution : le programme s'arrête |
| `[chili] WiFi connecté : ip=192.168.1.42 host=chili-a1b2c3.local` | la carte a rejoint le WiFi |

Par WiFi : `chili monitor 192.168.1.42`.

### `chili stop`

Met le programme **en pause** et éteint ses sorties (LED, relais…). Le programme reste sur la carte et **repart au prochain démarrage** (débranchement, bouton EN), ou avec `chili start`.

```powershell
chili stop
```

```
COM4 : programme en pause, sorties éteintes. Il repartira au prochain démarrage de la carte (ou avec « chili start »).
```

### `chili kill`

Arrête le programme **durablement** : il ne se relance plus au démarrage, jusqu'au prochain `chili start` ou `chili push`. Le programme reste stocké.

```powershell
chili kill
```

```
COM4 : programme arrêté, sorties éteintes. Il ne se relancera plus au démarrage : « chili start » pour le relancer.
```

### `chili start`

Relance depuis le début le programme stocké sur la carte, et réactive son lancement au démarrage.

```powershell
chili start
```

```
COM4 : programme relancé (il se relancera aussi à chaque démarrage)
```

### `chili erase`

Supprime le programme stocké. La carte reste avec le firmware Chili, prête pour un nouveau `push`.

```powershell
chili erase
```

```
COM4 : programme effacé
```

### `chili wifi`

Enregistre le réseau WiFi sur la carte, qui s'y connecte (et s'y reconnecte à chaque démarrage). À faire **par USB**.

```powershell
chili wifi MaBox motdepasse
```

```
COM4 : identifiants enregistrés, connexion à « MaBox »… ok
  ip=192.168.1.42  host=chili-a1b2c3.local

Tu peux maintenant envoyer sans câble :
  chili push programme.chl 192.168.1.42
```

Couper le WiFi et oublier le réseau :

```powershell
chili wifi off
```

```
COM4 : WiFi désactivé.
```

Détails, réseau avec espaces, 2,4 GHz : voir [WiFi](wifi.md).

### `chili version`

```powershell
chili version
```

```
chili 0.1.0 (firmware ESP32 embarqué : 0.1.3, bytecode v1)
```

- `chili 0.1.0` : version de l'outil ;
- `firmware ESP32 embarqué : 0.1.3` : version du firmware que `chili flash` installe ;
- `bytecode v1` : version du format de programme.

### `chili help`

Affiche la liste des commandes avec leurs arguments. `chili` tout seul fait la même chose.

---

## Pour les développeurs

`chili tokens fichier.chl` et `chili parse fichier.chl` affichent les étapes internes du compilateur (jetons du lexer, arbre syntaxique).

---

Suite : [WiFi](wifi.md)
