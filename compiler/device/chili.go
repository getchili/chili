package device

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"
)

// Dialogue avec le firmware Chili (cf. docs/protocol.md), par USB série ou
// par WiFi (TCP) : le protocole est le même.
//
// Chaque ligne reçue est diffusée à tous les abonnés (Subscribe) : la console
// d'affichage et les commandes en attente de réponse écoutent en parallèle.

const (
	ctrlPrefix = "!chili "
	TCPPort    = 7070
)

// ErrNoFirmware : aucune réponse du firmware Chili.
var ErrNoFirmware = errors.New("pas de firmware Chili")

// Info est la réponse à `!chili info`.
type Info map[string]string

// Conn est une connexion ouverte avec une carte.
type Conn struct {
	rw io.ReadWriteCloser

	mu   sync.Mutex
	subs map[chan string]struct{}
	done chan struct{} // fermé quand la lecture s'arrête
	err  error         // cause de l'arrêt (valide après done)

	cmdMu sync.Mutex // une seule commande à la fois
}

// Open ouvre un port série sans redémarrer la carte (DTR = RTS = 0).
func Open(portName string) (*Conn, error) {
	p, err := openSerial(portName, 115200)
	if err != nil {
		return nil, err
	}
	p.ResetInputBuffer()
	return newConn(p), nil
}

// Dial se connecte à une carte par WiFi (adresse IP ou nom, ex. chili-a1b2c3.local).
func Dial(host string) (*Conn, error) {
	addr := host
	if _, _, err := net.SplitHostPort(host); err != nil {
		addr = net.JoinHostPort(host, fmt.Sprint(TCPPort))
	}
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	return newConn(c), nil
}

func newConn(rw io.ReadWriteCloser) *Conn {
	c := &Conn{rw: rw, subs: map[chan string]struct{}{}, done: make(chan struct{})}
	go c.readLoop()
	return c
}

// Close ferme la connexion et attend la fin de la lecture en cours : sous
// Windows, le port n'est vraiment libéré qu'à ce moment-là.
func (c *Conn) Close() error {
	err := c.rw.Close()
	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
	}
	return err
}

// openSerial ouvre un port sans toucher DTR/RTS. Si le port est encore pris
// (par exemple juste après une fermeture), on réessaie un peu.
func openSerial(portName string, baud int) (serial.Port, error) {
	mode := &serial.Mode{
		BaudRate:          baud,
		InitialStatusBits: &serial.ModemOutputBits{DTR: false, RTS: false},
	}
	var err error
	for try := 0; try < 10; try++ {
		var p serial.Port
		if p, err = serial.Open(portName, mode); err == nil {
			return p, nil
		}
		var pe *serial.PortError
		if !errors.As(err, &pe) || pe.Code() != serial.PortBusy {
			return nil, err
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil, err
}

func (c *Conn) readLoop() {
	sc := bufio.NewScanner(c.rw)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		c.mu.Lock()
		for ch := range c.subs {
			select {
			case ch <- line:
			default: // abonné trop lent : la ligne est perdue pour lui seul
			}
		}
		c.mu.Unlock()
	}
	err := sc.Err()
	if err == nil {
		err = errors.New("connexion fermée")
	}
	c.mu.Lock()
	c.err = err
	for ch := range c.subs {
		close(ch)
	}
	c.subs = nil
	c.mu.Unlock()
	close(c.done)
}

// Subscribe renvoie un canal qui reçoit toutes les lignes à partir de
// maintenant ; il est fermé quand la connexion s'arrête. cancel le libère.
func (c *Conn) Subscribe() (lines <-chan string, cancel func()) {
	ch := make(chan string, 1024)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.subs == nil { // connexion déjà terminée
		close(ch)
		return ch, func() {}
	}
	c.subs[ch] = struct{}{}
	return ch, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if _, ok := c.subs[ch]; ok {
			delete(c.subs, ch)
			close(ch)
		}
	}
}

// Lines est un abonnement permanent (pour la console).
func (c *Conn) Lines() <-chan string {
	ch, _ := c.Subscribe()
	return ch
}

// Done est fermé quand la connexion s'arrête ; Err en donne alors la cause.
func (c *Conn) Done() <-chan struct{} { return c.done }

