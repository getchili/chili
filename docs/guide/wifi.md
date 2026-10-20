# Envoyer ses programmes par WiFi

Une fois la carte connectée à ton WiFi, tu peux lui envoyer des programmes et lire sa sortie **sans câble USB** : la carte peut être alimentée par n'importe quel chargeur.

## Avant de commencer

- L'ESP32 ne fonctionne qu'en **WiFi 2,4 GHz**. Il ne voit pas les réseaux **5 GHz**. La plupart des box émettent les deux : utilise le nom du réseau 2,4 GHz (souvent le même nom, ou un nom qui se termine par `_2G` / `-2.4`).
- Le PC et la carte doivent être sur le **même réseau**. Les réseaux « invités » isolent souvent les appareils entre eux : évite-les.
- Le firmware Chili doit être installé (`chili flash`).

## 1. Connecter la carte (par USB, une seule fois)

1. Branche la carte en USB.
2. Tape (remplace par ton réseau et ton mot de passe) :

   ```powershell
   chili wifi MaBox motdepasse
   ```

3. Si le nom ou le mot de passe contient des espaces ou des caractères spéciaux (`$`, `&`, `;`…), mets-les entre **apostrophes** :

   ```powershell
   chili wifi 'Ma Box Maison' 'mon$ecret 2024'
   ```

4. Après quelques secondes :

   ```
   COM4 : identifiants enregistrés, connexion à « MaBox »… ok
     ip=192.168.1.42  host=chili-a1b2c3.local

   Tu peux maintenant envoyer sans câble :
     chili push programme.chl 192.168.1.42
   ```

Note l'**adresse IP** (`192.168.1.42`) et le **nom réseau** (`chili-a1b2c3.local`). Le nom réseau est propre à chaque carte (il vient de son adresse MAC) et ne change jamais.

La carte retient le réseau : elle s'y reconnecte toute seule à chaque démarrage.

Si tu vois `pas encore connecté.`, vérifie le nom et le mot de passe (attention aux majuscules) et que le réseau est en 2,4 GHz, puis regarde l'état avec `chili check-port`.

## 2. Envoyer un programme par WiFi

Débranche le câble si tu veux (alimente la carte autrement), puis :

```powershell
chili push blink.chl 192.168.1.42
```

ou avec le nom réseau :

```powershell
chili push blink.chl chili-a1b2c3.local
```

```
Envoi de blink.chl (188 octets) vers 192.168.1.42… ok (95 ms)
Programme enregistré sur la carte et lancé. Il redémarrera tout seul à chaque mise sous tension.
Sortie de la carte — Ctrl+C pour quitter (le programme continue de tourner).
```

Toutes les commandes de carte acceptent l'adresse IP ou le nom réseau comme cible :

```powershell
chili monitor chili-a1b2c3.local
```

```powershell
chili stop 192.168.1.42
```

(`start`, `kill`, `erase` aussi.) Seuls `chili flash` et `chili wifi` ont besoin du câble USB.

### IP ou nom réseau ?

- L'**IP** marche partout, mais la box peut en attribuer une autre après un redémarrage.
- Le **nom** `chili-xxxxxx.local` ne change pas. Si Windows ne le trouve pas (`connexion impossible`), utilise l'IP.

## Retrouver l'adresse de la carte

Branche la carte en USB et lance :

```powershell
chili check-port
```

```
  WiFi           : connecté, 192.168.1.42 (chili-a1b2c3.local)
```

## Changer de réseau ou couper le WiFi

- Autre réseau : relance `chili wifi AutreBox autremotdepasse` (par USB).
- Couper le WiFi et oublier le réseau :

  ```powershell
  chili wifi off
  ```

## Bon à savoir

- La carte écoute sur le **port TCP 7070**.
- Une seule connexion WiFi à la fois : si un deuxième PC se connecte, le premier est déconnecté.
- La sortie (`print`) est envoyée à la fois sur l'USB et sur la connexion WiFi.

---

Suite : [Dépannage](depannage.md)
