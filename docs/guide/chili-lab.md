# Chili Lab : l'application de bureau

Chili Lab est une application Windows pour tout faire **à la souris** : voir l'état de la carte, envoyer un programme, lire sa sortie, installer ou mettre à jour le firmware, configurer le WiFi. Pas de terminal, pas de commande à taper.

Chili Lab est autonome : il contient déjà le compilateur et le firmware. Tu peux l'utiliser avec ou sans `chili.exe`.

## Installer

1. Télécharge **ChiliLabSetup-x.y.z.exe** (par exemple `ChiliLabSetup-0.1.0.exe`) sur la page des versions : <https://github.com/getchili/chili-lab/releases/latest>
2. Double-clique dessus. Si Windows affiche **« Windows a protégé votre ordinateur »** : **Informations complémentaires** → **Exécuter quand même** (voir [Installation](installation.md)).
3. Coche **Créer une icône sur le bureau** si tu veux un raccourci sur le bureau.
4. Clique sur **Installer**. À la fin, laisse cochée **Lancer Chili Lab** puis clique sur **Terminer**.

Pas besoin de droits administrateur.

## Ouvrir

Menu Démarrer → **Chili Lab**, ou double-clic sur l'icône du bureau.

Une seule copie de Chili Lab tourne à la fois : le relancer ouvre simplement une nouvelle fenêtre de la même application.

## La fenêtre

| Zone             | Ce qu'elle fait                                                                    |
|------------------|-------------------------------------------------------------------------------------|
| **Carte**        | détecte la carte toute seule (boutons **Relier** / **Déconnecter**) ; affiche le modèle, la mémoire libre, le firmware, le WiFi |
| **Programme**    | état du programme (en cours, en pause, arrêté, terminé, erreur) et boutons **Lancer**, **Pause**, **Kill**, **Effacer** |
| **Programmes**   | tes fichiers `.chl`, avec **Vérifier** (sans envoyer) et **Envoyer** (voir ci-dessous) |
| **Console**      | la sortie de la carte en direct (`print`, erreurs), avec **Défilement auto** et **Effacer** |
| **Firmware**     | **Installer Chili sur la carte** pour une carte neuve, **Mettre à jour** quand une nouvelle version est incluse |
| **WiFi**         | **Connecter au WiFi** : nom du réseau et mot de passe                              |

Le bas de la barre latérale affiche la version (`Chili Lab 0.1.0`).

### Tes programmes

La liste a deux groupes :

- **Mes programmes** : le dossier `Documents\Chili`. Il est créé au premier lancement, avec une copie des exemples pour démarrer.
  - **Ouvrir** ouvre ce dossier dans l'Explorateur : c'est là que tu crées et modifies tes fichiers `.chl` (avec VS Code ou le Bloc-notes).
  - **Changer** choisit un autre dossier ; Chili Lab s'en souvient.
- **Exemples** : les exemples fournis avec Chili Lab.

Tu peux aussi **glisser-déposer** un fichier `.chl` sur la fenêtre pour l'envoyer à la carte.

Si le programme contient une erreur, rien n'est envoyé : l'erreur s'affiche avec sa ligne (voir les [erreurs fréquentes](langage.md#10-erreurs-fréquentes)).

### Correspondance avec les commandes

| Bouton                                         | Commande `chili` équivalente |
|------------------------------------------------|------------------------------|
| Envoyer                                        | `chili push fichier.chl`     |
| Vérifier                                       | `chili check fichier.chl`    |
| Lancer                                         | `chili start`                |
| Pause                                          | `chili stop`                 |
| Kill                                           | `chili kill`                 |
| Effacer                                        | `chili erase`                |
| Installer Chili sur la carte / Mettre à jour   | `chili flash`                |
| Connecter au WiFi                              | `chili wifi …`               |

## Premier programme avec Chili Lab

1. Branche la carte avec un câble USB de données (pilote : voir [Premiers pas](premiers-pas.md#2-installer-le-pilote-usb-si-besoin)).
2. Ouvre Chili Lab : la carte apparaît toute seule.
3. Carte neuve : clique sur **Installer Chili sur la carte** et attends la fin (environ 30 secondes).
4. Dans **Mes programmes**, clique sur **Envoyer** à côté de `blink.chl` : la LED clignote.
5. Clique sur **Ouvrir**, modifie `blink.chl` (par exemple `let period = 100`), enregistre, puis **Envoyer** à nouveau.

## Bon à savoir

- **Un seul programme à la fois peut utiliser le port USB.** Tant que Chili Lab est relié à la carte, les commandes `chili push`, `chili monitor`… dans un terminal répondent que le port est occupé. Clique sur **Déconnecter** ou ferme Chili Lab avant de les utiliser.
- Fermer Chili Lab ne coupe pas la carte : le programme continue de tourner.
- Il faut Microsoft Edge (présent sur Windows 10/11) ou Google Chrome : Chili Lab s'affiche avec.

---

Retour au [sommaire](README.md)
