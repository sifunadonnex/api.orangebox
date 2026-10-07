package ingestion

import (
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

const maxProbeSubframes = 20_000

type FREDProfile struct {
	AircraftModel      string
	FileRevision       string
	WordsPerSecond     int
	BitsPerWord        int
	SecondsPerSubframe float64
	SyncWords          []uint16
	Parameters         []FREDParameter
}

type FREDParameter struct {
	Name       string
	Mnemonic   string
	Units      string
	DataType   string
	Samples    []FREDSample
	Conversion FREDConversion
}

type FREDSample struct {
	Segments []FREDSegment
}

type FREDSegment struct {
	Word      int
	LowBit    int
	HighBit   int
	Subframes []int
}

type FREDConversion struct {
	Signed bool
	Points []FREDPoint
}

type FREDPoint struct {
	Raw   float64 `json:"raw"`
	Value float64 `json:"value"`
}

type ParameterProbe struct {
	Mnemonic  string  `json:"mnemonic"`
	Name      string  `json:"name"`
	Units     string  `json:"units"`
	Samples   int     `json:"samples"`
	Minimum   float64 `json:"minimum"`
	Maximum   float64 `json:"maximum"`
	Varying   bool    `json:"varying"`
	Core      bool    `json:"core"`
	Plausible bool    `json:"plausible"`
}

type RecordingValidationReport struct {
	ProfileFormat           string           `json:"profileFormat"`
	AircraftModel           string           `json:"aircraftModel"`
	FileRevision            string           `json:"fileRevision"`
	ProfileWordsPerSecond   int              `json:"profileWordsPerSecond"`
	Recording               FrameQuality     `json:"recording"`
	DurationSeconds         float64          `json:"durationSeconds"`
	ParametersDefined       int              `json:"parametersDefined"`
	ParametersDecoded       int              `json:"parametersDecoded"`
	ParametersVarying       int              `json:"parametersVarying"`
	CoreParametersChecked   int              `json:"coreParametersChecked"`
	CoreParametersPlausible int              `json:"coreParametersPlausible"`
	ParameterChecks         []ParameterProbe `json:"parameterChecks"`
	Message                 string           `json:"message"`
}

type CanonicalConversionResult struct {
	ProfileFormat        string       `json:"profileFormat"`
	AircraftModel        string       `json:"aircraftModel"`
	FileRevision         string       `json:"fileRevision"`
	Recording            FrameQuality `json:"recording"`
	Rows                 int          `json:"rows"`
	Columns              []string     `json:"columns"`
	SampleIntervalMs     int64        `json:"sampleIntervalMs"`
	NormalizedBytes      int64        `json:"normalizedBytes"`
	MissingCanonicalData []string     `json:"missingCanonicalData,omitempty"`
}

type canonicalSignal struct {
	Header    string
	Parameter *FREDParameter
	Value     float64
	HasValue  bool
}

type canonicalDefinition struct {
	header     string
	identities []string
	required   bool
}

var fredCanonicalDefinitions = []canonicalDefinition{
	{"Airspeed", []string{"lcomputedairspeed", "computedairspeed", "lindicatedairspeed", "indicatedairspeed", "rcomputedairspeed"}, true},
	{"Altitude", []string{"lpressurealtitude", "pressurealtitude", "rpressurealtitude"}, true},
	{"Pitch", []string{"lpitchangle", "pitchangle", "rpitchangle"}, false},
	{"Roll", []string{"lrollangle", "rollangle", "rrollangle"}, false},
	{"Vertical Acceleration", []string{"verticalacceleration", "normalacceleration"}, false},
	{"Radio Altitude", []string{"lradioaltitude", "radioaltitude", "rradioaltitude"}, false},
	{"Ground Speed", []string{"lgroundspeed", "groundspeed", "rgroundspeed"}, false},
	{"WOW", []string{"lgearwow", "nosegearwow", "gearwow", "rgearwow", "weightonwheels"}, false},
	{"Flaps", []string{"lflapposition", "flapposition", "rflapposition"}, false},
	{"Heading", []string{"lmagneticheading", "magneticheading", "rmagneticheading", "trueheading"}, false},
	{"Left Engine N1", []string{"lenginen1", "leftenginen1"}, false},
	{"Right Engine N1", []string{"renginen1", "rightenginen1"}, false},
	{"GMT Hours", []string{"utctimehour", "gmttimehour", "gmthours"}, false},
	{"GMT Minutes", []string{"utctimeminute", "gmttimeminute", "gmtminutes"}, false},
	{"GMT Seconds", []string{"utctimesecond", "gmttimesecond", "gmtseconds"}, false},
}

type fredDocument struct {
	FRED717 fred717 `xml:"FRED717"`
}

type fred717 struct {
	Header     fredHeader      `xml:"Header"`
	Subframe   fredSubframe    `xml:"Subframe"`
	Parameters []fredParameter `xml:"Parameter717"`
}

type fredHeader struct {
	FileRevision  string `xml:"File_Revision"`
	AircraftModel string `xml:"Aircraft_Make_and_Model"`
}

type fredSubframe struct {
	BitsPerWord         int      `xml:"Bits_Per_Word"`
	WordsPerSubframe    int      `xml:"Words_Per_Subframe"`
	FDRWordsPerSubframe int      `xml:"FDR_Words_Per_Subframe"`
	Seconds             fraction `xml:"Seconds_Per_Subframe"`
	SyncPattern         []string `xml:"Sync_Pattern>Sync_Code"`
}

type fraction struct {
	Numerator   float64 `xml:"Numerator"`
	Denominator float64 `xml:"Denominator"`
}

type fredParameter struct {
	Name       string          `xml:"Name"`
	Mnemonic   string          `xml:"Mnemonic_Code"`
	Units      string          `xml:"Units"`
	Components []fredComponent `xml:"Component"`
	Ranges     []fredRange     `xml:"Range"`
}

type fredComponent struct {
	Words     []int    `xml:"Word_Numbers>Word_Num"`
	Subframes []int    `xml:"Subframe_Numbers>Subframe_Num"`
	Bits      fredBits `xml:"Bits"`
}

type fredBits struct {
	Start int `xml:"OneBasedIntRange_Start"`
	End   int `xml:"OneBasedIntRange_End"`
}

type fredRange struct {
	DataType       string               `xml:"Data_Type"`
	ConversionStep []fredConversionStep `xml:"Conversion_Step"`
}

type fredConversionStep struct {
	IntegerRealTable struct {
		Pairs []fredPair `xml:"Integer_Real_Pair"`
	} `xml:"Integer_Real_Table"`
}

type fredPair struct {
	Index string `xml:"index,attr"`
	Value string `xml:",chardata"`
}

func CompileFREDProfile(content []byte) (FREDProfile, error) {
	var document fredDocument
	if err := xml.Unmarshal(content, &document); err != nil {
		return FREDProfile{}, fmt.Errorf("parse FRED XML: %w", err)
	}
	fred := document.FRED717
	wps := fred.Subframe.WordsPerSubframe
	if wps == 0 {
		wps = fred.Subframe.FDRWordsPerSubframe
	}
	if wps == 1025 {
		wps = 1024
	}
	if !isStandardWPS(wps) {
		return FREDProfile{}, fmt.Errorf("unsupported FRED words per subframe %d", wps)
	}
	bitsPerWord := fred.Subframe.BitsPerWord
	if bitsPerWord == 0 {
		bitsPerWord = 12
	}
	if bitsPerWord != 12 {
		return FREDProfile{}, fmt.Errorf("unsupported FRED word width %d; ARINC 717 decoding currently requires 12-bit words", bitsPerWord)
	}
	duration := 1.0
	if fred.Subframe.Seconds.Numerator > 0 && fred.Subframe.Seconds.Denominator > 0 {
		duration = fred.Subframe.Seconds.Numerator / fred.Subframe.Seconds.Denominator
	}
	syncWords := make([]uint16, 0, 4)
	for _, value := range fred.Subframe.SyncPattern {
		parsed, err := strconv.ParseUint(strings.TrimSpace(value), 2, 16)
		if err != nil || parsed > 0x0fff {
			return FREDProfile{}, fmt.Errorf("invalid FRED sync code %q", value)
		}
		syncWords = append(syncWords, uint16(parsed))
	}
	if len(syncWords) == 0 {
		syncWords = append(syncWords, SyncWords...)
	}
	if len(syncWords) != 4 {
		return FREDProfile{}, fmt.Errorf("FRED profile must define four sync codes, found %d", len(syncWords))
	}

	profile := FREDProfile{
		AircraftModel:      strings.TrimSpace(fred.Header.AircraftModel),
		FileRevision:       strings.TrimSpace(fred.Header.FileRevision),
		WordsPerSecond:     wps,
		BitsPerWord:        bitsPerWord,
		SecondsPerSubframe: duration,
		SyncWords:          syncWords,
		Parameters:         make([]FREDParameter, 0, len(fred.Parameters)),
	}
	for index, source := range fred.Parameters {
		parameter, err := compileFREDParameter(source, wps)
		if err != nil {
			return FREDProfile{}, fmt.Errorf("parameter %d (%s): %w", index+1, source.Name, err)
		}
		profile.Parameters = append(profile.Parameters, parameter)
	}
	if len(profile.Parameters) == 0 {
		return FREDProfile{}, errors.New("FRED profile contains no decodable ARINC 717 parameters")
	}
	return profile, nil
}

func compileFREDParameter(source fredParameter, wps int) (FREDParameter, error) {
	if len(source.Components) == 0 {
		return FREDParameter{}, errors.New("no components")
	}
	occurrences := 0
	for _, component := range source.Components {
		if len(component.Words) > occurrences {
			occurrences = len(component.Words)
		}
	}
	if occurrences == 0 {
		return FREDParameter{}, errors.New("no word numbers")
	}
	parameter := FREDParameter{
		Name:     strings.TrimSpace(source.Name),
		Mnemonic: strings.TrimSpace(source.Mnemonic),
		Units:    strings.TrimSpace(source.Units),
		Samples:  make([]FREDSample, 0, occurrences),
	}
	if parameter.Mnemonic == "" {
		parameter.Mnemonic = parameter.Name
	}
	if len(source.Ranges) > 0 {
		parameter.DataType = strings.TrimSpace(source.Ranges[0].DataType)
		parameter.Conversion.Signed = strings.HasPrefix(strings.ToLower(parameter.DataType), "signed binary")
		for _, step := range source.Ranges[0].ConversionStep {
			for _, pair := range step.IntegerRealTable.Pairs {
				raw, rawErr := strconv.ParseFloat(strings.TrimSpace(pair.Index), 64)
				value, valueErr := strconv.ParseFloat(strings.TrimSpace(pair.Value), 64)
				if rawErr == nil && valueErr == nil {
					parameter.Conversion.Points = append(parameter.Conversion.Points, FREDPoint{Raw: raw, Value: value})
				}
			}
		}
		sort.Slice(parameter.Conversion.Points, func(i, j int) bool {
			return parameter.Conversion.Points[i].Raw < parameter.Conversion.Points[j].Raw
		})
	}
	for occurrence := 0; occurrence < occurrences; occurrence++ {
		sample := FREDSample{Segments: make([]FREDSegment, 0, len(source.Components))}
		for componentIndex := len(source.Components) - 1; componentIndex >= 0; componentIndex-- {
			component := source.Components[componentIndex]
			if len(component.Words) == 0 {
				continue
			}
			wordIndex := occurrence
			if len(component.Words) == 1 || wordIndex >= len(component.Words) {
				wordIndex = len(component.Words) - 1
			}
			word := component.Words[wordIndex]
			low, high := component.Bits.Start, component.Bits.End
			if low > high {
				low, high = high, low
			}
			if word < 1 || word > wps || low < 1 || high > 12 {
				return FREDParameter{}, fmt.Errorf("invalid allocation W%d B%d-%d for %d WPS", word, low, high, wps)
			}
			sample.Segments = append(sample.Segments, FREDSegment{
				Word: word, LowBit: low, HighBit: high, Subframes: append([]int(nil), component.Subframes...),
			})
		}
		if len(sample.Segments) > 0 {
			parameter.Samples = append(parameter.Samples, sample)
		}
	}
	if len(parameter.Samples) == 0 {
		return FREDParameter{}, errors.New("no usable samples")
	}
	return parameter, nil
}

func ValidateFREDRecording(profileContent, recording []byte) (RecordingValidationReport, error) {
	profile, err := CompileFREDProfile(profileContent)
	if err != nil {
		return RecordingValidationReport{}, err
	}
	detection, err := DetectARINC717(recording)
	if err != nil {
		return RecordingValidationReport{}, err
	}
	if detection.WordsPerSecond != profile.WordsPerSecond {
		return RecordingValidationReport{}, fmt.Errorf("recording is %d WPS but the FRED profile requires %d WPS", detection.WordsPerSecond, profile.WordsPerSecond)
	}
	quality, err := InspectARINC717(recording, detection)
	if err != nil {
		return RecordingValidationReport{}, err
	}
	if quality.Subframes < 20 {
		return RecordingValidationReport{}, errors.New("recording has fewer than 20 complete subframes")
	}
	if quality.InSyncPct < 95 {
		return RecordingValidationReport{}, fmt.Errorf("recording synchronization is %.1f%%; at least 95%% is required", quality.InSyncPct)
	}

	report := RecordingValidationReport{
		ProfileFormat:         "fred",
		AircraftModel:         profile.AircraftModel,
		FileRevision:          profile.FileRevision,
		ProfileWordsPerSecond: profile.WordsPerSecond,
		Recording:             quality,
		DurationSeconds:       float64(quality.Subframes) * profile.SecondsPerSubframe,
		ParametersDefined:     len(profile.Parameters),
		ParameterChecks:       make([]ParameterProbe, 0, len(profile.Parameters)),
	}
	reader, _ := newWordReader(recording, detection.Packing)
	for _, parameter := range profile.Parameters {
		probe := probeParameter(reader, detection, quality.Subframes, parameter)
		if probe.Samples == 0 {
			continue
		}
		report.ParametersDecoded++
		if probe.Varying {
			report.ParametersVarying++
		}
		if probe.Core {
			report.CoreParametersChecked++
			if probe.Plausible {
				report.CoreParametersPlausible++
			}
		}
		if probe.Core {
			report.ParameterChecks = append(report.ParameterChecks, probe)
		}
	}
	if report.ParametersDecoded == 0 {
		return RecordingValidationReport{}, errors.New("the profile did not decode any parameter samples from this recording")
	}
	if report.CoreParametersChecked > 0 && report.CoreParametersPlausible != report.CoreParametersChecked {
		return report, errors.New("one or more core flight parameters decoded outside broad physical limits")
	}
	report.Message = fmt.Sprintf(
		"Validated %d parameters against %.1f hours of recording at %.1f%% synchronization",
		report.ParametersDecoded, report.DurationSeconds/3600, quality.InSyncPct,
	)
	return report, nil
}

// DecodeFREDToCanonicalCSV decodes a plain ARINC 573/717 recording with a
// validated FRED profile into the compact CSV contract consumed by the phase,
// replay, and exceedance engines. The raw recording remains the source of
// truth; this file is a reproducible analysis derivative.
func DecodeFREDToCanonicalCSV(profileContent, recording []byte, destination string) (result CanonicalConversionResult, err error) {
	profile, err := CompileFREDProfile(profileContent)
	if err != nil {
		return result, err
	}
	detection, err := DetectARINC717(recording)
	if err != nil {
		return result, err
	}
	if detection.WordsPerSecond != profile.WordsPerSecond {
		return result, fmt.Errorf("recording is %d WPS but the FRED profile requires %d WPS", detection.WordsPerSecond, profile.WordsPerSecond)
	}
	quality, err := InspectARINC717(recording, detection)
	if err != nil {
		return result, err
	}
	if quality.Subframes < 20 {
		return result, errors.New("recording has fewer than 20 complete subframes")
	}
	if quality.InSyncPct < 95 {
		return result, fmt.Errorf("recording synchronization is %.1f%%; at least 95%% is required", quality.InSyncPct)
	}

	signals := make([]canonicalSignal, 0, len(fredCanonicalDefinitions))
	missing := make([]string, 0)
	for _, definition := range fredCanonicalDefinitions {
		parameter := findFREDParameter(profile.Parameters, definition.identities...)
		if parameter == nil {
			if definition.required {
				return result, fmt.Errorf("FRED profile does not define a canonical %s parameter", definition.header)
			}
			missing = append(missing, definition.header)
		}
		signals = append(signals, canonicalSignal{Header: definition.header, Parameter: parameter})
	}

	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return result, fmt.Errorf("create canonical CSV: %w", err)
	}
	complete := false
	defer func() {
		_ = output.Close()
		if !complete {
			_ = os.Remove(destination)
		}
	}()

	columns := []string{"Sample", "Time(sec)"}
	for _, signal := range signals {
		columns = append(columns, signal.Header)
	}
	columns = append(columns, "Vertical Speed")
	writer := csv.NewWriter(output)
	if err = writer.Write(columns); err != nil {
		return result, fmt.Errorf("write canonical CSV header: %w", err)
	}

	reader, _ := newWordReader(recording, detection.Packing)
	span := int64(detection.WordsPerSecond) * reader.wordBits
	position := detection.BitOffset
	logicalIndex := 0
	rows := 0
	var previousAltitude float64
	previousAltitudeIndex := 0
	hasPreviousAltitude := false

	for position+span <= reader.total {
		subframe := profileSyncIndex(reader.at(position), profile.SyncWords)
		if subframe < 0 {
			next := findSync(reader, detection.WordsPerSecond, position+reader.stepBits, reader.total)
			if next < 0 {
				break
			}
			skipped := int(math.Round(float64(next-position) / float64(span)))
			if skipped < 1 {
				skipped = 1
			}
			logicalIndex += skipped
			position = next
			continue
		}
		subframe++
		for index := range signals {
			if signals[index].Parameter == nil {
				continue
			}
			if value, ok := decodeFREDParameter(reader, position, subframe, *signals[index].Parameter); ok {
				signals[index].Value = value
				signals[index].HasValue = true
			}
		}

		record := make([]string, 0, len(columns))
		record = append(record, strconv.Itoa(logicalIndex), formatFREDFloat(float64(logicalIndex)*profile.SecondsPerSubframe))
		altitude, hasAltitude := 0.0, false
		for _, signal := range signals {
			if signal.HasValue {
				record = append(record, formatFREDFloat(signal.Value))
				if signal.Header == "Altitude" {
					altitude, hasAltitude = signal.Value, true
				}
			} else {
				record = append(record, "")
			}
		}
		verticalSpeed := ""
		if hasAltitude && hasPreviousAltitude && logicalIndex > previousAltitudeIndex {
			seconds := float64(logicalIndex-previousAltitudeIndex) * profile.SecondsPerSubframe
			if seconds > 0 {
				verticalSpeed = formatFREDFloat((altitude - previousAltitude) / seconds * 60)
			}
		}
		if hasAltitude {
			previousAltitude, previousAltitudeIndex, hasPreviousAltitude = altitude, logicalIndex, true
		}
		record = append(record, verticalSpeed)
		if err = writer.Write(record); err != nil {
			return result, fmt.Errorf("write canonical CSV row: %w", err)
		}
		rows++
		logicalIndex++
		position += span

		if rows%4096 == 0 {
			writer.Flush()
			if err = writer.Error(); err != nil {
				return result, fmt.Errorf("flush canonical CSV: %w", err)
			}
			info, statErr := output.Stat()
			if statErr != nil {
				return result, fmt.Errorf("measure canonical CSV: %w", statErr)
			}
			if info.Size() > MaxNormalizedBytes {
				return result, ErrTooLarge
			}
		}
	}
	writer.Flush()
	if err = writer.Error(); err != nil {
		return result, fmt.Errorf("flush canonical CSV: %w", err)
	}
	if rows == 0 {
		return result, errors.New("recording did not yield any canonical rows")
	}
	if err = output.Sync(); err != nil {
		return result, fmt.Errorf("sync canonical CSV: %w", err)
	}
	info, err := output.Stat()
	if err != nil {
		return result, fmt.Errorf("measure canonical CSV: %w", err)
	}
	if info.Size() > MaxNormalizedBytes {
		return result, ErrTooLarge
	}
	complete = true
	return CanonicalConversionResult{
		ProfileFormat: "fred", AircraftModel: profile.AircraftModel, FileRevision: profile.FileRevision,
		Recording: quality, Rows: rows, Columns: columns,
		SampleIntervalMs: int64(math.Round(profile.SecondsPerSubframe * 1000)),
		NormalizedBytes:  info.Size(), MissingCanonicalData: missing,
	}, nil
}

