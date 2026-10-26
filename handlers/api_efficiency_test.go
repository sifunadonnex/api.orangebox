package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"fdm-backend/models"
	"github.com/gin-gonic/gin"
)

func efficiencyTestDB(t *testing.T) *sql.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := reportTestDB(t) // Intentionally only one available connection.
	columns := map[string][]string{
		"Company":    {"email TEXT DEFAULT 'test@example.com'", "phone TEXT", "address TEXT", "country TEXT", "logo TEXT", "subscriptionId TEXT", "createdAt INTEGER DEFAULT 1", "updatedAt INTEGER DEFAULT 1"},
		"Aircraft":   {"airline TEXT DEFAULT 'Test Air'", "parameters TEXT", "createdAt INTEGER DEFAULT 1", "updatedAt INTEGER DEFAULT 1"},
		"FlightLeg":  {"recordingId TEXT DEFAULT 'recording-a'", "flightDate TEXT", "flightNumber TEXT", "picCrewCode TEXT", "foCrewCode TEXT", "techLogReference TEXT", "loadSheetNumber TEXT", "takeoffWeight REAL", "landingWeight REAL", "weightUnit TEXT", "v1 REAL", "vr REAL", "v2 REAL", "vref REAL", "vapp REAL", "notes TEXT", "pilot TEXT", "analysisSummary TEXT", "legIndex INTEGER DEFAULT 1", "startRow INTEGER DEFAULT 2", "endRow INTEGER", "startSample TEXT", "endSample TEXT", "boundarySource TEXT DEFAULT 'manual'", "updatedAt INTEGER DEFAULT 1"},
		"Exceedance": {"description TEXT", "file TEXT DEFAULT 'source.csv'", "comment TEXT", "updatedAt INTEGER DEFAULT 1", "detectionRunId TEXT", "startTimeMs INTEGER", "endTimeMs INTEGER", "durationMs INTEGER", "ruleHash TEXT"},
	}
	for table, definitions := range columns {
		for _, definition := range definitions {
			if _, err := db.Exec("ALTER TABLE " + table + " ADD COLUMN " + definition); err != nil {
				t.Fatal(err)
			}
		}
	}
	_, err := db.Exec(`CREATE TABLE Csv (id TEXT PRIMARY KEY, file TEXT, sampleIntervalMs INTEGER);
		INSERT INTO Csv VALUES ('recording-a', 'unused.csv', 1000);
		CREATE TABLE EventLog (id TEXT PRIMARY KEY, eventName TEXT, displayName TEXT, eventCode TEXT, eventDescription TEXT, eventParameter TEXT, eventTrigger TEXT, eventType TEXT, flightPhase TEXT, high REAL, high1 REAL, high2 REAL, low REAL, low1 REAL, low2 REAL, triggerType TEXT, detectionPeriod REAL, severities TEXT, sop TEXT, aircraftId TEXT, createdAt INTEGER, updatedAt INTEGER);
		CREATE TABLE FlightPhaseRun (recordingId TEXT, flightIndex INTEGER, flightStatus TEXT, phase TEXT, startRow INTEGER, endRow INTEGER, startSample TEXT, endSample TEXT, durationMs INTEGER);`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestFlightListSingleConnectionPreservesScopeAndWireContract(t *testing.T) {
	db := efficiencyTestDB(t)
	handler := NewCSVHandler(db)
	for _, test := range []struct {
		role, company   string
		flights, events int
	}{
		{models.RoleAdmin, "", 2, 3}, {models.RoleUser, "company-a", 1, 1},
		{models.RoleGatekeeper, "company-a", 1, 1}, {models.RoleUser, "company-b", 1, 1},
	} {
		ctx, response := reportContext(http.MethodGet, "/api/csv", test.role, test.company)
		deadline, cancel := context.WithTimeout(ctx.Request.Context(), 2*time.Second)
		ctx.Request = ctx.Request.WithContext(deadline)
		handler.GetCSVs(ctx)
		cancel()
		if response.Code != 200 {
			t.Fatalf("%s: %d %s", test.role, response.Code, response.Body.String())
		}
		var flights []listedFlight
		if err := json.Unmarshal(response.Body.Bytes(), &flights); err != nil {
			t.Fatal(err)
		}
		if len(flights) != test.flights {
			t.Fatalf("%s saw %d flights", test.role, len(flights))
		}
		events := 0
		for _, flight := range flights {
			if flight.Aircraft == nil || flight.Exceedance == nil {
				t.Fatalf("lost nested response fields: %+v", flight)
			}
			for _, occurrence := range flight.Exceedance {
				if occurrence.FlightID != flight.ID {
					t.Fatal("exceedance assigned to wrong flight")
				}
				if test.company != "" && occurrence.EventStatus != "Valid" {
					t.Fatal("unvalidated evidence leaked")
				}
			}
			events += len(flight.Exceedance)
		}
		if events != test.events {
			t.Fatalf("%s saw %d events, want %d", test.role, events, test.events)
		}
	}
	// Simultaneous list requests must also finish with a one-connection pool.
	var wait sync.WaitGroup
	for index := 0; index < 12; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ctx, response := reportContext(http.MethodGet, "/api/csv", models.RoleAdmin, "")
			deadline, cancel := context.WithTimeout(ctx.Request.Context(), 3*time.Second)
			defer cancel()
			ctx.Request = ctx.Request.WithContext(deadline)
			handler.GetCSVs(ctx)
			if response.Code != 200 {
				t.Errorf("concurrent list: %d %s", response.Code, response.Body.String())
			}
		}()
	}
	wait.Wait()
}

