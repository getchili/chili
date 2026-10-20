package main

import (
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/getchili/chili/compiler/natives"
	"github.com/getchili/chili/compiler/refvm"
)

// simulator fournit les natives de la carte pour `chili run` : les broches
// sont simulées et leurs changements affichés.
type simulator struct {
	vm    *refvm.VM
	start time.Time
	pins  map[int32]int32
}

func newSimulator(start time.Time) *simulator {
	return &simulator{start: start, pins: map[int32]int32{}}
}

// adc simule une entrée analogique : une rampe qui monte puis descend de
// 0 à 4095 en 4 secondes, comme un potentiomètre qu'on tourne.
func (s *simulator) adc() int32 {
	t := time.Since(s.start).Milliseconds() % 4000
	if t > 2000 {
		t = 4000 - t
	}
	return int32(t * 4095 / 2000)
}

func (s *simulator) call(id int, v []refvm.Value) (refvm.Value, error) {
	zero := refvm.IntV(0)
	name := natives.Name(id)
	switch name {
	case "print":
		fmt.Println(s.vm.Text(v[0]))
	case "delay":
		if v[0].I > 0 {
			time.Sleep(time.Duration(v[0].I) * time.Millisecond)
		}
	case "millis":
		return refvm.IntV(int32(time.Since(s.start).Milliseconds())), nil
	case "gpio.mode":
		fmt.Printf("[gpio] broche %d en mode %d\n", v[0].I, v[1].I)
	case "gpio.write":
		s.pins[v[0].I] = v[1].I
		fmt.Printf("[gpio] broche %d = %d\n", v[0].I, v[1].I)
	case "gpio.read":
		return refvm.IntV(s.pins[v[0].I]), nil
	case "random":
		r, err := natives.RandomRange(v[0].I, v[1].I, rand.Uint32())
		return refvm.IntV(r), err
	case "printc":
		if err := natives.CheckChar("printc", v[0].I); err != nil {
			return zero, err
		}
		fmt.Print(string(rune(v[0].I)))
	case "println":
		fmt.Println()

	case "pwm.write":
		if err := natives.CheckRange(name, "valeur", v[1].I, 0, natives.PWMMax); err != nil {
			return zero, err
		}
		fmt.Printf("[pwm] broche %d = %d/255\n", v[0].I, v[1].I)
	case "pwm.tone":
		if err := natives.CheckTone(v[1].I); err != nil {
			return zero, err
		}
		if v[1].I == 0 {
			fmt.Printf("[pwm] broche %d : silence\n", v[0].I)
		} else {
			fmt.Printf("[pwm] broche %d : son %d Hz\n", v[0].I, v[1].I)
		}
	case "servo.write":
		if err := natives.CheckRange(name, "angle", v[1].I, 0, natives.ServoMax); err != nil {
			return zero, err
		}
		fmt.Printf("[servo] broche %d = %d° (impulsion %d µs)\n", v[0].I, v[1].I, natives.ServoPulse(v[1].I))
	case "adc.read":
		return refvm.IntV(s.adc()), nil
	case "adc.mv":
		return refvm.IntV(s.adc() * 3300 / 4095), nil

	// Pas de bus I2C sur le PC : aucun périphérique ne répond.
	case "i2c.begin":
		fmt.Printf("[i2c] bus sur SDA %d, SCL %d\n", v[0].I, v[1].I)
	case "i2c.scan":
		fmt.Println("[i2c] scan : aucun périphérique (simulation)")
		return refvm.ArrV(), nil
	case "i2c.write":
		if err := natives.CheckRange(name, "adresse", v[0].I, 0, 127); err != nil {
			return zero, err
		}
		b, ok := refvm.Bytes(v[1])
		if !ok {
			return zero, fmt.Errorf("i2c.write : les données doivent être une chaîne ou un tableau d'octets (0 à 255)")
		}
		if err := natives.CheckRange(name, "taille", int32(len(b)), 0, natives.I2CMaxBytes); err != nil {
			return zero, err
		}
		fmt.Printf("[i2c] écriture vers 0x%02X : % X (pas de réponse)\n", v[0].I, b)
	case "i2c.read":
		if err := natives.CheckRange(name, "adresse", v[0].I, 0, 127); err != nil {
			return zero, err
		}
		if err := natives.CheckRange(name, "taille", v[1].I, 0, natives.I2CMaxBytes); err != nil {
			return zero, err
		}
		fmt.Printf("[i2c] lecture de %d octet(s) depuis 0x%02X (pas de réponse)\n", v[1].I, v[0].I)
		return refvm.ArrV(), nil

	default:
		return zero, fmt.Errorf("native %s non simulée", name)
	}
	return zero, nil
}