// FREDCanonicalParameters reports the canonical analysis columns a profile can
// populate. Vertical speed is derived from altitude by the canonical decoder.
func FREDCanonicalParameters(profileContent []byte) ([]string, error) {
	profile, err := CompileFREDProfile(profileContent)
	if err != nil {
		return nil, err
	}
	parameters := make([]string, 0, len(fredCanonicalDefinitions)+1)
	hasAltitude := false
	for _, definition := range fredCanonicalDefinitions {
		if findFREDParameter(profile.Parameters, definition.identities...) == nil {
			continue
		}
		parameters = append(parameters, definition.header)
		if definition.header == "Altitude" {
			hasAltitude = true
		}
	}
	if hasAltitude {
		parameters = append(parameters, "Vertical Speed")
	}
	return parameters, nil
}

func findFREDParameter(parameters []FREDParameter, identities ...string) *FREDParameter {
	for _, expected := range identities {
		for index := range parameters {
			if fredIdentity(parameters[index].Name) == expected || fredIdentity(parameters[index].Mnemonic) == expected {
				return &parameters[index]
			}
		}
	}
	return nil
}

func fredIdentity(value string) string {
	return strings.Map(func(character rune) rune {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			return character
		}
		if character >= 'A' && character <= 'Z' {
			return character + ('a' - 'A')
		}
		return -1
	}, value)
}

