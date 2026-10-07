package ingestion

import (
	"encoding/csv"
	"io"
	"os"
	"path/filepath"
	"testing"
)

const syntheticFRED = `<?xml version="1.0"?>
<FREDFile xmlns="www.aviation-ia.com/aeec/SupportFiles/647a-1/">
  <FRED717>
    <Header>
      <File_Revision>TEST-64WPS</File_Revision>
      <Aircraft_Make_and_Model>Test Aircraft</Aircraft_Make_and_Model>
    </Header>
    <Subframe>
      <Bits_Per_Word>12</Bits_Per_Word>
      <Words_Per_Subframe>64</Words_Per_Subframe>
      <Seconds_Per_Subframe><Numerator>1</Numerator><Denominator>1</Denominator></Seconds_Per_Subframe>
      <Sync_Pattern>
        <Sync_Code>001001000111</Sync_Code><Sync_Code>010110111000</Sync_Code>
        <Sync_Code>101001000111</Sync_Code><Sync_Code>110110111000</Sync_Code>
      </Sync_Pattern>
    </Subframe>
    <Parameter717>
      <Name>Pressure Altitude</Name><Mnemonic_Code>ALT</Mnemonic_Code><Units>ft</Units>
      <Component>
        <Word_Numbers><Word_Num>2</Word_Num></Word_Numbers>
        <Subframe_Numbers><Subframe_Num>1</Subframe_Num><Subframe_Num>2</Subframe_Num><Subframe_Num>3</Subframe_Num><Subframe_Num>4</Subframe_Num></Subframe_Numbers>
        <Bits><OneBasedIntRange_Start>1</OneBasedIntRange_Start><OneBasedIntRange_End>12</OneBasedIntRange_End></Bits>
      </Component>
      <Range>
        <Data_Type>Unsigned Binary</Data_Type>
        <Conversion_Step><Integer_Real_Table>
          <Integer_Real_Pair index="0">0</Integer_Real_Pair>
          <Integer_Real_Pair index="4095">4095</Integer_Real_Pair>
        </Integer_Real_Table></Conversion_Step>
      </Range>
    </Parameter717>
    <Parameter717>
      <Name>Computed Airspeed</Name><Mnemonic_Code>IAS</Mnemonic_Code><Units>kt</Units>
      <Component>
        <Word_Numbers><Word_Num>3</Word_Num></Word_Numbers>
        <Subframe_Numbers><Subframe_Num>1</Subframe_Num><Subframe_Num>2</Subframe_Num><Subframe_Num>3</Subframe_Num><Subframe_Num>4</Subframe_Num></Subframe_Numbers>
        <Bits><OneBasedIntRange_Start>1</OneBasedIntRange_Start><OneBasedIntRange_End>12</OneBasedIntRange_End></Bits>
      </Component>
      <Range>
        <Data_Type>Unsigned Binary</Data_Type>
        <Conversion_Step><Integer_Real_Table>
          <Integer_Real_Pair index="0">0</Integer_Real_Pair>
          <Integer_Real_Pair index="4095">4095</Integer_Real_Pair>
        </Integer_Real_Table></Conversion_Step>
      </Range>
    </Parameter717>
  </FRED717>
</FREDFile>`

func TestCompileFREDProfile(t *testing.T) {
	profile, err := CompileFREDProfile([]byte(syntheticFRED))
	if err != nil {
		t.Fatal(err)
	}
	if profile.WordsPerSecond != 64 || len(profile.Parameters) != 2 {
		t.Fatalf("unexpected profile: %+v", profile)
	}
	if profile.Parameters[0].Samples[0].Segments[0].Word != 2 {
		t.Fatalf("unexpected compiled parameter: %+v", profile.Parameters[0])
	}
}

func TestFREDCanonicalParametersIncludesDerivedVerticalSpeed(t *testing.T) {
	parameters, err := FREDCanonicalParameters([]byte(syntheticFRED))
	if err != nil {
		t.Fatal(err)
	}
	wanted := map[string]bool{"Airspeed": false, "Altitude": false, "Vertical Speed": false}
	for _, parameter := range parameters {
		if _, ok := wanted[parameter]; ok {
			wanted[parameter] = true
		}
	}
	for parameter, found := range wanted {
		if !found {
			t.Fatalf("expected canonical parameter %s in %#v", parameter, parameters)
		}
	}
}

