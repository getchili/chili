package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getchili/chili/compiler/bytecode"
	"github.com/getchili/chili/compiler/device"
	"github.com/getchili/chili/compiler/firmware"
)

// Commandes qui parlent à une carte : push, monitor, start, stop, erase, wifi.
// La cible est un port série (COM5, /dev/ttyUSB0), une adresse IP ou un nom
// réseau (chili-a1b2c3.local). Sans cible, la carte USB est détectée seule.

// boardCommand exécute cmd s'il s'agit d'une commande de carte.
func boardCommand(cmd string, args []string) bool {
	switch cmd {
	case "check-port":
		checkPort(args)
	case "push":
		if len(args) < 1 || len(args) > 2 {
			usageExit()
		}
		push(args[0], optArg(args, 1))
	case "monitor":
		conn, where := connect(optArg(args, 0), len(args) <= 1)
		fmt.Printf("Sortie de la carte (%s) — Ctrl+C pour quitter.\n\n", where)
		monitor(conn, conn.Lines())
	case "start", "kill", "stop", "erase":
		if len(args) > 1 {
			usageExit()
		}
		conn, where := connect(optArg(args, 0), true)
		defer conn.Close()
		if err := conn.Command(cmd); err != nil {
			if strings.Contains(err.Error(), "commande inconnue") {
				die("%s : le firmware de la carte ne connaît pas « %s ». Mets-le à jour avec : chili flash", where, cmd)
			}
			die("%s : %v", where, err)
		}
		fmt.Printf("%s : %s\n", where, map[string]string{
			"start": "programme relancé (il se relancera aussi à chaque démarrage)",
			"stop":  "programme en pause, sorties éteintes. Il repartira au prochain démarrage de la carte (ou avec « chili start »).",
			"kill":  "programme arrêté, sorties éteintes. Il ne se relancera plus au démarrage : « chili start » pour le relancer.",
			"erase": "programme effacé",
		}[cmd])
	case "wifi":
		wifi(args)
	case "flash":
		flash(args)
	default:
		return false
	}
	return true
}

