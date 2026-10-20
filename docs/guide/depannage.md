# Dépannage

Premier réflexe dans tous les cas :

```powershell
chili check-port
```

Elle dit ce qui est branché, si le firmware Chili répond, et quoi faire ensuite.

## `chili` n'est pas reconnu

```
chili : Le terme «chili» n'est pas reconnu comme nom d'applet de commande…
```

1. Ferme le terminal et ouvre-en un **nouveau** (le PATH n'est lu qu'à l'ouverture).
2. Si ça ne suffit pas, relance l'installateur en laissant cochée **Ajouter chili au PATH**.

## Aucun port série détecté

```
Aucun port série détecté.
```

Le PC ne voit pas la carte du tout.

1. **Change de câble** : c'est la cause n° 1. Beaucoup de câbles USB ne font que charger. Prends un câble qui sert déjà à transférer des fichiers (téléphone…).
2. Essaie un autre port USB du PC (évite les hubs).
3. Installe le pilote de la puce USB de la carte (écrite près du port USB) :
   - **CH340** / CH9102 : <https://www.wch-ic.com/downloads/CH341SER_EXE.html>
   - **CP2102** / CP210x : <https://www.silabs.com/developers/usb-to-uart-bridge-vcp-drivers>
4. Débranche et rebranche la carte, puis relance `chili check-port`.

Pour vérifier côté Windows : clic droit sur Démarrer → **Gestionnaire de périphériques** → **Ports (COM et LPT)**. Un nouveau port doit apparaître quand tu branches la carte. Un point d'exclamation jaune = pilote manquant.

## Carte non reconnue

```
Aucune carte ESP32 reconnue. Si ta carte est sur l'un de ces ports, force la détection :
  chili check-port COM3
```

Un port existe mais sa puce USB n'est pas dans la liste connue. Si tu sais que c'est ta carte, force :

```powershell
chili check-port COM3
```

puis, si la puce est détectée, `chili flash COM3`.

## Port occupé

```
  Port occupé : ferme les autres programmes qui l'utilisent (Arduino IDE, moniteur série…).
```

Un seul programme à la fois peut utiliser un port série. Ferme :

- Arduino IDE (et son moniteur série), PlatformIO, Thonny, PuTTY… ;
- **Chili Lab** (clique sur **Déconnecter**, ou ferme Chili Lab) ;
- un autre terminal où tourne `chili monitor` ou `chili push` (Ctrl+C pour l'arrêter).

Puis relance la commande.

## « Aucune carte avec le firmware Chili n'a été trouvée en USB »

```
Aucune carte avec le firmware Chili n'a été trouvée en USB.
Lance « chili check-port » pour voir ce qui est branché, ou indique la cible (COM5, adresse IP…).
```

- `check-port` indique `Firmware Chili : absent` : installe-le avec `chili flash`.
- Le port est occupé (Chili Lab ouvert ?) : voir ci-dessus.
- La carte ne répond plus : appuie sur **EN** (ou **RST**) et réessaie.

## La carte ne passe pas en mode programmation (BOOT / EN)

```
La carte ne passe pas en mode programmation automatiquement.
Maintiens le bouton BOOT enfoncé, appuie une fois sur EN (ou RST), relâche BOOT, puis relance la commande.
```

Certaines cartes ne se laissent pas redémarrer en mode programmation par le PC. Fais-le à la main :

1. maintiens **BOOT** enfoncé (parfois marqué **IO0** ou **FLASH**) ;
2. appuie une fois sur **EN** (ou **RST**, **RESET**) et relâche-le ;
3. relâche **BOOT** ;
4. relance `chili flash` (ou `chili check-port`).

Après le flash, appuie sur **EN** pour redémarrer la carte normalement.

## Le firmware est à mettre à jour

Après une mise à jour de Chili, le firmware de la carte peut être plus ancien. Les messages possibles :

```
  Firmware Chili : 0.1.2 (bytecode v1)
                   une autre version (0.1.3) est disponible : chili flash
```

```
COM4 : le firmware de la carte ne connaît pas « kill ». Mets-le à jour avec : chili flash
```

```
COM4 : le firmware gère le bytecode v0, ce compilateur produit la v1.
Mets le firmware à jour avec : chili flash
```

Solution, par USB :

```powershell
chili flash
```

Puis renvoie ton programme avec `chili push`.

## Le WiFi ne se connecte pas

```
pas encore connecté.
Vérifie le nom et le mot de passe du réseau (2,4 GHz uniquement : l'ESP32 ne voit pas le 5 GHz),
puis regarde l'état avec : chili check-port
```

1. **5 GHz** : l'ESP32 ne voit que les réseaux **2,4 GHz**. Dans les réglages de ta box, active le 2,4 GHz ou utilise son nom dédié.
2. Vérifie le nom et le mot de passe : majuscules, espaces. Avec des espaces ou des caractères spéciaux, mets-les entre apostrophes : `chili wifi 'Ma Box' 'mot de passe'`.
3. Rapproche la carte de la box.
4. Relance `chili check-port` : la connexion peut prendre un peu plus de temps (`WiFi : connexion en cours…`).

## `chili push` par WiFi échoue

```
192.168.1.42 : connexion impossible : …
```

1. Vérifie que la carte est allumée et connectée : par USB, `chili check-port` affiche `WiFi : connecté, 192.168.1.42 (…)`. L'IP a peut-être changé.
2. Le PC et la carte doivent être sur le même réseau (pas de réseau « invité »).
3. Avec le nom `chili-xxxxxx.local`, essaie plutôt l'IP.

## Le programme s'arrête sur une erreur

`chili monitor` (ou `chili push`) affiche :

```
[chili] erreur : division par zero (fonction f0, pc 0008)
```

et `chili check-port` indique `État : arrêté sur une erreur`.

Le programme a fait une opération impossible (division par zéro, broche invalide, récursion sans fin…). La carte, elle, fonctionne toujours.

1. Lis le message : il dit quelle opération a échoué (liste dans [Le langage](langage.md#erreurs-dexécution)).
2. Teste sur le PC pour comprendre : `chili run programme.chl`.
3. Corrige, puis `chili push programme.chl`.

## La LED ne clignote pas

1. `chili check-port` doit indiquer `État : en cours d'exécution`. Sinon : `chili start`.
2. Toutes les cartes n'ont pas de LED sur la broche 2. Branche une LED externe (avec une résistance de 220 Ω) entre **GPIO 2** et **GND**, ou change le numéro de broche dans le programme.
3. Vérifie le sens de la LED : patte longue côté broche, patte courte côté GND.

## Rien ne s'affiche dans `chili monitor`

- Le programme n'appelle peut-être pas `print`.
- Le programme est en pause ou arrêté : `chili check-port` pour voir son état, `chili start` pour le relancer.

## La carte redémarre en boucle

Le firmware s'est peut-être mal écrit (débranchement pendant le flash). Relance `chili flash`, au besoin avec la manip BOOT / EN ci-dessus.

## Repartir de zéro

```powershell
chili erase
```

efface le programme. Pour tout réinstaller (firmware compris) :

```powershell
chili flash
```

---

Suite : [Chili Lab](chili-lab.md)