func TestFlightExceedanceBatchesPreserveLegacyIDsAndValidation(t *testing.T) {
	db := efficiencyTestDB(t)
	if _, err := db.Exec(`UPDATE Exceedance SET flightLegId = NULL WHERE id = 'event-a-valid'`); err != nil {
		t.Fatal(err)
	}
	ids := []string{"flight-a", "flight-b"}
	for index := 0; index < 805; index++ {
		ids = append(ids, fmt.Sprintf("missing-%d", index))
	}
	items, err := NewCSVHandler(db).getFlightsExceedances(context.Background(), ids, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items["flight-a"]) != 1 || len(items["flight-b"]) != 1 {
		t.Fatalf("legacy/current visibility mismatch: %+v", items)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = NewCSVHandler(db).getFlightsExceedances(ctx, ids, true); err == nil {
		t.Fatal("canceled batch continued")
	}
}

func TestFlightListOptionalPagination(t *testing.T) {
	handler := NewCSVHandler(efficiencyTestDB(t))
	for _, test := range []struct {
		query, id string
		status    int
	}{
		{"?limit=1", "flight-b", 200}, {"?limit=1&offset=1", "flight-a", 200},
		{"?limit=1&offset=2", "", 200}, {"?offset=1", "", 400},
		{"?limit=0", "", 400}, {"?limit=501", "", 400}, {"?limit=1&offset=-1", "", 400},
	} {
		ctx, response := reportContext(http.MethodGet, "/api/csv"+test.query, models.RoleAdmin, "")
		handler.GetCSVs(ctx)
		if response.Code != test.status {
			t.Fatalf("%s: %d %s", test.query, response.Code, response.Body.String())
		}
		if test.status != 200 {
			continue
		}
		var flights []listedFlight
		if err := json.Unmarshal(response.Body.Bytes(), &flights); err != nil {
			t.Fatal(err)
		}
		if test.id == "" {
			if len(flights) != 0 {
				t.Fatal("offset returned extra flights")
			}
			continue
		}
		if len(flights) != 1 || flights[0].ID != test.id {
			t.Fatalf("wrong page: %s", response.Body.String())
		}
	}
}

func TestCachedReplayStillChecksTenantAndInvalidatesPhaseEdits(t *testing.T) {
	db := efficiencyTestDB(t)
	if err := os.MkdirAll("csvs", 0755); err != nil {
		t.Fatal(err)
	}
	source, err := os.CreateTemp("csvs", "efficiency-*.csv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(source.Name()) })
	_, err = source.WriteString("Time(sec),Latitude,Longitude\n0,-1.3,36.8\n1,-1.2,36.7\n")
	closeErr := source.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("write source: %v %v", err, closeErr)
	}
	if _, err = db.Exec(`UPDATE Csv SET file = ?`, filepath.Base(source.Name())); err != nil {
		t.Fatal(err)
	}
	handler := NewCSVHandler(db)
	request := func(company string) (int, string) {
		ctx, response := reportContext(http.MethodGet, "/api/csv/flight-a/replay", models.RoleUser, company)
		ctx.Params = gin.Params{{Key: "id", Value: "flight-a"}}
		handler.GetFlightReplay(ctx)
		return response.Code, response.Body.String()
	}
	status, first := request("company-a")
	if status != 200 || !strings.Contains(first, `"supported":true`) {
		t.Fatalf("replay: %d %s", status, first)
	}
	// A hit is served even when every expensive-work slot is occupied.
	for tryRecordingWork(handler.work) {
	}
	status, second := request("company-a")
	if status != 200 || second != first {
		t.Fatalf("cache hit rebuilt replay: %d %s", status, second)
	}
	status, _ = request("company-b")
	if status != 403 {
		t.Fatalf("cache bypassed tenant check: %d", status)
	}
	for len(handler.work) > 0 {
		releaseRecordingWork(handler.work)
	}
	_, err = db.Exec(`INSERT INTO FlightPhaseRun VALUES ('recording-a',1,'airborne','CLIMB',2,3,'','',1000)`)
	if err != nil {
		t.Fatal(err)
	}
	status, updated := request("company-a")
	if status != 200 || updated == first || !strings.Contains(updated, `"phase":"detected:`) {
		t.Fatalf("stale phase response: %d %s", status, updated)
	}
	if _, err = db.Exec(`UPDATE Aircraft SET companyId = 'company-b' WHERE id = 'aircraft-a'`); err != nil {
		t.Fatal(err)
	}
	status, _ = request("company-a")
	if status != 403 {
		t.Fatalf("cached replay ignored changed ownership: %d", status)
	}
}

func TestRecordingOverloadReturnsRetryableErrorBeforeWork(t *testing.T) {
	handler := NewCSVHandler(nil)
	for tryRecordingWork(handler.work) {
	}
	for _, endpoint := range []func(*gin.Context){handler.UploadCSV, handler.ReanalyzeCSV, handler.BackfillEvent} {
		ctx, response := reportContext(http.MethodPost, "/", models.RoleAdmin, "")
		endpoint(ctx)
		if response.Code != 503 || response.Header().Get("Retry-After") != "2" || !strings.Contains(response.Body.String(), "RECORDING_BUSY") {
			t.Fatalf("unexpected overload response: %d %s", response.Code, response.Body.String())
		}
	}
	releaseRecordingWork(handler.work)
	ctx, response := reportContext(http.MethodPost, "/", models.RoleAdmin, "")
	handler.UploadCSV(ctx) // Invalid input should release its acquired slot.
	if response.Code != 400 || len(handler.work) != cap(handler.work)-1 {
		t.Fatal("early return leaked work slot")
	}
}

func TestExceedanceListPaginationPreservesTenantFiltering(t *testing.T) {
	handler := NewExceedanceHandler(efficiencyTestDB(t))
	request := func(query, role, company string) []map[string]any {
		t.Helper()
		ctx, response := reportContext(http.MethodGet, "/api/exceedances"+query, role, company)
		handler.GetExceedances(ctx)
		if response.Code != 200 {
			t.Fatalf("exceedance list: %d %s", response.Code, response.Body.String())
		}
		var items []map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
			t.Fatal(err)
		}
		return items
	}
	all := request("", models.RoleAdmin, "")
	page := request("?limit=1&offset=1", models.RoleAdmin, "")
	if len(all) != 3 || len(page) != 1 || page[0]["id"] != all[1]["id"] {
		t.Fatalf("unstable exceedance page: %+v %+v", all, page)
	}
	items := request("?limit=1", models.RoleUser, "company-a")
	if len(items) != 1 || items[0]["id"] != "event-a-valid" {
		t.Fatalf("paged exceedances bypassed visibility: %+v", items)
	}
	if items := request("?limit=1&offset=1", models.RoleUser, "company-a"); len(items) != 0 {
		t.Fatalf("offset bypassed tenant scope: %+v", items)
	}
}
