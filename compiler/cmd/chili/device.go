package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/getchili/chili/compiler/device"
	"github.com/getchili/chili/compiler/firmware"
)

// checkPort implémente `chili check-port [PORT]`.
func checkPort(args []string) {
	if len(args) > 1 {
		usageExit()
	}
	ports, err := device.ListPorts()
	if err != nil {
		fmt.Fprintln(os.Stderr, "impossible de lister les ports série :", err)
		os.Exit(1)
	}

	if len(ports) == 0 {
		fmt.Println("Aucun port série détecté.")
		fmt.Println()
		fmt.Println("Vérifie que :")
		fmt.Println("  1. la carte est branchée avec un câble USB de données (certains câbles ne font que la charge) ;")
		fmt.Println("  2. le pilote de la puce USB de la carte est installé (CP210x ou CH340 selon le modèle).")
		os.Exit(1)
	}

	fmt.Println("Ports série :")
	for _, p := range ports {
		fmt.Printf("  %-8s %s\n", p.Name, describePort(p))
	}

	var targets []device.Port
	if len(args) == 1 {
		name := strings.ToUpper(args[0])
		for _, p := range ports {
			if strings.ToUpper(p.Name) == name {
				targets = append(targets, p)
			}
		}
		if len(targets) == 0 {
			fmt.Fprintf(os.Stderr, "\n%s n'existe pas.\n", args[0])
			os.Exit(1)
		}
	} else {
		for _, p := range ports {
			if p.Likely {
				targets = append(targets, p)
			}
		}
		if len(targets) == 0 {
			fmt.Println()
			fmt.Println("Aucune carte ESP32 reconnue. Si ta carte est sur l'un de ces ports, force la détection :")
			fmt.Printf("  chili check-port %s\n", ports[0].Name)
			os.Exit(1)
		}
	}

	for _, p := range targets {
		fmt.Println()
		inspect(p)
	}
}

func describePort(p device.Port) string {
	switch {
	case p.Bridge != "":
		return fmt.Sprintf("%s (%s:%s)  <- carte ESP32 probable", p.Bridge, p.VID, p.PID)
	case p.USB:
		desc := fmt.Sprintf("périphérique USB %s:%s", p.VID, p.PID)
		if p.Product != "" {
			desc += " — " + p.Product
		}
		return desc
	default:
		return "port série (non USB)"
	}
}

// inspect interroge d'abord le firmware Chili (sans reset), puis, faute de
// réponse, le bootloader ROM (ce qui redémarre la carte).
func inspect(p device.Port) {
	fmt.Printf("%s :\n", p.Name)

	conn, err := device.Open(p.Name)
	if err != nil {
		portError(err)
		return
	}
	info, err := conn.Info(2 * time.Second)
	conn.Close()
	if err == nil {
		printFirmware(info)
		return
	}

	fmt.Println("  Firmware Chili : absent")
	chip, err := device.ProbeBootloader(p.Name, strings.HasPrefix(p.VID, "303A"))
	switch {
	case err == nil:
		line := chip.Name
		if chip.MAC != "" {
			line += "  (MAC " + chip.MAC + ")"
		}
		fmt.Println("  Puce           :", line)
		fmt.Println()
		fmt.Println("  La carte est prête à recevoir le firmware Chili :")
		fmt.Printf("    chili flash %s\n", p.Name)
	case errors.Is(err, device.ErrNoBootloader):
		fmt.Println("  Puce           : pas de réponse")
		fmt.Println()
		fmt.Println("  La carte n'a pas pu être passée en mode programmation automatiquement.")
		fmt.Println("  Maintiens le bouton BOOT enfoncé, appuie une fois sur EN (ou RST), relâche BOOT,")
		fmt.Printf("  puis relance : chili check-port %s\n", p.Name)
	default:
		portError(err)
	}
}

func printFirmware(info device.Info) {
	fmt.Printf("  Firmware Chili : %s (bytecode v%s)\n", or(info["version"], "?"), or(info["bytecode"], "?"))
	if v := info["version"]; v != "" && v != firmware.Version() {
		fmt.Printf("                   une autre version (%s) est disponible : chili flash\n", firmware.Version())
	}
	chip := or(info["chip"], "?")
	if heap, err := strconv.Atoi(info["heap"]); err == nil {
		chip += fmt.Sprintf(", %d Ko de mémoire libre", heap/1024)
	}
	fmt.Println("  Puce           :", chip)
	switch info["wifi"] {
	case "connected":
		fmt.Printf("  WiFi           : connecté, %s (%s)\n", info["ip"], info["host"])
	case "connecting":
		fmt.Println("  WiFi           : connexion en cours…")
	default:
		fmt.Println("  WiFi           : non configuré (chili wifi <réseau> <mot de passe>)")
	}

	prog := info["program"]
	if prog == "" || prog == "none" {
		fmt.Println("  Programme      : aucun")
		fmt.Println()
		fmt.Println("  Envoie un programme avec : chili push fichier.chl")
		return
	}
	size, crc, _ := strings.Cut(prog, ":")
	fmt.Printf("  Programme      : %s octets (crc %s)\n", size, crc)
	states := map[string]string{
		"running": "en cours d'exécution",
		"stopped": "en pause (chili stop), repartira au prochain démarrage",
		"error":   "arrêté sur une erreur",
		"done":    "terminé (le programme est arrivé au bout)",
		"empty":   "aucun",
	}
	st := or(states[info["state"]], info["state"])
	if info["state"] == "stopped" && info["autorun"] == "0" {
		st = "arrêté (chili kill), ne se relance pas au démarrage : chili start pour le relancer"
	}
	fmt.Println("  État           :", st)
}

func portError(err error) {
	msg := err.Error()
	if strings.Contains(strings.ToLower(msg), "busy") || strings.Contains(strings.ToLower(msg), "denied") {
		fmt.Println("  Port occupé : ferme les autres programmes qui l'utilisent (Arduino IDE, moniteur série…).")
		return
	}
	fmt.Println("  Erreur :", msg)
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
