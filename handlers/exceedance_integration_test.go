package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"fdm-backend/models"
	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

// This test is opt-in because it validates the handlers against a migrated
// database copy without ever opening the live database for writes.
func TestExceedanceListContractAgainstDatabaseCopy(t *testing.T) {
	databasePath := os.Getenv("FDM_EXCEEDANCE_TEST_DB")
	if databasePath == "" {
		t.Skip("FDM_EXCEEDANCE_TEST_DB is not set")
	}

	db, err := sql.Open("sqlite", "file:"+databasePath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("userRole", models.RoleAdmin)

	NewExceedanceHandler(db).GetExceedances(context)
	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var response []map[string]interface{}
	if err = json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response) == 0 {
		t.Fatal("expected the copied database to contain captured occurrences")
	}
	if response[0]["startTimeMs"] == nil || response[0]["durationMs"] == nil || response[0]["peakValue"] == nil {
		t.Fatalf("structured occurrence fields are missing: %v", response[0])
	}
	aircraft, ok := response[0]["aircraft"].(map[string]interface{})
	if !ok || aircraft["registration"] == nil {
		t.Fatalf("aircraft registration is missing: %v", response[0]["aircraft"])
	}

	occurrenceID, ok := response[0]["id"].(string)
	if !ok || occurrenceID == "" {
		t.Fatalf("occurrence id is missing: %v", response[0]["id"])
	}
	detailRecorder := httptest.NewRecorder()
	detailContext, _ := gin.CreateTestContext(detailRecorder)
	detailContext.Params = gin.Params{{Key: "id", Value: occurrenceID}}
	detailContext.Set("userRole", models.RoleAdmin)

	NewExceedanceHandler(db).GetExceedanceByID(detailContext)
	if detailRecorder.Code != 200 {
		t.Fatalf("expected detail 200, got %d: %s", detailRecorder.Code, detailRecorder.Body.String())
	}
	var detail map[string]interface{}
	if err = json.Unmarshal(detailRecorder.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail["startTimeMs"] == nil || detail["reviews"] == nil {
		t.Fatalf("detail contract is incomplete: %v", detail)
	}
}
