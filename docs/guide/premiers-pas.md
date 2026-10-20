# Premiers pas : faire clignoter la LED

But : brancher l'ESP32, installer le firmware Chili dessus, puis envoyer un premier programme qui fait clignoter la LED bleue de la carte. Compte 5 minutes.

Prérequis : Chili est installé ([Installation](installation.md)) et `chili version` répond.

> Cette page utilise le terminal. Tout ce qui suit se fait aussi à la souris avec l'application [Chili Lab](chili-lab.md#premier-programme-avec-chili-lab) : elle détecte la carte, propose **Installer Chili sur la carte**, puis **Envoyer** à côté de `blink.chl`.

## 1. Brancher la carte

1. Branche l'ESP32 sur le PC avec un **câble USB de données**.
   Beaucoup de câbles livrés avec des écouteurs ou des batteries ne font que **charger** : la carte s'allume mais le PC ne la voit pas. Si rien n'est détecté plus bas, essaie d'abord un autre câble.
2. Une petite LED rouge (alimentation) s'allume sur la carte.

## 2. Installer le pilote USB (si besoin)

La carte communique avec le PC par une petite puce USB-série. Windows 10/11 installe souvent son pilote tout seul. Sinon :

| Puce écrite sur la carte (près du port USB) | Pilote à installer                                              |
|---------------------------------------------|-----------------------------------------------------------------|
| **CH340** ou CH9102                          | « CH341SER » sur le site de WCH : <https://www.wch-ic.com/downloads/CH341SER_EXE.html> |
| **CP2102** ou CP2104 (CP210x)                | « CP210x VCP » sur le site de Silicon Labs : <https://www.silabs.com/developers/usb-to-uart-bridge-vcp-drivers> |

Après l'installation du pilote, débranche et rebranche la carte.

## 3. Détecter la carte

Dans un terminal PowerShell :

```powershell
chili check-port
```

Sur une carte neuve (sans firmware Chili), tu obtiens par exemple :

```
Ports série :
  COM4     WCH CH340 (1A86:7523)  <- carte ESP32 probable

COM4 :
  Firmware Chili : absent
  Puce           : ESP32  (MAC 08:3a:f2:12:34:56)

  La carte est prête à recevoir le firmware Chili :
    chili flash COM4
```

Note le nom du port (`COM4` ici, ce sera peut-être `COM3`, `COM5`… chez toi). Tu n'en as en général pas besoin : quand une seule carte est branchée, `chili` la trouve seul.

Si la sortie est différente, va voir [Dépannage](depannage.md).

## 4. Installer le firmware Chili

Le firmware est le petit programme qui reste sur la carte et qui exécute tes programmes Chili. On l'installe **une seule fois par carte** (et à chaque mise à jour de Chili).

```powershell
chili flash
```

```
Installation du firmware Chili 0.2.0 sur COM4.
Ne débranche pas la carte pendant l'opération.

  [##############################] 100 %  firmware.bin

Firmware écrit et vérifié en 28 s. Redémarrage de la carte…
Firmware Chili 0.2.0 opérationnel sur ESP32-D0WD-V3.

Envoie ton premier programme :
  chili push examples\blink.chl COM4
```

> Ta carte a déjà un firmware Chili plus ancien (avant 0.2.0) ? Relance aussi `chili flash` : les programmes écrits avec la syntaxe actuelle (`class Main`, `function`) ne tournent pas sur un ancien firmware.

Si le message dit que la carte « ne passe pas en mode programmation automatiquement » :

1. maintiens le bouton **BOOT** de la carte enfoncé ;
2. appuie une fois sur **EN** (parfois marqué **RST**) ;
3. relâche **BOOT** ;
4. relance `chili flash`.

## 5. Récupérer les exemples

Les exemples sont installés avec Chili. Copie-les dans tes Documents pour pouvoir les modifier :

```powershell
Copy-Item "$env:LOCALAPPDATA\Programs\Chili\examples" "$HOME\Documents\chili" -Recurse
```

```powershell
cd "$HOME\Documents\chili"
```

## 6. Envoyer ton premier programme

```powershell
chili push blink.chl
```

```
Envoi de blink.chl (188 octets) vers COM4… ok (180 ms)
Programme enregistré sur la carte et lancé. Il redémarrera tout seul à chaque mise sous tension.
Sortie de la carte — Ctrl+C pour quitter (le programme continue de tourner).
```

La LED bleue de la carte clignote. 🎉

- **Ctrl+C** ferme l'affichage, mais le programme continue de tourner sur la carte.
- Le programme est enregistré dans la carte : débranche-la, rebranche-la (même sur un simple chargeur USB), il repart tout seul.
- Pour l'arrêter : `chili stop` (pause) ou `chili kill` (arrêt durable). Voir [Commandes](commandes.md).

> Si ta carte n'a pas de LED sur la broche 2, branche une LED + une résistance de 220 Ω entre la broche **GPIO 2** et **GND**.

## 7. Modifier le programme

1. Ouvre `blink.chl` dans un éditeur (VS Code, ou le Bloc-notes).
2. Change `let period = 500` en `let period = 100`.
3. Enregistre, puis relance :

   ```powershell
   chili push blink.chl
   ```

La LED clignote plus vite. C'est tout : pas de recompilation du firmware, l'envoi prend moins d'une seconde.

## Les autres exemples

| Fichier        | Ce qu'il fait                                                     |
|----------------|-------------------------------------------------------------------|
| `blink.chl`    | fait clignoter la LED (GPIO 2)                                    |
| `bouton.chl`   | allume la LED tant qu'on appuie sur le bouton **BOOT** (GPIO 0)   |
| `compteur.chl` | compte les appuis sur BOOT et affiche le total avec `print`       |
| `sos.chl`      | « SOS » en morse sur la LED, avec des fonctions                   |
| `de.chl`       | dé électronique : un appui sur BOOT, la LED clignote de 1 à 6 fois (`random`) |
| `motifs.chl`   | plusieurs motifs de clignotement tirés au hasard, écrits avec des [classes](classes.md) |
| `fib.chl`      | calcule la suite de Fibonacci et mesure le temps de calcul        |

Essaie par exemple :

```powershell
chili push compteur.chl
```

puis appuie plusieurs fois sur le bouton BOOT : `1`, `2`, `3`… s'affichent dans le terminal.

---

Suite : [Le langage](langage.md)
