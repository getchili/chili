// Package firmware embarque le firmware Chili compilé, pour que `chili flash`
// fonctionne sans aucun autre outil. Les fichiers de esp32/ sont produits par
// build.ps1 (à la racine du dépôt) à partir de runtimes/esp32.
package firmware

import (
	"embed"
	"strings"
)

//go:embed esp32/*.bin esp32/version.txt
var files embed.FS

// Image est un fichier à écrire en flash à une adresse donnée.
type Image struct {
	Name   string
	Offset uint32
	Data   []byte
}

// Disposition de la flash d'un ESP32 classique (Arduino, table de partitions par défaut).
var esp32Layout = []struct {
	file   string
	offset uint32
}{
	{"bootloader.bin", 0x1000},
	{"partitions.bin", 0x8000},
	{"boot_app0.bin", 0xE000},
	{"firmware.bin", 0x10000},
}

// ESP32 renvoie les images du firmware pour l'ESP32 classique.
func ESP32() []Image {
	var imgs []Image
	for _, l := range esp32Layout {
		data, err := files.ReadFile("esp32/" + l.file)
		if err != nil {
			panic("firmware embarqué incomplet : " + err.Error())
		}
		imgs = append(imgs, Image{Name: l.file, Offset: l.offset, Data: data})
	}
	return imgs
}

// Version renvoie la version du firmware embarqué.
func Version() string {
	v, err := files.ReadFile("esp32/version.txt")
	if err != nil {
		return "?"
	}
	return strings.TrimSpace(string(v))
}
