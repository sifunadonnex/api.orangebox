package ingestion

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

type RecorderPreparation struct {
	Payload         []byte `json:"-"`
	ContainerFormat string `json:"containerFormat"`
	Adapter         string `json:"adapter"`
	Transformed     bool   `json:"transformed"`
}

// PrepareRecorderPayload converts a supported recorder container into the
// plain ARINC 573/717 byte stream expected by FRED validation and decoding.
// Detection remains deliberately conservative: manufacturer-packed FDR/FDT
// images are never guessed or silently treated as flat data.
func PrepareRecorderPayload(filename string, data []byte) (RecorderPreparation, error) {
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	plain := func(adapter string) (RecorderPreparation, error) {
		if _, err := DetectARINC717(data); err != nil {
			return RecorderPreparation{}, err
		}
		return RecorderPreparation{Payload: data, ContainerFormat: format, Adapter: adapter}, nil
	}

	switch format {
	case "ddf", "raw", "bin":
		return RecorderPreparation{Payload: data, ContainerFormat: format, Adapter: "plain-arinc"}, nil
	case "dlu":
		prepared, err := plain("honeywell-dlu-plain")
		if err != nil {
			return RecorderPreparation{}, errors.New("Honeywell DLU is not a plain ARINC frame stream; compressed DLU downloads must be exported with the manufacturer's ground software")
		}
		return prepared, nil
	case "dat":
		if prepared, err := plain("plain-arinc-dat"); err == nil {
			return prepared, nil
		}
		payload, _, err := UnwrapL3Flat(data)
		if err != nil {
			return RecorderPreparation{}, err
		}
		return RecorderPreparation{Payload: payload, ContainerFormat: format, Adapter: "l3-flat", Transformed: true}, nil
	case "tsc":
		if prepared, err := plain("plain-arinc-tsc"); err == nil {
			return prepared, nil
		}
		payload, _, err := UnwrapAvionicaTSC(data)
		if err != nil {
			return RecorderPreparation{}, err
		}
		return RecorderPreparation{Payload: payload, ContainerFormat: format, Adapter: "avionica-tsc", Transformed: true}, nil
	case "dfd":
		var payload bytes.Buffer
		if _, err := ExtractA220DFD(bytes.NewReader(data), int64(len(data)), &payload, 0); err != nil {
			return RecorderPreparation{}, err
		}
		prepared := payload.Bytes()
		if _, err := DetectARINC717(prepared); err != nil {
			return RecorderPreparation{}, fmt.Errorf("A220 DFD payload does not contain a supported ARINC 717 stream: %w", err)
		}
		return RecorderPreparation{Payload: prepared, ContainerFormat: format, Adapter: "a220-dfd", Transformed: true}, nil
	case "fdr", "fdt":
		return RecorderPreparation{}, errors.New("packed FDR/FDT files must first be exported to a flat .dat file with the manufacturer's ground software")
	default:
		return RecorderPreparation{}, fmt.Errorf("unsupported recorder format %q", format)
	}
}

const (
	A220DFDRecordBytes  int64 = 4096
	A220DFDHeaderBytes        = 24
	A220DFDPayloadBytes       = 1536
)

// UnwrapL3Flat removes an L3 download header, padding, and the status word
// preceding every ARINC subframe. Packed DCP/flash-image files are rejected.
func UnwrapL3Flat(data []byte) ([]byte, FrameDetection, error) {
	start := 0
	if marker := bytes.Index(data, []byte(";EOH")); marker >= 0 {
		start = marker + len(";EOH")
		for start < len(data) && (data[start] == '\r' || data[start] == '\n') {
			start++
		}
	}
	for start < len(data) && data[start] == 0xff {
		start++
	}
	if len(data)-start < 2*33*5 {
		return nil, FrameDetection{}, errors.New("L3 flat download does not contain enough frame words")
	}

	words := make([]uint16, (len(data)-start)/2)
	for index := range words {
		words[index] = binary.LittleEndian.Uint16(data[start+index*2:])
	}
	firstSync := -1
	for index := 0; index < len(words) && index < 5000; index++ {
		if syncIndex(words[index]&0x0fff) >= 0 {
			firstSync = index
			break
		}
	}
	if firstSync < 0 {
		return nil, FrameDetection{}, errors.New("no ARINC 717 sync word found in L3 download")
	}
	stride := l3Stride(words[firstSync:])
	if stride == 0 {
		return nil, FrameDetection{}, errors.New("L3 file is not a flat download; export packed FDR/FDT data to .dat with manufacturer software")
	}

	wps := stride - 1
	prefix := firstSync % stride
	clean := make([]uint16, 0, len(words)/stride*wps)
	for position := firstSync - prefix; position+stride <= len(words); position += stride {
		for word := 0; word < wps; word++ {
			clean = append(clean, words[position+prefix+word]&0x0fff)
		}
	}
	packed := packWords12LSB(clean)
	return packed, FrameDetection{WordsPerSecond: wps, Packing: PackingBitsLSB, Score: 24}, nil
}

