package device

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"go.bug.st/serial"
)

// Client minimal du bootloader ROM des ESP32 (même protocole qu'esptool) :
// paquets encadrés en SLIP, commandes SYNC et READ_REG.

const (
	cmdSync    = 0x08
	cmdReadReg = 0x0A

	slipEnd    = 0xC0
	slipEsc    = 0xDB
	slipEscEnd = 0xDC
	slipEscEsc = 0xDD

	chipMagicReg = 0x40001000
)

// ErrNoBootloader : la carte n'a pas répondu au bootloader.
var ErrNoBootloader = errors.New("pas de réponse du bootloader")

// Chip décrit une puce ESP32 identifiée par son bootloader.
type Chip struct {
	Name string
	MAC  string // "" si non lue
}

type chipDef struct {
	name   string
	macReg uint32 // registre eFuse de la MAC (mot bas, puis mot haut à +4)
}

// Valeurs du registre « magic » de chaque famille (cf. esptool).
var chipMagics = map[uint32]chipDef{
	0x00F01D83: {"ESP32", 0x3FF5A004},
	0x000007C6: {"ESP32-S2", 0x3F41A044},
	0x00000009: {"ESP32-S3", 0x60007044},
	0x6921506F: {"ESP32-C3", 0x60008844},
	0x1B31506F: {"ESP32-C3", 0x60008844},
	0x4881606F: {"ESP32-C3", 0x60008844},
	0x4361606F: {"ESP32-C3", 0x60008844},
	0x6F51306F: {"ESP32-C2", 0x60008844},
	0x7C41A06F: {"ESP32-C2", 0x60008844},
	0x2CE0806F: {"ESP32-C6", 0x600B0844},
	0xD7B73E80: {"ESP32-H2", 0x600B0844},
	0xFFF0C101: {"ESP8266", 0},
}

type bootloader struct {
	port     serial.Port
	buf      []byte // octets reçus pas encore décodés
	lastBody []byte // données de la dernière réponse (après l'en-tête)
}

// ProbeBootloader redémarre la carte en mode bootloader, identifie la puce,
// puis la redémarre normalement. nativeUSB indique un port USB natif
// Espressif, qui demande une autre séquence de reset.
func ProbeBootloader(portName string, nativeUSB bool) (*Chip, error) {
	bl, err := openBootloader(portName, nativeUSB)
	if err != nil {
		return nil, err
	}
	defer bl.port.Close()
	defer hardReset(bl.port)
	return bl.identify()
}

// openBootloader ouvre le port et fait passer la carte en mode bootloader.
func openBootloader(portName string, nativeUSB bool) (*bootloader, error) {
	p, err := openSerial(portName, 115200)
	if err != nil {
		return nil, err
	}
	bl := &bootloader{port: p}
	for attempt := 0; attempt < 3; attempt++ {
		enterBootloader(p, nativeUSB, attempt)
		p.ResetInputBuffer()
		if bl.sync() == nil {
			return bl, nil
		}
	}
	hardReset(p)
	p.Close()
	return nil, ErrNoBootloader
}

// enterBootloader tire GPIO0 à 0 pendant un reset (circuit auto-reset des
// cartes de développement : DTR → GPIO0, RTS → EN).
func enterBootloader(p serial.Port, nativeUSB bool, attempt int) {
	if nativeUSB {
		p.SetRTS(false)
		p.SetDTR(false)
		time.Sleep(100 * time.Millisecond)
		p.SetDTR(true)
		p.SetRTS(false)
		time.Sleep(100 * time.Millisecond)
		p.SetRTS(true)
		p.SetDTR(false)
		p.SetRTS(true)
		time.Sleep(100 * time.Millisecond)
		p.SetDTR(false)
		p.SetRTS(false)
		return
	}
	// Certaines cartes ont besoin d'un délai plus long : on l'allonge à chaque essai.
	p.SetDTR(false)
	p.SetRTS(true)
	time.Sleep(100 * time.Millisecond)
	p.SetDTR(true)
	p.SetRTS(false)
	time.Sleep(time.Duration(50+200*attempt) * time.Millisecond)
	p.SetDTR(false)
}

func hardReset(p serial.Port) {
	p.SetDTR(false)
	p.SetRTS(true)
	time.Sleep(100 * time.Millisecond)
	p.SetRTS(false)
}

func (bl *bootloader) sync() error {
	payload := append([]byte{0x07, 0x07, 0x12, 0x20}, bytes.Repeat([]byte{0x55}, 32)...)
	for i := 0; i < 5; i++ {
		if _, err := bl.command(cmdSync, payload, 100*time.Millisecond); err == nil {
			// Le ROM envoie plusieurs réponses au SYNC : on les vide.
			time.Sleep(50 * time.Millisecond)
			bl.port.ResetInputBuffer()
			bl.buf = nil
			return nil
		}
	}
	return ErrNoBootloader
}