func profileSyncIndex(word uint16, syncWords []uint16) int {
	for index, sync := range syncWords {
		if word == sync {
			return index
		}
	}
	return -1
}

func decodeFREDParameter(reader wordReader, subframeStart int64, subframe int, parameter FREDParameter) (float64, bool) {
	var value float64
	found := false
	for _, sample := range parameter.Samples {
		raw, width, ok := decodeSample(reader, subframeStart, subframe, sample)
		if !ok {
			continue
		}
		candidate := applyFREDConversion(raw, width, parameter.Conversion)
		if math.IsNaN(candidate) || math.IsInf(candidate, 0) {
			continue
		}
		value, found = candidate, true
	}
	return value, found
}

func formatFREDFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func probeParameter(reader wordReader, detection FrameDetection, subframes int, parameter FREDParameter) ParameterProbe {
	probe := ParameterProbe{
		Mnemonic: parameter.Mnemonic,
		Name:     parameter.Name,
		Units:    parameter.Units,
		Minimum:  math.Inf(1),
		Maximum:  math.Inf(-1),
	}
	probe.Core, _, _ = coreParameterBounds(parameter)
	stride := 1
	if subframes > maxProbeSubframes {
		stride = int(math.Ceil(float64(subframes) / maxProbeSubframes))
	}
	span := int64(detection.WordsPerSecond) * reader.wordBits
	for subframeIndex := 0; subframeIndex < subframes; subframeIndex += stride {
		start := detection.BitOffset + int64(subframeIndex)*span
		if start+span > reader.total {
			break
		}
		sync := syncIndex(reader.at(start))
		if sync < 0 {
			continue
		}
		for _, sample := range parameter.Samples {
			raw, width, ok := decodeSample(reader, start, sync+1, sample)
			if !ok {
				continue
			}
			value := applyFREDConversion(raw, width, parameter.Conversion)
			if math.IsNaN(value) || math.IsInf(value, 0) {
				continue
			}
			probe.Samples++
			if value < probe.Minimum {
				probe.Minimum = value
			}
			if value > probe.Maximum {
				probe.Maximum = value
			}
		}
	}
	if probe.Samples == 0 {
		probe.Minimum, probe.Maximum = 0, 0
		return probe
	}
	probe.Varying = math.Abs(probe.Maximum-probe.Minimum) > 1e-9
	_, minimum, maximum := coreParameterBounds(parameter)
	probe.Plausible = !probe.Core || (probe.Minimum >= minimum && probe.Maximum <= maximum)
	return probe
}