// Err renvoie la cause de l'arrêt de la connexion (bloque jusqu'à l'arrêt).
func (c *Conn) Err() error {
	<-c.done
	return c.err
}

// Send envoie une commande de contrôle (sans le préfixe).
func (c *Conn) Send(cmd string) error {
	// Le '\n' initial termine une éventuelle ligne parasite côté carte.
	_, err := c.rw.Write([]byte("\n" + ctrlPrefix + cmd + "\n"))
	return err
}

// await attend sur sub une ligne de contrôle commençant par want (ex. "info").
// Elle renvoie la suite de la ligne ; `!chili err …` devient une erreur.
func (c *Conn) await(sub <-chan string, want string, timeout time.Duration) (string, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case line, ok := <-sub:
			if !ok {
				return "", c.Err()
			}
			rest, found := strings.CutPrefix(line, ctrlPrefix)
			if !found {
				continue
			}
			if rest == want || strings.HasPrefix(rest, want+" ") {
				return strings.TrimSpace(strings.TrimPrefix(rest, want)), nil
			}
			if msg, isErr := strings.CutPrefix(rest, "err "); isErr {
				return "", errors.New(msg)
			}
		case <-timer.C:
			return "", ErrNoFirmware
		}
	}
}

// Await attend une ligne de contrôle (abonnement créé à l'appel : à réserver
// aux événements spontanés, comme `wifi`).
func (c *Conn) Await(want string, timeout time.Duration) (string, error) {
	sub, cancel := c.Subscribe()
	defer cancel()
	return c.await(sub, want, timeout)
}

// request envoie cmd et attend la réponse want (abonnement pris avant l'envoi).
func (c *Conn) request(cmd, want string, timeout time.Duration) (string, error) {
	c.cmdMu.Lock()
	defer c.cmdMu.Unlock()
	sub, cancel := c.Subscribe()
	defer cancel()
	if err := c.Send(cmd); err != nil {
		return "", err
	}
	return c.await(sub, want, timeout)
}

// Info interroge le firmware. La commande est renvoyée plusieurs fois au cas
// où la carte serait en train de démarrer.
func (c *Conn) Info(timeout time.Duration) (Info, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		rest, err := c.request("info", "info", 500*time.Millisecond)
		if err == nil {
			return parseFields(rest), nil
		}
		if !errors.Is(err, ErrNoFirmware) {
			return nil, err
		}
	}
	return nil, ErrNoFirmware
}

// Command envoie une commande simple (start, stop, kill, erase…) et attend `ok`.
func (c *Conn) Command(cmd string) error {
	_, err := c.request(cmd, "ok", 3*time.Second)
	return err
}

// Load envoie un fichier .chlb ; la carte le vérifie, l'enregistre et le lance.
func (c *Conn) Load(data []byte) error {
	c.cmdMu.Lock()
	defer c.cmdMu.Unlock()
	sub, cancel := c.Subscribe()
	defer cancel()

	if err := c.Send(fmt.Sprintf("load %d %08x", len(data), crc32.ChecksumIEEE(data))); err != nil {
		return err
	}
	if _, err := c.await(sub, "ready", 3*time.Second); err != nil {
		return err
	}
	// Par blocs : laisse le temps à la carte de vider son tampon de réception.
	for off := 0; off < len(data); off += 1024 {
		end := min(off+1024, len(data))
		if _, err := c.rw.Write(data[off:end]); err != nil {
			return err
		}
		time.Sleep(5 * time.Millisecond)
	}
	// 115 200 bauds ≈ 11 Ko/s : marge large pour le transfert et l'écriture en flash.
	_, err := c.await(sub, "ok", 5*time.Second+time.Duration(len(data))*time.Millisecond)
	return err
}

// SetWiFi enregistre les identifiants WiFi sur la carte (ssid vide = WiFi coupé).
func (c *Conn) SetWiFi(ssid, password string) error {
	if ssid == "" {
		return c.Command("wifi off")
	}
	enc := func(s string) string {
		if s == "" {
			return "-"
		}
		return hex.EncodeToString([]byte(s))
	}
	return c.Command("wifi " + enc(ssid) + " " + enc(password))
}

func parseFields(s string) Info {
	info := Info{}
	for _, f := range strings.Fields(s) {
		if k, v, ok := strings.Cut(f, "="); ok {
			info[k] = v
		}
	}
	return info
}
