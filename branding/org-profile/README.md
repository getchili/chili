<p align="center">
  <img src="https://raw.githubusercontent.com/getchili/.github/main/profile/logo-500.png" width="120" alt="Chili">
</p>

<h1 align="center">Chili</h1>

<p align="center">
  <b>Le langage qui pimente ton ESP32.</b><br>
  Écris ton programme, envoie-le à la carte par USB ou WiFi en une commande.<br>
  Pas d'IDE, pas de reflash, pas d'attente.
</p>

<p align="center">
  <a href="https://getchili.github.io">Site</a> ·
  <a href="https://github.com/getchili/chili/releases/latest">Télécharger</a> ·
  <a href="https://github.com/getchili/chili">Code source</a>
</p>

```chili
fn main() {
    gpio.mode(2, 1)
    while true {
        gpio.write(2, 1)
        delay(500)
        gpio.write(2, 0)
        delay(500)
    }
}
```

```
> chili push blink.chl
Envoi de blink.chl (111 octets) vers COM5… ok (0.2 s)
```
