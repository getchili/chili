// Package device gère la communication avec les cartes : détection des ports
// série, dialogue avec le bootloader ROM de l'ESP32 et avec le firmware Chili.
package device

import (
	"sort"
	"strings"

	"go.bug.st/serial/enumerator"
)

// Port décrit un port série du PC.
type Port struct {
	Name    string // COM3, /dev/ttyUSB0…
	USB     bool
	VID     string
	PID     string
	Serial  string
	Product string
	Bridge  string // puce USB-série reconnue, "" si inconnue
	Likely  bool   // probablement une carte ESP32
}

// Puces USB-série courantes sur les cartes ESP32, par VID:PID.
var bridges = map[string]string{
	"10C4:EA60": "Silicon Labs CP210x",
	"1A86:7523": "WCH CH340",
	"1A86:55D4": "WCH CH9102",
	"1A86:55D3": "WCH CH343",
	"0403:6001": "FTDI FT232R",
	"0403:6010": "FTDI FT2232",
	"0403:6015": "FTDI FT231X",
	"303A:1001": "USB natif Espressif (ESP32-S3/C3/C6)",
	"303A:0002": "USB natif Espressif (ESP32-S2)",
}

// ListPorts renvoie les ports série du PC, les ESP32 probables en premier.
func ListPorts() ([]Port, error) {
	details, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil, err
	}
	var ports []Port
	for _, d := range details {
		p := Port{
			Name:    d.Name,
			USB:     d.IsUSB,
			VID:     strings.ToUpper(d.VID),
			PID:     strings.ToUpper(d.PID),
			Serial:  d.SerialNumber,
			Product: d.Product,
		}
		if p.USB {
			p.Bridge = bridges[p.VID+":"+p.PID]
			p.Likely = p.Bridge != ""
		}
		ports = append(ports, p)
	}
	sort.SliceStable(ports, func(i, j int) bool { return ports[i].Likely && !ports[j].Likely })
	return ports, nil
}
