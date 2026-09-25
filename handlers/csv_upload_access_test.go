package handlers

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"fdm-backend/models"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

func uploadRequest(t *testing.T, aircraftID string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "logger.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(content); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"name": "Logger recording", "aircraftId": aircraftID, "sampleIntervalMs": "1000",
	} {
		if err = writer.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/csv", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func uploadTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`CREATE TABLE Aircraft (
		id TEXT PRIMARY KEY, airline TEXT NOT NULL, aircraftMake TEXT NOT NULL,
		modelNumber TEXT, serialNumber TEXT NOT NULL, registration TEXT,
		companyId TEXT NOT NULL, parameters TEXT
	);
	CREATE TABLE Csv (id TEXT PRIMARY KEY, aircraftId TEXT NOT NULL, contentHash TEXT);`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func insertUploadTestAircraft(t *testing.T, db *sql.DB, id, companyID string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO Aircraft
		(id, airline, aircraftMake, modelNumber, serialNumber, registration, companyId, parameters)
		VALUES (?, 'Test Air', 'Cessna', '208B', 'serial-1', '5Y-TEST', ?, 'IAS,AltInd')`, id, companyID)
	if err != nil {
		t.Fatal(err)
	}
}

func uploadContext(request *http.Request, companyID string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	context.Set("userRole", models.RoleUser)
	context.Set("userCompanyId", companyID)
	context.Set("userId", "client-user")
	return context, recorder
}

func TestClientUploadRejectsAircraftFromAnotherCompany(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := uploadTestDatabase(t)
	insertUploadTestAircraft(t, db, "aircraft-b", "company-b")
	context, recorder := uploadContext(uploadRequest(t, "aircraft-b", []byte("Time,IAS\n00:00:00,100\n")), "company-a")

	NewCSVHandler(db).UploadCSV(context)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected cross-company upload to be forbidden, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestClientUploadRejectsDuplicateForOwnedAircraft(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := uploadTestDatabase(t)
	insertUploadTestAircraft(t, db, "aircraft-a", "company-a")
	content := []byte("Time,IAS\n00:00:00,100\n")
	sum := sha256.Sum256(content)
	_, err := db.Exec(`INSERT INTO Csv (id, aircraftId, contentHash) VALUES ('existing-recording', ?, ?)`,
		"aircraft-a", hex.EncodeToString(sum[:]))
	if err != nil {
		t.Fatal(err)
	}
	context, recorder := uploadContext(uploadRequest(t, "aircraft-a", content), "company-a")

	NewCSVHandler(db).UploadCSV(context)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected duplicate upload conflict, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":"DUPLICATE_RECORDING"`)) {
		t.Fatalf("expected stable duplicate error code, got %s", recorder.Body.String())
	}
}
