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

func TestPrepareRecorderPayloadUsesL3Adapter(t *testing.T) {
	const wps = 64
	var source bytes.Buffer
	source.WriteString("L3 Communications\r\nFORMAT=FLAT\r\n;EOH\r\n")
	for subframe := 0; subframe < 24; subframe++ {
		_ = binary.Write(&source, binary.LittleEndian, uint16(0x8000))
		_ = binary.Write(&source, binary.LittleEndian, SyncWords[subframe&3])
		for word := 1; word < wps; word++ {
			_ = binary.Write(&source, binary.LittleEndian, uint16(word))
		}
	}
	prepared, err := PrepareRecorderPayload("flight.dat", source.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Adapter != "l3-flat" || !prepared.Transformed || prepared.ContainerFormat != "dat" {
		t.Fatalf("unexpected preparation: %+v", prepared)
	}
	if _, err = DetectARINC717(prepared.Payload); err != nil {
		t.Fatalf("prepared L3 stream is not detectable: %v", err)
	}
}

func TestPrepareRecorderPayloadRejectsPackedManufacturerFile(t *testing.T) {
	if _, err := PrepareRecorderPayload("download.fdt", make([]byte, 8192)); err == nil {
		t.Fatal("expected packed FDT guidance")
	}
}

func TestPrepareRecorderPayloadExtractsA220DFD(t *testing.T) {
	const wps = 64
	words := make([]uint16, 80*wps)
	for subframe := 0; subframe < 80; subframe++ {
		words[subframe*wps] = SyncWords[subframe&3]
	}
	packed := packWords12LSB(words)
	records := (len(packed) + A220DFDPayloadBytes - 1) / A220DFDPayloadBytes
	source := make([]byte, records*int(A220DFDRecordBytes))
	for record := 0; record < records; record++ {
		start := record * A220DFDPayloadBytes
		end := start + A220DFDPayloadBytes
		if end > len(packed) {
			end = len(packed)
		}
		copy(source[record*int(A220DFDRecordBytes)+A220DFDHeaderBytes:], packed[start:end])
	}
	prepared, err := PrepareRecorderPayload("a220.dfd", source)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Adapter != "a220-dfd" || !prepared.Transformed {
		t.Fatalf("unexpected preparation: %+v", prepared)
	}
	detection, err := DetectARINC717(prepared.Payload)
	if err != nil || detection.WordsPerSecond != wps {
		t.Fatalf("prepared DFD stream is not detectable: %+v %v", detection, err)
	}
}
