package device

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"

	"go.bug.st/serial"
)

// Écriture de la flash par le bootloader ROM (sous-ensemble d'esptool, sans
// « stub ») : SPI_ATTACH, SPI_SET_PARAMS, CHANGE_BAUDRATE, FLASH_BEGIN/DATA,
// puis vérification MD5 de chaque image.

const (
	cmdFlashBegin   = 0x02
	cmdFlashData    = 0x03
	cmdSPISetParams = 0x0B
	cmdSPIAttach    = 0x0D
	cmdChangeBaud   = 0x0F
	cmdSPIFlashMD5  = 0x13

	flashBlockSize = 0x400 // taille de bloc du ROM
	flashBaud      = 460800
	checksumSeed   = 0xEF
)

// FlashImage est un fichier à écrire à une adresse de la flash.
type FlashImage struct {
	Name   string
	Offset uint32
	Data   []byte
}

// FlashProgress est appelée pendant l'écriture (octets écrits / total, toutes images).
type FlashProgress func(image string, done, total int)

// Flash écrit les images sur la carte, vérifie leur MD5 puis redémarre la
// carte. wantChip est le modèle attendu (ex. "ESP32") : une autre puce est refusée.
func Flash(portName string, nativeUSB bool, wantChip string, images []FlashImage, progress FlashProgress) error {
	bl, err := openBootloader(portName, nativeUSB)
	if err != nil {
		return err
	}
	defer bl.port.Close()
	defer hardReset(bl.port)

	chip, err := bl.identify()
	if err != nil {
		return err
	}
	if chip.Name != wantChip {
		return fmt.Errorf("puce %s détectée : ce firmware est prévu pour %s", chip.Name, wantChip)
	}

	if _, err := bl.command(cmdSPIAttach, make([]byte, 8), 3*time.Second); err != nil {
		return fmt.Errorf("SPI_ATTACH : %w", err)
	}
	params := make([]byte, 0, 24)
	for _, v := range []uint32{0, 4 << 20, 64 << 10, 4 << 10, 256, 0xFFFF} {
		params = binary.LittleEndian.AppendUint32(params, v)
	}
	if _, err := bl.command(cmdSPISetParams, params, 3*time.Second); err != nil {
		return fmt.Errorf("SPI_SET_PARAMS : %w", err)
	}

	// Passe en 460 800 bauds : 4 fois plus rapide. En cas d'échec, on reste à 115 200.
	baud := binary.LittleEndian.AppendUint32(nil, flashBaud)
	baud = binary.LittleEndian.AppendUint32(baud, 0)
	if _, err := bl.command(cmdChangeBaud, baud, 3*time.Second); err == nil {
		bl.port.SetMode(&serial.Mode{BaudRate: flashBaud})
		time.Sleep(50 * time.Millisecond)
		bl.port.ResetInputBuffer()
		bl.buf = nil
	}

	total := 0
	for _, img := range images {
		total += len(img.Data)
	}
	done := 0
	for _, img := range images {
		if err := bl.writeImage(img, func(n int) {
			if progress != nil {
				progress(img.Name, done+n, total)
			}
		}); err != nil {
			return fmt.Errorf("%s : %w", img.Name, err)
		}
		done += len(img.Data)
		if err := bl.verifyMD5(img); err != nil {
			return fmt.Errorf("%s : %w", img.Name, err)
		}
	}
	return nil
}

func (bl *bootloader) writeImage(img FlashImage, progress func(int)) error {
	size := len(img.Data)
	blocks := (size + flashBlockSize - 1) / flashBlockSize

	begin := make([]byte, 0, 16)
	for _, v := range []uint32{uint32(size), uint32(blocks), flashBlockSize, img.Offset} {
		begin = binary.LittleEndian.AppendUint32(begin, v)
	}
	// FLASH_BEGIN efface la zone : environ 30 s par Mo dans le pire cas.
	eraseTimeout := max(5*time.Second, time.Duration(size)*30*time.Second/(1<<20))
	if _, err := bl.command(cmdFlashBegin, begin, eraseTimeout); err != nil {
		return fmt.Errorf("effacement : %w", err)
	}

	for seq := 0; seq < blocks; seq++ {
		block := bytes.Repeat([]byte{0xFF}, flashBlockSize)
		copy(block, img.Data[seq*flashBlockSize:])

		pkt := make([]byte, 0, 16+flashBlockSize)
		for _, v := range []uint32{flashBlockSize, uint32(seq), 0, 0} {
			pkt = binary.LittleEndian.AppendUint32(pkt, v)
		}
		pkt = append(pkt, block...)

		chk := uint32(checksumSeed)
		for _, b := range block {
			chk ^= uint32(b)
		}
		var err error
		for try := 0; try < 3; try++ {
			if _, err = bl.commandChk(cmdFlashData, pkt, chk, 3*time.Second); err == nil {
				break
			}
		}
		if err != nil {
			return fmt.Errorf("écriture du bloc %d/%d : %w", seq+1, blocks, err)
		}
		progress(min((seq+1)*flashBlockSize, size))
	}
	return nil
}

// verifyMD5 demande au ROM le MD5 de la zone écrite et le compare.
func (bl *bootloader) verifyMD5(img FlashImage) error {
	req := make([]byte, 0, 16)
	for _, v := range []uint32{img.Offset, uint32(len(img.Data)), 0, 0} {
		req = binary.LittleEndian.AppendUint32(req, v)
	}
	timeout := max(3*time.Second, time.Duration(len(img.Data))*8*time.Second/(1<<20))
	if _, err := bl.command(cmdSPIFlashMD5, req, timeout); err != nil {
		return fmt.Errorf("vérification MD5 : %w", err)
	}
	// Le ROM renvoie le MD5 en 32 caractères hexadécimaux avant les octets de statut.
	if len(bl.lastBody) < 32 {
		return fmt.Errorf("réponse MD5 invalide")
	}
	sum := md5.Sum(img.Data)
	if got, want := string(bl.lastBody[:32]), hex.EncodeToString(sum[:]); got != want {
		return fmt.Errorf("contenu de la flash différent après écriture (MD5 %s, attendu %s)", got, want)
	}
	return nil
}
