# Protocole série Chili — v1

Liaison entre `chili.exe` (PC) et le firmware Chili (ESP32) sur le port USB série.

- **115 200 bauds, 8N1**. À l'ouverture, `chili` force DTR = RTS = 0 pour ne pas
  redémarrer la carte.
- La sortie du programme (`print`) et les messages de démarrage arrivent comme du
  texte ordinaire, ligne par ligne.
- Les lignes de **contrôle** commencent par `!chili ` dans les deux sens. Le PC
  ignore toute autre ligne en attendant une réponse (mais `chili monitor` les affiche).
- Les champs d'une réponse sont des paires `clé=valeur` séparées par des espaces
  (pas d'espace dans les valeurs).

## Commandes

| Requête (PC → carte)            | Réponse (carte → PC)                                   |
|---------------------------------|--------------------------------------------------------|
| `!chili info`                   | `!chili info version=… chip=… heap=… program=… state=…` |
| `!chili load <taille> <crc32>`  | `!chili ready`, puis le PC envoie `<taille>` octets bruts ; enfin `!chili ok` ou `!chili err <message>` |
| `!chili start`                  | `!chili ok` (relance le programme stocké et réactive son lancement au démarrage) |
| `!chili stop`                   | `!chili ok` (pause : arrête le programme et remet ses sorties au repos ; il repart au prochain démarrage) |
| `!chili kill`                   | `!chili ok` (comme `stop`, et désactive en plus le lancement au démarrage jusqu'au prochain `start` ou `load`) |
| `!chili erase`                  | `!chili ok` (supprime le programme stocké)             |

Champs de `info` :

| Champ      | Valeurs                                                        |
|------------|----------------------------------------------------------------|
| `version`  | version du firmware, ex. `0.1.0`                               |
| `bytecode` | version du format bytecode supportée (`1`)                     |
| `chip`     | modèle, ex. `ESP32`, `ESP32-S3`                                |
| `heap`     | mémoire libre en octets                                        |
| `program`  | `none` ou `<taille>:<id>` du `.chlb` stocké ; `id` = CRC-32 du contenu (les 4 derniers octets du fichier). Le CRC du fichier entier ne sert pas d'identifiant : il vaut toujours `2144df1c`. |
| `state`    | `running`, `stopped` (stop/kill), `done` (programme terminé), `error` ou `empty` |
| `autorun`  | `1` si le programme est lancé au démarrage, `0` après un `kill` |

Événements spontanés (carte → PC) :

- `!chili boot version=…` au démarrage du firmware ;
- `!chili error <message>` quand le programme s'arrête sur une erreur d'exécution ;
- `!chili halt` quand le programme se termine normalement.

## Chargement

`<crc32>` est le CRC-32 IEEE du fichier `.chlb` complet, en hexadécimal. La carte
vérifie le fichier (`docs/bytecode.md` §4) avant de l'écrire en flash ; le programme
stocké est lancé automatiquement à chaque démarrage.
