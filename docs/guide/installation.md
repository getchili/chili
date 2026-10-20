# Installation

Chili s'installe avec un seul fichier, sans droits administrateur. Tout est inclus : le compilateur, le firmware de la carte et les exemples.

## 1. Télécharger

Télécharge **ChiliSetup-x.y.z.exe** (par exemple `ChiliSetup-0.1.0.exe`) depuis la page des versions :
<https://github.com/getchili/chili/releases/latest>

## 2. Lancer l'installateur

1. Double-clique sur `ChiliSetup-0.1.0.exe`.
2. Si Windows affiche **« Windows a protégé votre ordinateur »** (SmartScreen) : clique sur **Informations complémentaires**, puis sur **Exécuter quand même**.
   Ce message apparaît pour tout programme récent qui n'est pas encore signé ; il ne signale pas un virus.
3. Laisse les options cochées :
   - **Ajouter chili au PATH** : indispensable, c'est ce qui permet de taper `chili` dans un terminal ;
   - **Associer les fichiers .chl et .chlb** : donne une icône à tes programmes ;
   - **Installer l'extension Chili pour VS Code** : coloration du code (proposé seulement si VS Code est installé).
4. Clique sur **Installer**, puis **Terminer**.

Chili est installé dans `%LOCALAPPDATA%\Programs\Chili` :

| Dossier      | Contenu                                  |
|--------------|------------------------------------------|
| `bin\`       | `chili.exe`                              |
| `examples\`  | programmes d'exemple (`blink.chl`…)      |

## 3. Vérifier

1. Ouvre un **nouveau** terminal (menu Démarrer → tape `PowerShell` → Entrée). Un terminal déjà ouvert avant l'installation ne connaît pas encore la commande.
2. Tape :

   ```powershell
   chili version
   ```

3. Tu dois voir :

   ```
   chili 0.1.0 (firmware ESP32 embarqué : 0.1.3, bytecode v1)
   ```

Si tu vois `chili : Le terme «chili» n'est pas reconnu…`, ferme le terminal et ouvre-en un nouveau. Si ça ne suffit pas, relance l'installateur en laissant cochée l'option **Ajouter chili au PATH**.

## Sans terminal : Chili Lab

Si tu préfères ne pas utiliser de terminal, installe plutôt (ou en plus) [Chili Lab](chili-lab.md), l'application de bureau. Elle a son propre installateur, `ChiliLabSetup-x.y.z.exe`, et contient tout ce qu'il faut.

## Mettre à jour

Lance simplement l'installateur de la nouvelle version : il remplace l'ancienne. Ensuite, mets aussi à jour le firmware de la carte avec `chili flash` (voir [Commandes](commandes.md#chili-flash)).

## Désinstaller

Paramètres Windows → **Applications** → **Applications installées** → **Chili** → **Désinstaller**.

---

Suite : [Premiers pas](premiers-pas.md)