func TestValidateFREDRecording(t *testing.T) {
	words := make([]uint16, 80*64)
	for subframe := 0; subframe < 80; subframe++ {
		words[subframe*64] = SyncWords[subframe%4]
		words[subframe*64+1] = uint16(1000 + subframe)
		words[subframe*64+2] = uint16(150 + subframe)
	}
	report, err := ValidateFREDRecording([]byte(syntheticFRED), packWords12LSB(words))
	if err != nil {
		t.Fatal(err)
	}
	if report.ParametersDecoded != 2 || report.ParametersVarying != 2 || report.Recording.InSyncPct != 100 {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestDecodeFREDToCanonicalCSV(t *testing.T) {
	words := make([]uint16, 80*64)
	for subframe := 0; subframe < 80; subframe++ {
		words[subframe*64] = SyncWords[subframe%4]
		words[subframe*64+1] = uint16(1000 + subframe*10)
		words[subframe*64+2] = uint16(150 + subframe)
	}
	destination := filepath.Join(t.TempDir(), "canonical.csv")
	result, err := DecodeFREDToCanonicalCSV([]byte(syntheticFRED), packWords12LSB(words), destination)
	if err != nil {
		t.Fatal(err)
	}
	if result.Rows != 80 || result.SampleIntervalMs != 1000 || result.NormalizedBytes <= 0 {
		t.Fatalf("unexpected conversion result: %+v", result)
	}
	file, err := os.Open(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	header, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(header) < 4 || header[2] != "Airspeed" || header[3] != "Altitude" || header[len(header)-1] != "Vertical Speed" {
		t.Fatalf("unexpected canonical header: %#v", header)
	}
	first, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	second, err := reader.Read()
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if first[2] != "150" || first[3] != "1000" || second[len(second)-1] != "600" {
		t.Fatalf("unexpected canonical values: first=%#v second=%#v", first, second)
	}
}

func TestBeaconCRJFixture(t *testing.T) {
	directory := os.Getenv("BEACON_FLIGHTDATA_DIR")
	if directory == "" {
		t.Skip("BEACON_FLIGHTDATA_DIR is not set")
	}
	profile, err := os.ReadFile(filepath.Join(directory, "CRJ100_200_440-SL-31-014_128WPS_REV00_D_FRED.xml"))
	if err != nil {
		t.Fatal(err)
	}
	recording, err := os.ReadFile(filepath.Join(directory, "5Y-DRM080926.ddf"))
	if err != nil {
		t.Fatal(err)
	}
	report, err := ValidateFREDRecording(profile, recording)
	if err != nil {
		t.Fatalf("fixture validation failed with partial report %+v: %v", report, err)
	}
	if report.ProfileWordsPerSecond != 128 || report.Recording.InSyncPct < 99.9 {
		t.Fatalf("unexpected fixture report: %+v", report)
	}
	if report.ParametersDecoded < 400 {
		t.Fatalf("expected at least 400 decoded parameters, got %d", report.ParametersDecoded)
	}
	t.Logf("%s", report.Message)
}

func TestBeaconDHC8DLUFixture(t *testing.T) {
	directory := os.Getenv("BEACON_FLIGHTDATA_DIR")
	if directory == "" {
		t.Skip("BEACON_FLIGHTDATA_DIR is not set")
	}
	profile, err := os.ReadFile(filepath.Join(directory, "DHC8FDR_frame.xml"))
	if err != nil {
		t.Fatal(err)
	}
	recording, err := os.ReadFile(filepath.Join(directory, "5Y-SKO050626.dlu"))
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareRecorderPayload("5Y-SKO050626.dlu", recording)
	if err != nil {
		t.Fatal(err)
	}
	report, err := ValidateFREDRecording(profile, prepared.Payload)
	if err != nil {
		t.Fatalf("fixture validation failed with adapter %s and partial report %+v: %v", prepared.Adapter, report, err)
	}
	if report.ProfileWordsPerSecond != 64 || report.Recording.InSyncPct < 95 {
		t.Fatalf("unexpected fixture report: %+v", report)
	}
	if report.ParametersDecoded == 0 {
		t.Fatal("expected at least one decoded parameter")
	}
	t.Logf("adapter=%s format=%s: %s", prepared.Adapter, prepared.ContainerFormat, report.Message)
}

func TestBeaconCRJCanonicalFixture(t *testing.T) {
	directory := os.Getenv("BEACON_FLIGHTDATA_DIR")
	if directory == "" {
		t.Skip("BEACON_FLIGHTDATA_DIR is not set")
	}
	profile, err := os.ReadFile(filepath.Join(directory, "CRJ100_200_440-SL-31-014_128WPS_REV00_D_FRED.xml"))
	if err != nil {
		t.Fatal(err)
	}
	recording, err := os.ReadFile(filepath.Join(directory, "5Y-DRM080926.ddf"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := DecodeFREDToCanonicalCSV(profile, recording, filepath.Join(t.TempDir(), "beacon-canonical.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Rows < 260_000 || result.Recording.InSyncPct < 99.9 || result.NormalizedBytes <= 0 {
		t.Fatalf("unexpected fixture conversion: %+v", result)
	}
	t.Logf("decoded %d rows and %d columns into %d bytes", result.Rows, len(result.Columns), result.NormalizedBytes)
}