func decodeSample(reader wordReader, subframeStart int64, subframe int, sample FREDSample) (uint64, int, bool) {
	var raw uint64
	width := 0
	for _, segment := range sample.Segments {
		if len(segment.Subframes) > 0 && !containsInt(segment.Subframes, subframe) {
			return 0, 0, false
		}
		segmentWidth := segment.HighBit - segment.LowBit + 1
		if width+segmentWidth > 63 {
			return 0, 0, false
		}
		wordPosition := subframeStart + int64(segment.Word-1)*reader.wordBits
		word := reader.at(wordPosition)
		mask := uint16((1 << segmentWidth) - 1)
		value := uint64((word >> (segment.LowBit - 1)) & mask)
		raw = raw<<segmentWidth | value
		width += segmentWidth
	}
	return raw, width, width > 0
}

func applyFREDConversion(raw uint64, width int, conversion FREDConversion) float64 {
	x := float64(raw)
	if len(conversion.Points) > 0 {
		points := conversion.Points
		if x <= points[0].Raw {
			return points[0].Value
		}
		for index := 1; index < len(points); index++ {
			if x <= points[index].Raw {
				left, right := points[index-1], points[index]
				if right.Raw == left.Raw {
					return right.Value
				}
				return left.Value + (x-left.Raw)/(right.Raw-left.Raw)*(right.Value-left.Value)
			}
		}
		return points[len(points)-1].Value
	}
	if conversion.Signed && width > 0 && raw >= uint64(1)<<(width-1) {
		return float64(int64(raw) - int64(uint64(1)<<width))
	}
	return x
}

func coreParameterBounds(parameter FREDParameter) (bool, float64, float64) {
	identity := strings.ToLower(parameter.Name + " " + parameter.Mnemonic)
	switch {
	case strings.Contains(identity, "pressure altitude") &&
		!strings.Contains(identity, "coarse") && !strings.Contains(identity, "fine"):
		return true, -5_000, 100_000
	case strings.Contains(identity, "computed airspeed") || strings.Contains(identity, "indicated airspeed"):
		return true, -10, 1_000
	case strings.Contains(identity, "pitch angle") || strings.Contains(identity, "roll angle"):
		return true, -360, 360
	default:
		return false, 0, 0
	}
}

func containsInt(values []int, expected int) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
