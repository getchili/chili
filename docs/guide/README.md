# Guide de Chili

Chili est un petit langage pour programmer une carte **ESP32**. Tu écris ton programme sur ton PC, tu l'envoies à la carte en une commande (par USB ou WiFi), et il tourne. Pas besoin de reflasher la carte à chaque modification.

## Par où commencer

Suis les pages dans l'ordre la première fois :

1. [Installation](installation.md) — installer Chili sur Windows (2 minutes).
2. [Premiers pas](premiers-pas.md) — brancher la carte et faire clignoter sa LED.
3. [Le langage](langage.md) — variables, conditions, boucles, fonctions.
4. [Textes, tableaux et `for`](textes-tableaux.md) — `"bonjour"`, `[1, 2, 3]`, `for v in t`.
5. [Classes](classes.md) — regrouper données et fonctions en objets (`class`, `new`, `extends`).
6. [Fonctions natives](natives.md) — `print`, `delay`, `millis`, `random`, broches GPIO.
7. [Commandes](commandes.md) — toutes les commandes de `chili`, avec leurs sorties.
8. [WiFi](wifi.md) — envoyer tes programmes sans câble.
9. [Dépannage](depannage.md) — quand quelque chose ne marche pas.
10. [Chili Lab](chili-lab.md) — l'application de bureau, tout à la souris.

## Ce qu'il te faut

- Un PC sous **Windows 10 ou 11** (64 bits).
- Une carte **ESP32** (modèle classique, par exemple une « ESP32 DevKit »).
- Un **câble USB de données** (pas un câble qui ne fait que charger).

## En résumé

```powershell
chili check-port               # détecte la carte
chili flash                    # installe le firmware Chili (une seule fois)
chili push examples\blink.chl  # envoie et lance un programme
```

Tu préfères la souris ? Installe [Chili Lab](chili-lab.md), l'application de bureau : détection de la carte, envoi des programmes, console et firmware, avec des boutons.
