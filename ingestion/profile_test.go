package ingestion

import "testing"

func TestValidateParameterProfileTBX(t *testing.T) {
	content := []byte("[FILE]\nFormat version = 1\n\n[PARAMETER: ALT]\nName = Pressure Altitude\nWord = 3\n\n[PARAMETER IAS]\nName = Airspeed\nWord = 4\n")
	summary, err := ValidateParameterProfile("aircraft.tbx", content)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Format != "tbx" || summary.ParameterCount != 2 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestValidateParameterProfileJSON(t *testing.T) {
	summary, err := ValidateParameterProfile("aircraft.json", []byte(`{"params":[{"mn":"ALT"},{"mn":"IAS"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if summary.Format != "json" || summary.ParameterCount != 2 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestValidateParameterProfileRejectsEmptyMap(t *testing.T) {
	if _, err := ValidateParameterProfile("aircraft.json", []byte(`{"params":[]}`)); err == nil {
		t.Fatal("expected an empty parameter map to be rejected")
	}
}
