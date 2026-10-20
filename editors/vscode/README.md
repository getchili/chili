# Chili pour VS Code

Pour les fichiers `.chl` :

- coloration syntaxique (classes, `function`, `this`, natives…) ;
- erreurs soulignées en rouge pendant la frappe (`chili check`, il faut `chili` dans le PATH ou le réglage `chili.path`) ;
- snippets : `main`, `class`, `abstract`, `extends`, `function`, `if`, `ifelse`, `while`, `for` ;
- commentaires (`Ctrl+/`), accolades auto, icône piment.

## Installation / mise à jour

```powershell
cd D:\Dev\Chili\chili\editors\vscode
npx --yes @vscode/vsce package --skip-license --allow-missing-repository
code --install-extension chili-lang-0.2.0.vsix --force
```

Puis `Ctrl+Shift+P` → « Developer: Reload Window ».

## Palette « Nuit froide »

Bleus, violets, verts et cyan désaturés, pensés pour Default Dark Modern. Aucun rouge : le rouge est réservé aux erreurs.

Les couleurs sont dans `chili-colors.json`. Elles ne touchent que les fichiers Chili. Une extension ne peut pas imposer ses couleurs, il faut donc fusionner ce fichier dans les réglages utilisateur de VS Code (`%APPDATA%\Code\User\settings.json`) : remplacer les règles dont le nom commence par « Chili · ».
La règle `material-icon-theme.files.associations` affiche l'icône piment avec Material Icon Theme. Si la version de l'extension change, mets à jour le chemin.
