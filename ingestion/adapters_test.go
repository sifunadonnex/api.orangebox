package ingestion

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestUnwrapL3Flat(t *testing.T) {
	const wps = 64
	var source bytes.Buffer
	source.WriteString("L3 Communications\r\nFORMAT=FLAT\r\n;EOH\r\n")
	source.Write([]byte{0xff, 0xff, 0xff, 0xff})
	for subframe := 0; subframe < 12; subframe++ {
		_ = binary.Write(&source, binary.LittleEndian, uint16(0x8000))
		_ = binary.Write(&source, binary.LittleEndian, SyncWords[subframe&3])
		for word := 1; word < wps; word++ {
			_ = binary.Write(&source, binary.LittleEndian, uint16(word))
		}
	}
	raw, detection, err := UnwrapL3Flat(source.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if detection.WordsPerSecond != wps {
		t.Fatalf("unexpected detection: %+v", detection)
	}
	quality, err := InspectARINC717(raw, detection)
	if err != nil || quality.InSyncPct < 99 {
		t.Fatalf("unexpected L3 quality: %+v, %v", quality, err)
	}
}

func TestUnwrapAvionicaTSC(t *testing.T) {
	const wps = 64
	words := make([]uint16, 100*wps)
	for subframe := 0; subframe < 100; subframe++ {
		words[subframe*wps] = SyncWords[subframe&3]
	}
	packed := packWords12LSB(words)
	source := append([]byte{0x42}, packed...)
	raw, detection, err := UnwrapAvionicaTSC(source)
	if err != nil {
		t.Fatal(err)
	}
	if detection.WordsPerSecond != wps || detection.Packing != PackingBitsLSB {
		t.Fatalf("unexpected TSC detection: %+v", detection)
	}
	quality, err := InspectARINC717(raw, detection)
	if err != nil || quality.InSyncPct < 99 {
		t.Fatalf("unexpected TSC quality: %+v, %v", quality, err)
	}
}

func TestExtractA220DFDUsesMostRecentRecords(t *testing.T) {
	source := make([]byte, 3*int(A220DFDRecordBytes))
	for record := 0; record < 3; record++ {
		for index := 0; index < A220DFDPayloadBytes; index++ {
			source[record*int(A220DFDRecordBytes)+A220DFDHeaderBytes+index] = byte(record + 1)
		}
	}
	var output bytes.Buffer
	count, err := ExtractA220DFD(bytes.NewReader(source), int64(len(source)), &output, 2)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || output.Len() != 2*A220DFDPayloadBytes {
		t.Fatalf("unexpected extraction: records=%d bytes=%d", count, output.Len())
	}
	if output.Bytes()[0] != 2 || output.Bytes()[A220DFDPayloadBytes] != 3 {
		t.Fatal("expected the two most recent DFD records")
	}
}