func optArg(args []string, i int) string {
	if i < len(args) {
		return args[i]
	}
	return ""
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func isSerialName(s string) bool {
	u := strings.ToUpper(s)
	return strings.HasPrefix(u, "COM") || strings.HasPrefix(s, "/dev/")
}

// connect ouvre la connexion avec la carte et vérifie que le firmware Chili
// répond. Renvoie aussi une description de la cible pour les messages.
func connect(target string, check bool) (*device.Conn, string) {
	if target != "" {
		var conn *device.Conn
		var err error
		if isSerialName(target) {
			conn, err = device.Open(target)
		} else {
			conn, err = device.Dial(target)
		}
		if err != nil {
			die("%s : connexion impossible : %v", target, err)
		}
		if check {
			if _, err := conn.Info(3 * time.Second); err != nil {
				conn.Close()
				die("%s : le firmware Chili ne répond pas (%v).\nVérifie avec : chili check-port", target, err)
			}
		}
		return conn, target
	}

	ports, err := device.ListPorts()
	if err != nil {
		die("impossible de lister les ports série : %v", err)
	}
	for _, p := range ports {
		if !p.Likely {
			continue
		}
		conn, err := device.Open(p.Name)
		if err != nil {
			continue
		}
		if _, err := conn.Info(2 * time.Second); err == nil {
			return conn, p.Name
		}
		conn.Close()
	}
	die("Aucune carte avec le firmware Chili n'a été trouvée en USB.\n" +
		"Lance « chili check-port » pour voir ce qui est branché, ou indique la cible (COM5, adresse IP…).")
	return nil, ""
}

// push compile (si besoin) et envoie un programme, puis affiche sa sortie.
func push(path, target string) {
	src, err := os.ReadFile(path)
	if err != nil {
		die("%v", err)
	}
	var data []byte
	if strings.HasPrefix(string(src), bytecode.Magic) {
		load(path, src) // vérifie le fichier
		data = src
	} else {
		data = mustCompile(path, src).Encode()
	}

	conn, where := connect(target, false)
	info, err := conn.Info(3 * time.Second)
	if err != nil {
		die("%s : le firmware Chili ne répond pas.\nVérifie avec : chili check-port", where)
	}
	if v := info["bytecode"]; v != "" && v != fmt.Sprint(bytecode.Version) {
		die("%s : le firmware gère le bytecode v%s, ce compilateur produit la v%d.\n"+
			"Mets le firmware à jour avec : chili flash", where, v, bytecode.Version)
	}

	fmt.Printf("Envoi de %s (%d octets) vers %s… ", filepath.Base(path), len(data), where)
	lines := conn.Lines() // abonné avant l'envoi : aucune sortie du programme n'est perdue
	start := time.Now()
	if err := conn.Load(data); err != nil {
		fmt.Println()
		if strings.Contains(err.Error(), "non disponible sur cette carte") || strings.Contains(err.Error(), "version de bytecode") {
			die("échec : %v\nLe firmware de la carte est trop ancien pour ce programme. Mets-le à jour avec : chili flash", err)
		}
		die("échec : %v", err)
	}
	fmt.Printf("ok (%d ms)\n", time.Since(start).Milliseconds())
	fmt.Println("Programme enregistré sur la carte et lancé. Il redémarrera tout seul à chaque mise sous tension.")
	fmt.Println("Sortie de la carte — Ctrl+C pour quitter (le programme continue de tourner).")
	fmt.Println()
	monitor(conn, lines)
}

// monitor affiche la sortie de la carte jusqu'à Ctrl+C ou déconnexion.
func monitor(conn *device.Conn, lines <-chan string) {
	for line := range lines {
		rest, ctrl := strings.CutPrefix(line, "!chili ")
		if !ctrl {
			fmt.Println(line)
			continue
		}
		switch {
		case strings.HasPrefix(rest, "error "):
			fmt.Println("[chili] erreur :", strings.TrimPrefix(rest, "error "))
		case rest == "halt":
			fmt.Println("[chili] programme terminé")
		case strings.HasPrefix(rest, "boot"):
			fmt.Println("[chili] la carte a redémarré")
		case strings.HasPrefix(rest, "wifi "):
			fmt.Println("[chili] WiFi connecté :", strings.TrimPrefix(rest, "wifi "))
		}
	}
	die("connexion perdue : %v", conn.Err())
}

// wifi : `chili wifi <ssid> <mot de passe> [cible]` ou `chili wifi off [cible]`.
func wifi(args []string) {
	var ssid, pass, target string
	switch {
	case len(args) >= 1 && len(args) <= 2 && args[0] == "off":
		target = optArg(args, 1)
	case len(args) >= 2 && len(args) <= 3:
		ssid, pass, target = args[0], args[1], optArg(args, 2)
	default:
		usageExit()
	}

	conn, where := connect(target, true)
	defer conn.Close()
	if err := conn.SetWiFi(ssid, pass); err != nil {
		die("%s : %v", where, err)
	}
	if ssid == "" {
		fmt.Printf("%s : WiFi désactivé.\n", where)
		return
	}
	fmt.Printf("%s : identifiants enregistrés, connexion à « %s »… ", where, ssid)
	if rest, err := conn.Await("wifi", 20*time.Second); err == nil {
		fields := strings.Fields(rest)
		fmt.Println("ok")
		fmt.Println(" ", strings.Join(fields, "  "))
		for _, f := range fields {
			if ip, ok := strings.CutPrefix(f, "ip="); ok {
				fmt.Printf("\nTu peux maintenant envoyer sans câble :\n  chili push programme.chl %s\n", ip)
			}
		}
		return
	} else if !errors.Is(err, device.ErrNoFirmware) {
		die("\n%v", err)
	}
	fmt.Println("pas encore connecté.")
	fmt.Println("Vérifie le nom et le mot de passe du réseau (2,4 GHz uniquement : l'ESP32 ne voit pas le 5 GHz),")
	fmt.Println("puis regarde l'état avec : chili check-port")
}

// flash installe le firmware Chili embarqué : `chili flash [PORT]`.
func flash(args []string) {
	if len(args) > 1 {
		usageExit()
	}
	var port device.Port
	ports, err := device.ListPorts()
	if err != nil {
		die("impossible de lister les ports série : %v", err)
	}
	if len(args) == 1 {
		port = device.Port{Name: args[0]}
		for _, p := range ports {
			if strings.EqualFold(p.Name, args[0]) {
				port = p
			}
		}
	} else {
		var found []device.Port
		for _, p := range ports {
			if p.Likely {
				found = append(found, p)
			}
		}
		switch len(found) {
		case 0:
			die("Aucune carte ESP32 détectée. Lance « chili check-port » pour diagnostiquer.")
		case 1:
			port = found[0]
		default:
			names := make([]string, len(found))
			for i, p := range found {
				names[i] = p.Name
			}
			die("Plusieurs cartes détectées (%s) : précise laquelle, ex. chili flash %s", strings.Join(names, ", "), names[0])
		}
	}

	var images []device.FlashImage
	for _, img := range firmware.ESP32() {
		images = append(images, device.FlashImage{Name: img.Name, Offset: img.Offset, Data: img.Data})
	}
	fmt.Printf("Installation du firmware Chili %s sur %s.\n", firmware.Version(), port.Name)
	fmt.Println("Ne débranche pas la carte pendant l'opération.")
	fmt.Println()

	start := time.Now()
	err = device.Flash(port.Name, strings.HasPrefix(port.VID, "303A"), "ESP32", images,
		func(name string, done, total int) {
			const width = 30
			n := done * width / total
			fmt.Printf("\r  [%s%s] %3d %%  %-16s", strings.Repeat("#", n), strings.Repeat(".", width-n), done*100/total, name)
		})
	fmt.Println()
	if errors.Is(err, device.ErrNoBootloader) {
		die("\nLa carte ne passe pas en mode programmation automatiquement.\n" +
			"Maintiens le bouton BOOT enfoncé, appuie une fois sur EN (ou RST), relâche BOOT, puis relance la commande.")
	}
	if err != nil {
		die("\nÉchec : %v", err)
	}
	fmt.Printf("\nFirmware écrit et vérifié en %.0f s. Redémarrage de la carte…\n", time.Since(start).Seconds())

	conn, err := device.Open(port.Name)
	if err == nil {
		defer conn.Close()
		if info, err := conn.Info(6 * time.Second); err == nil {
			fmt.Printf("Firmware Chili %s opérationnel sur %s.\n\n", info["version"], or(info["chip"], "ESP32"))
			fmt.Println("Envoie ton premier programme :")
			fmt.Printf("  chili push examples\\blink.chl %s\n", port.Name)
			return
		}
	}
	fmt.Println("La carte ne répond pas encore : appuie sur EN (ou RST) puis vérifie avec « chili check-port ».")
}
