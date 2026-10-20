package device

import (
	"bytes"
	"testing"
)

func TestSlipRoundTrip(t *testing.T) {
	pkt := []byte{0x00, 0xC0, 0x01, 0xDB, 0xDC, 0xDD, 0xC0}
	enc := slipEncode(pkt)
	want := []byte{0xC0, 0x00, 0xDB, 0xDC, 0x01, 0xDB, 0xDD, 0xDC, 0xDD, 0xDB, 0xDC, 0xC0}
	if !bytes.Equal(enc, want) {
		t.Fatalf("encodage : % x, attendu % x", enc, want)
	}
	if dec := slipDecode(enc[1 : len(enc)-1]); !bytes.Equal(dec, pkt) {
		t.Fatalf("décodage : % x, attendu % x", dec, pkt)
	}
}

func TestParseFields(t *testing.T) {
	info := parseFields("version=0.1.0 chip=ESP32 program=111:1a2b3c4d state=running bruit")
	want := map[string]string{"version": "0.1.0", "chip": "ESP32", "program": "111:1a2b3c4d", "state": "running"}
	if len(info) != len(want) {
		t.Fatalf("%v", info)
	}
	for k, v := range want {
		if info[k] != v {
			t.Errorf("%s = %q, attendu %q", k, info[k], v)
		}
	}
}
