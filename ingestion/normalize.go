package ingestion

import (
	"archive/zip"
	"bufio"
	"compress/gzip"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	MaxUploadBytes     int64 = 100 * 1024 * 1024
	MaxNormalizedBytes int64 = 200 * 1024 * 1024
)

type SourceFormat string

const (
	FormatCSV  SourceFormat = "csv"
	FormatGZIP SourceFormat = "gzip"
	FormatZIP  SourceFormat = "zip"
)

type Result struct {
	SourceFormat    SourceFormat
	SourceEntry     string
	NormalizedBytes int64
}

var (
	ErrUnsupportedFormat = errors.New("unsupported recording format")
	ErrTooLarge          = errors.New("expanded recording exceeds the permitted size")
)

// NormalizeCSV converts a direct CSV or a CSV held in a ZIP/GZIP archive into
// the canonical CSV file consumed by the existing analysis pipeline.
func NormalizeCSV(source io.ReaderAt, size int64, filename, destination string) (Result, error) {
	if size <= 0 || size > MaxUploadBytes {
		return Result{}, fmt.Errorf("recording size must be between 1 byte and %d MB", MaxUploadBytes/(1024*1024))
	}

	section := io.NewSectionReader(source, 0, size)
	format, err := detectFormat(section, filename)
	if err != nil {
		return Result{}, err
	}
	if _, err = section.Seek(0, io.SeekStart); err != nil {
		return Result{}, fmt.Errorf("rewind recording: %w", err)
	}

	var reader io.Reader = section
	result := Result{SourceFormat: format}
	var closer io.Closer

	switch format {
	case FormatGZIP:
		gz, openErr := gzip.NewReader(section)
		if openErr != nil {
			return Result{}, fmt.Errorf("open GZIP recording: %w", openErr)
		}
		reader, closer = gz, gz
		result.SourceEntry = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	case FormatZIP:
		entry, openErr := singleCSVEntry(source, size)
		if openErr != nil {
			return Result{}, openErr
		}
		archiveFile, openErr := entry.Open()
		if openErr != nil {
			return Result{}, fmt.Errorf("open ZIP entry %q: %w", entry.Name, openErr)
		}
		reader, closer = archiveFile, archiveFile
		result.SourceEntry = filepath.ToSlash(entry.Name)
	}
	if closer != nil {
		defer closer.Close()
	}

	written, err := writeLimited(destination, reader, MaxNormalizedBytes)
	if err != nil {
		return Result{}, err
	}
	if err = validateCSV(destination); err != nil {
		_ = os.Remove(destination)
		return Result{}, err
	}
	result.NormalizedBytes = written
	return result, nil
}

func detectFormat(reader *io.SectionReader, filename string) (SourceFormat, error) {
	header := make([]byte, 4)
	n, err := io.ReadFull(reader, header)
	if err != nil && err != io.ErrUnexpectedEOF {
		return "", fmt.Errorf("inspect recording: %w", err)
	}
	header = header[:n]
	if len(header) >= 2 && header[0] == 0x1f && header[1] == 0x8b {
		return FormatGZIP, nil
	}
	if len(header) >= 4 && header[0] == 0x50 && header[1] == 0x4b && header[2] == 0x03 && header[3] == 0x04 {
		return FormatZIP, nil
	}
	if strings.EqualFold(filepath.Ext(filename), ".csv") {
		return FormatCSV, nil
	}
	return "", fmt.Errorf("%w: upload CSV, CSV.GZ, GZ, or ZIP containing one CSV", ErrUnsupportedFormat)
}

func singleCSVEntry(source io.ReaderAt, size int64) (*zip.File, error) {
	archive, err := zip.NewReader(source, size)
	if err != nil {
		return nil, fmt.Errorf("open ZIP recording: %w", err)
	}
	candidates := make([]*zip.File, 0, 1)
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() || !strings.EqualFold(filepath.Ext(entry.Name), ".csv") {
			continue
		}
		if entry.UncompressedSize64 > uint64(MaxNormalizedBytes) {
			return nil, fmt.Errorf("ZIP entry %q: %w", entry.Name, ErrTooLarge)
		}
		candidates = append(candidates, entry)
	}
	if len(candidates) == 0 {
		return nil, errors.New("ZIP archive does not contain a CSV recording")
	}
	if len(candidates) > 1 {
		return nil, errors.New("ZIP archive contains multiple CSV recordings; upload each recording separately")
	}
	return candidates[0], nil
}

func writeLimited(destination string, source io.Reader, limit int64) (int64, error) {
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, fmt.Errorf("create normalized recording: %w", err)
	}
	ok := false
	defer func() {
		_ = output.Close()
		if !ok {
			_ = os.Remove(destination)
		}
	}()

	written, err := io.Copy(output, io.LimitReader(source, limit+1))
	if err != nil {
		return written, fmt.Errorf("extract recording: %w", err)
	}
	if written > limit {
		return written, ErrTooLarge
	}
	if err = output.Sync(); err != nil {
		return written, fmt.Errorf("flush normalized recording: %w", err)
	}
	ok = true
	return written, nil
}

func validateCSV(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open normalized CSV: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(bufio.NewReader(file))
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("recording has no readable CSV header: %w", err)
	}
	if len(header) < 2 {
		return errors.New("recording CSV must contain at least two columns")
	}
	if _, err = reader.Read(); err != nil {
		return fmt.Errorf("recording CSV has no readable data row: %w", err)
	}
	return nil
}
