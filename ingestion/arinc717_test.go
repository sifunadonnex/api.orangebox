package ingestion

import (
	"testing"
)

func TestDetectAndInspectARINC717PackedLSB(t *testing.T) {
	const wps = 64
	words := make([]uint16, 12*wps)
	for subframe := 0; subframe < 12; subframe++ {
		words[subframe*wps] = SyncWords[subframe&3]
		for word := 1; word < wps; word++ {
			words[subframe*wps+word] = uint16((subframe*31 + word) & 0x0fff)
		}
	}
	data := append(make([]byte, 17), pack12LSB(words)...)
	detection, err := DetectARINC717(data)
	if err != nil {
		t.Fatal(err)
	}
	if detection.WordsPerSecond != wps || detection.Packing != PackingBitsLSB {
		t.Fatalf("unexpected detection: %+v", detection)
	}
	quality, err := InspectARINC717(data, detection)
	if err != nil {
		t.Fatal(err)
	}
	if quality.InSync < 11 || quality.InSyncPct < 99 {
		t.Fatalf("unexpected quality: %+v", quality)
	}
}

func TestDetectARINC717RejectsNoise(t *testing.T) {
	data := make([]byte, 4096)
	for index := range data {
		data[index] = byte(index*37 + 11)
	}
	if _, err := DetectARINC717(data); err == nil {
		t.Fatal("expected noise to be rejected")
	}
}

func pack12LSB(words []uint16) []byte {
	output := make([]byte, (len(words)*12+7)/8)
	bit := 0
	for _, word := range words {
		value := uint32(word & 0x0fff)
		byteIndex := bit >> 3
		shift := uint(bit & 7)
		output[byteIndex] |= byte(value << shift)
		output[byteIndex+1] |= byte(value >> (8 - shift))
		if shift > 4 && byteIndex+2 < len(output) {
			output[byteIndex+2] |= byte(value >> (16 - shift))
		}
		bit += 12
	}
	return output
}
