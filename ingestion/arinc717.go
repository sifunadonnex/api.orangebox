package ingestion

import (
	"errors"
	"fmt"
	"math"
)

type Packing string

const (
	PackingBitsLSB Packing = "bits-lsb"
	PackingBitsMSB Packing = "bits-msb"
	PackingW16LE   Packing = "w16le"
	PackingW16BE   Packing = "w16be"
	PackingW16LEHi Packing = "w16le-hi"
	PackingW16BEHi Packing = "w16be-hi"
)

var (
	standardWPS = []int{64, 128, 256, 512, 1024, 32, 2048}
	packings    = []Packing{PackingBitsLSB, PackingBitsMSB, PackingW16LE, PackingW16BE, PackingW16LEHi, PackingW16BEHi}
	SyncWords   = []uint16{0o1107, 0o2670, 0o5107, 0o6670}
)

type FrameDetection struct {
	WordsPerSecond int     `json:"wordsPerSecond"`
	Packing        Packing `json:"packing"`
	BitOffset      int64   `json:"bitOffset"`
	Score          int     `json:"score"`
}

type FrameQuality struct {
	FrameDetection
	Subframes   int     `json:"subframes"`
	InSync      int     `json:"inSync"`
	InSyncPct   float64 `json:"inSyncPercent"`
	Missing     int     `json:"missingSubframes"`
	SyncLosses  int     `json:"syncLosses"`
	LeadingBits int64   `json:"leadingBits"`
}

type wordReader struct {
	data     []byte
	packing  Packing
	wordBits int64
	stepBits int64
	total    int64
}

func newWordReader(data []byte, packing Packing) (wordReader, error) {
	reader := wordReader{data: append(data, 0, 0, 0, 0), packing: packing, total: int64(len(data) * 8)}
	switch packing {
	case PackingBitsLSB, PackingBitsMSB:
		reader.wordBits, reader.stepBits = 12, 1
	case PackingW16LE, PackingW16BE, PackingW16LEHi, PackingW16BEHi:
		reader.wordBits, reader.stepBits = 16, 8
	default:
		return wordReader{}, fmt.Errorf("unknown ARINC packing %q", packing)
	}
	return reader, nil
}

func (r wordReader) at(bit int64) uint16 {
	i := int(bit >> 3)
	shift := uint(bit & 7)
	switch r.packing {
	case PackingBitsLSB:
		value := uint32(r.data[i]) | uint32(r.data[i+1])<<8 | uint32(r.data[i+2])<<16
		return uint16((value >> shift) & 0x0fff)
	case PackingBitsMSB:
		value := uint32(r.data[i])<<16 | uint32(r.data[i+1])<<8 | uint32(r.data[i+2])
		return uint16((value >> (12 - shift)) & 0x0fff)
	case PackingW16LE:
		return (uint16(r.data[i]) | uint16(r.data[i+1])<<8) & 0x0fff
	case PackingW16BE:
		return (uint16(r.data[i])<<8 | uint16(r.data[i+1])) & 0x0fff
	case PackingW16LEHi:
		return (uint16(r.data[i]) | uint16(r.data[i+1])<<8) >> 4 & 0x0fff
	case PackingW16BEHi:
		return (uint16(r.data[i])<<8 | uint16(r.data[i+1])) >> 4 & 0x0fff
	default:
		return 0
	}
}

// DetectARINC717 identifies standard ARINC 573/717 rates and common 12/16-bit
// packing layouts by looking for a repeating four-subframe sync sequence.
func DetectARINC717(data []byte) (FrameDetection, error) {
	if len(data) < 16 {
		return FrameDetection{}, errors.New("recording is too small to contain an ARINC 717 frame")
	}
	var best *FrameDetection
	for _, packing := range packings {
		reader, _ := newWordReader(data, packing)
		for _, wps := range standardWPS {
			limit := reader.total
			if limit > 64_000_000 {
				limit = 64_000_000
			}
			position := findSync(reader, wps, 0, limit)
			if position < 0 {
				continue
			}
			score := consecutiveSyncs(reader, wps, position, 24)
			candidate := FrameDetection{WordsPerSecond: wps, Packing: packing, BitOffset: position, Score: score}
			if best == nil || candidate.Score > best.Score {
				copy := candidate
				best = &copy
			}
		}
	}
	if best == nil || best.Score < 3 {
		return FrameDetection{}, errors.New("no repeating ARINC 573/717 frame sync pattern was found")
	}
	return *best, nil
}

// InspectARINC717 measures synchronization quality without decoding aircraft
// parameters. Engineering-unit conversion is deliberately a separate step.
func InspectARINC717(data []byte, detection FrameDetection) (FrameQuality, error) {
	reader, err := newWordReader(data, detection.Packing)
	if err != nil {
		return FrameQuality{}, err
	}
	if detection.WordsPerSecond <= 0 {
		return FrameQuality{}, errors.New("words per second must be positive")
	}
	span := int64(detection.WordsPerSecond) * reader.wordBits
	position := detection.BitOffset
	quality := FrameQuality{FrameDetection: detection, LeadingBits: position}
	for position+span <= reader.total {
		sync := syncIndex(reader.at(position))
		if sync >= 0 {
			quality.Subframes++
			quality.InSync++
			position += span
			continue
		}

		next := findSync(reader, detection.WordsPerSecond, position+reader.stepBits, reader.total)
		if next < 0 {
			break
		}
		missing := int(math.Round(float64(next-position) / float64(span)))
		if missing > 0 {
			quality.Subframes += missing
			quality.Missing += missing
		}
		quality.SyncLosses++
		position = next
	}
	if quality.Subframes == 0 {
		return FrameQuality{}, errors.New("ARINC detection did not yield any complete subframes")
	}
	quality.InSyncPct = float64(quality.InSync) / float64(quality.Subframes) * 100
	return quality, nil
}

func findSync(reader wordReader, wps int, from, limit int64) int64 {
	span := int64(wps) * reader.wordBits
	end := limit - span - reader.wordBits
	if end > reader.total-span-reader.wordBits {
		end = reader.total - span - reader.wordBits
	}
	for position := from; position <= end; position += reader.stepBits {
		index := syncIndex(reader.at(position))
		if index >= 0 && reader.at(position+span) == SyncWords[(index+1)&3] {
			return position
		}
	}
	return -1
}

func consecutiveSyncs(reader wordReader, wps int, position int64, maximum int) int {
	span := int64(wps) * reader.wordBits
	score := 0
	for score < maximum && position+span <= reader.total && syncIndex(reader.at(position)) >= 0 {
		score++
		position += span
	}
	return score
}

func syncIndex(word uint16) int {
	for index, sync := range SyncWords {
		if word == sync {
			return index
		}
	}
	return -1
}
