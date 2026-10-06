package handlers

import (
	"bytes"
	"database/sql"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"fdm-backend/models"
	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

const handlerFRED = `<?xml version="1.0"?><FREDFile xmlns="www.aviation-ia.com/aeec/SupportFiles/647a-1/"><FRED717>
<Header><File_Revision>TEST</File_Revision><Aircraft_Make_and_Model>Test Aircraft</Aircraft_Make_and_Model></Header>
<Subframe><Bits_Per_Word>12</Bits_Per_Word><Words_Per_Subframe>64</Words_Per_Subframe>
<Seconds_Per_Subframe><Numerator>1</Numerator><Denominator>1</Denominator></Seconds_Per_Subframe>
<Sync_Pattern><Sync_Code>001001000111</Sync_Code><Sync_Code>010110111000</Sync_Code><Sync_Code>101001000111</Sync_Code><Sync_Code>110110111000</Sync_Code></Sync_Pattern></Subframe>
<Parameter717><Name>Pressure Altitude</Name><Mnemonic_Code>ALT</Mnemonic_Code><Units>ft</Units>
<Component><Word_Numbers><Word_Num>2</Word_Num></Word_Numbers><Subframe_Numbers><Subframe_Num>1</Subframe_Num><Subframe_Num>2</Subframe_Num><Subframe_Num>3</Subframe_Num><Subframe_Num>4</Subframe_Num></Subframe_Numbers><Bits><OneBasedIntRange_Start>1</OneBasedIntRange_Start><OneBasedIntRange_End>12</OneBasedIntRange_End></Bits></Component>
<Range><Data_Type>Unsigned Binary</Data_Type><Conversion_Step><Integer_Real_Table><Integer_Real_Pair index="0">0</Integer_Real_Pair><Integer_Real_Pair index="4095">4095</Integer_Real_Pair></Integer_Real_Table></Conversion_Step></Range>
</Parameter717></FRED717></FREDFile>`

func decoderProfileTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`CREATE TABLE Aircraft (id TEXT PRIMARY KEY, companyId TEXT NOT NULL);
		CREATE TABLE AircraftDecoderProfile (
		 id TEXT PRIMARY KEY, aircraftId TEXT, version INTEGER, name TEXT,
		 parameterFormat TEXT, parameterFileName TEXT, parameterText TEXT,
		 decoderConfig TEXT, checksum TEXT, notes TEXT, status TEXT,
		 validationStatus TEXT, validationSummary TEXT,
		 validationRecordingName TEXT, validationRecordingChecksum TEXT,
		 validatedBy TEXT, validatedAt INTEGER, createdBy TEXT,
		 publishedBy TEXT, createdAt INTEGER, publishedAt INTEGER
		);
		INSERT INTO Aircraft VALUES ('aircraft-b', 'company-b');
		INSERT INTO AircraftDecoderProfile
		 (id, aircraftId, version, name, parameterFormat, parameterFileName,
		  parameterText, decoderConfig, checksum, notes, status, validationStatus,
		  validationSummary, createdAt) VALUES
		 ('profile-b', 'aircraft-b', 1, 'B profile', 'tbx', 'b.tbx', '[PARAMETER B]',
		  '{}', 'hash', 'initial', 'draft', 'failed', '{}', 1);`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func decoderProfileContext(aircraftID, role, companyID string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/aircrafts/"+aircraftID+"/decoder-profiles", nil)
	context.Params = gin.Params{{Key: "id", Value: aircraftID}}
	context.Set("userRole", role)
	context.Set("userCompanyId", companyID)
	return context, recorder
}

func TestDecoderProfilesRejectCrossCompanyGatekeeper(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, recorder := decoderProfileContext("aircraft-b", models.RoleGatekeeper, "company-a")
	NewDecoderProfileHandler(decoderProfileTestDB(t)).List(context)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden response, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestDecoderProfilesAllowGlobalFDA(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, recorder := decoderProfileContext("aircraft-b", models.RoleFDA, "")
	NewDecoderProfileHandler(decoderProfileTestDB(t)).List(context)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected success response, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestDecoderProfilesValidateControlledRecording(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := decoderProfileTestDB(t)
	if _, err := db.Exec(`UPDATE AircraftDecoderProfile SET parameterFormat = 'fred',
		parameterFileName = 'test.fred', parameterText = ? WHERE id = 'profile-b'`, handlerFRED); err != nil {
		t.Fatal(err)
	}
	words := make([]uint16, 80*64)
	syncs := []uint16{0o1107, 0o2670, 0o5107, 0o6670}
	for subframe := 0; subframe < 80; subframe++ {
		words[subframe*64] = syncs[subframe%4]
		words[subframe*64+1] = uint16(1000 + subframe)
	}
	recording := packTestWords(words)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("recording", "controlled.ddf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(recording); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/decoder-profiles/profile-b/validate", &body)
	context.Request.Header.Set("Content-Type", writer.FormDataContentType())
	context.Params = gin.Params{{Key: "profileId", Value: "profile-b"}}
	context.Set("userRole", models.RoleGatekeeper)
	context.Set("userCompanyId", "company-b")
	context.Set("userId", "gatekeeper-b")

	NewDecoderProfileHandler(db).ValidateRecording(context)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected successful validation, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var status, recordingName string
	if err = db.QueryRow(`SELECT validationStatus, validationRecordingName
		FROM AircraftDecoderProfile WHERE id = 'profile-b'`).Scan(&status, &recordingName); err != nil {
		t.Fatal(err)
	}
	if status != "passed" || recordingName != "controlled.ddf" {
		t.Fatalf("unexpected validation evidence: status=%s recording=%s", status, recordingName)
	}
}

func packTestWords(words []uint16) []byte {
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