// UnwrapAvionicaTSC locates the steady packed ARINC frame after an Avionica
// header and returns the aligned recording bytes.
func UnwrapAvionicaTSC(data []byte) ([]byte, FrameDetection, error) {
	for _, offset := range []int{1, 0} {
		wordCount := (len(data) - offset) / 2
		if wordCount < 4000 {
			continue
		}
		words := make([]uint16, wordCount)
		for index := range words {
			words[index] = binary.LittleEndian.Uint16(data[offset+index*2:])
		}
		hits := make([]int, 0, 400)
		for index := 0; index < len(words) && index < 400000 && len(hits) < 400; index++ {
			if syncIndex(words[index]&0x0fff) >= 0 {
				hits = append(hits, index)
			}
		}
		if len(hits) < 20 {
			continue
		}
		best, count := mostCommonDistance(hits)
		wps := best * 16 / 12
		if best*16%12 != 0 || !isStandardWPS(wps) || count < len(hits)*8/10 {
			continue
		}
		first := hits[0]
		for index := 0; index+2 < len(hits); index++ {
			if hits[index+1]-hits[index] == best && hits[index+2]-hits[index+1] == best {
				first = hits[index]
				break
			}
		}
		start := offset + first*2
		return data[start:], FrameDetection{WordsPerSecond: wps, Packing: PackingBitsLSB, Score: count}, nil
	}
	return nil, FrameDetection{}, errors.New("no steady ARINC 717 frame found in Avionica TSC download")
}

// ExtractA220DFD streams the ARINC payload from fixed-size A220 DFD records.
// maxRecords selects the most recent records; zero means every complete record.
func ExtractA220DFD(source io.ReaderAt, size int64, destination io.Writer, maxRecords int64) (int64, error) {
	total := size / A220DFDRecordBytes
	if total < 1 {
		return 0, errors.New("A220 DFD file is smaller than one complete record")
	}
	count := total
	if maxRecords > 0 && count > maxRecords {
		count = maxRecords
	}
	first := total - count
	record := make([]byte, A220DFDRecordBytes)
	for index := int64(0); index < count; index++ {
		if _, err := source.ReadAt(record, (first+index)*A220DFDRecordBytes); err != nil {
			return index, fmt.Errorf("read A220 DFD record %d: %w", first+index, err)
		}
		if _, err := destination.Write(record[A220DFDHeaderBytes : A220DFDHeaderBytes+A220DFDPayloadBytes]); err != nil {
			return index, fmt.Errorf("write A220 DFD payload %d: %w", first+index, err)
		}
	}
	return count, nil
}

func l3Stride(words []uint16) int {
	hits := make([]int, 0, 200)
	for index, word := range words {
		if syncIndex(word&0x0fff) >= 0 {
			hits = append(hits, index)
			if len(hits) == 200 {
				break
			}
		}
	}
	if len(hits) < 5 {
		return 0
	}
	best, count := mostCommonDistance(hits)
	valid := best == 33 || best == 65 || best == 129 || best == 257 || best == 513 || best == 1025 || best == 2049
	if !valid || count < (len(hits)-1)*9/10 {
		return 0
	}
	return best
}

func mostCommonDistance(hits []int) (int, int) {
	counts := make(map[int]int)
	best, bestCount := 0, 0
	for index := 0; index+1 < len(hits); index++ {
		distance := hits[index+1] - hits[index]
		counts[distance]++
		if counts[distance] > bestCount {
			best, bestCount = distance, counts[distance]
		}
	}
	return best, bestCount
}

func isStandardWPS(value int) bool {
	for _, candidate := range standardWPS {
		if value == candidate {
			return true
		}
	}
	return false
}

func packWords12LSB(words []uint16) []byte {
	output := make([]byte, (len(words)*12+7)/8)
	bit := 0
	for _, word := range words {
		value := uint32(word & 0x0fff)
		byteIndex := bit >> 3
		shift := uint(bit & 7)
		output[byteIndex] |= byte(value << shift)
		if byteIndex+1 < len(output) {
			output[byteIndex+1] |= byte(value >> (8 - shift))
		}
		if shift > 4 && byteIndex+2 < len(output) {
			output[byteIndex+2] |= byte(value >> (16 - shift))
		}
		bit += 12
	}
	return output
}