func (bl *bootloader) readReg(addr uint32) (uint32, error) {
	return bl.command(cmdReadReg, binary.LittleEndian.AppendUint32(nil, addr), time.Second)
}

func (bl *bootloader) identify() (*Chip, error) {
	magic, err := bl.readReg(chipMagicReg)
	if err != nil {
		return nil, err
	}
	def, ok := chipMagics[magic]
	if !ok {
		return &Chip{Name: fmt.Sprintf("puce Espressif inconnue (magic %08x)", magic)}, nil
	}
	chip := &Chip{Name: def.name}
	if def.macReg != 0 {
		lo, err1 := bl.readReg(def.macReg)
		hi, err2 := bl.readReg(def.macReg + 4)
		if err1 == nil && err2 == nil {
			chip.MAC = fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x",
				byte(hi>>8), byte(hi), byte(lo>>24), byte(lo>>16), byte(lo>>8), byte(lo))
		}
	}
	return chip, nil
}

// command envoie une commande et attend la réponse correspondante.
// Elle renvoie le champ « value » de la réponse.
func (bl *bootloader) command(op byte, data []byte, timeout time.Duration) (uint32, error) {
	return bl.commandChk(op, data, 0, timeout)
}

// commandChk est command avec une somme de contrôle (utilisée par FLASH_DATA).
func (bl *bootloader) commandChk(op byte, data []byte, chk uint32, timeout time.Duration) (uint32, error) {
	pkt := []byte{0x00, op}
	pkt = binary.LittleEndian.AppendUint16(pkt, uint16(len(data)))
	pkt = binary.LittleEndian.AppendUint32(pkt, chk)
	pkt = append(pkt, data...)
	if _, err := bl.port.Write(slipEncode(pkt)); err != nil {
		return 0, err
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		frame, err := bl.readFrame(time.Until(deadline))
		if err != nil {
			return 0, err
		}
		// Réponse : 0x01, op, taille u16, valeur u32, données (dont octets de statut).
		if len(frame) < 8 || frame[0] != 0x01 || frame[1] != op {
			continue
		}
		value := binary.LittleEndian.Uint32(frame[4:])
		body := frame[8:]
		bl.lastBody = body
		// Octets de statut en fin de réponse : 4 pour le ROM des ESP32 (2 pour l'ESP8266).
		n := 4
		if len(body) < 4 {
			n = 2
		}
		if len(body) >= n && body[len(body)-n] != 0 {
			return 0, fmt.Errorf("commande 0x%02x refusée (erreur 0x%02x)", op, body[len(body)-n+1])
		}
		return value, nil
	}
	return 0, ErrNoBootloader
}

// readFrame lit la prochaine trame SLIP complète.
func (bl *bootloader) readFrame(timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	chunk := make([]byte, 256)
	for {
		// Cherche une trame complète C0 … C0 dans le tampon.
		if start := bytes.IndexByte(bl.buf, slipEnd); start >= 0 {
			if end := bytes.IndexByte(bl.buf[start+1:], slipEnd); end >= 0 {
				raw := bl.buf[start+1 : start+1+end]
				bl.buf = bl.buf[start+1+end:] // garde le C0 final, il peut ouvrir la suivante
				if len(raw) == 0 {
					continue
				}
				return slipDecode(raw), nil
			}
		}
		left := time.Until(deadline)
		if left <= 0 {
			return nil, ErrNoBootloader
		}
		bl.port.SetReadTimeout(left)
		n, err := bl.port.Read(chunk)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, ErrNoBootloader
		}
		bl.buf = append(bl.buf, chunk[:n]...)
	}
}

func slipEncode(pkt []byte) []byte {
	out := []byte{slipEnd}
	for _, b := range pkt {
		switch b {
		case slipEnd:
			out = append(out, slipEsc, slipEscEnd)
		case slipEsc:
			out = append(out, slipEsc, slipEscEsc)
		default:
			out = append(out, b)
		}
	}
	return append(out, slipEnd)
}

func slipDecode(raw []byte) []byte {
	out := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] == slipEsc && i+1 < len(raw) {
			i++
			switch raw[i] {
			case slipEscEnd:
				out = append(out, slipEnd)
			case slipEscEsc:
				out = append(out, slipEsc)
			default:
				out = append(out, raw[i])
			}
			continue
		}
		out = append(out, raw[i])
	}
	return out
}
