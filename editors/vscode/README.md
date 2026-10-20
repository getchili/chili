# Chili pour VS Code

Coloration syntaxique, commentaires (`Ctrl+/`), accolades auto et icône pour les fichiers `.chl`.

## Installation / mise à jour

```powershell
cd D:\Dev\Chili\editors\vscode
npx --yes @vscode/vsce package --skip-license --allow-missing-repository
code --install-extension chili-lang-0.1.0.vsix
```

Puis `Ctrl+Shift+P` → « Developer: Reload Window ».

## Palette « Salsa »

Les couleurs sont dans `chili-colors.json`. Elles ne touchent que les fichiers Chili. Une extension ne peut pas imposer ses couleurs, il faut donc fusionner ce fichier dans les réglages utilisateur de VS Code (`%APPDATA%\Code\User\settings.json`).
La règle `material-icon-theme.files.associations` affiche l'icône piment avec Material Icon Theme. Si la version de l'extension change, mets à jour le chemin.
