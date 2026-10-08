package handlers

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
	"testing"
)

func TestCustomerAnalysisDoesNotExposeUnvalidatedEvidence(t *testing.T) {
	original := `{"status":"completed_with_warnings","occurrenceCount":3,"occurrences":[{"value":99}],"diagnostics":[{"message":"Missing IAS"}],"flights":[{"occurrenceCount":2}]}`
	summary := customerAnalysisSummary(&original)
	if summary == nil || strings.Contains(*summary, "occurrence") || !strings.Contains(*summary, "Missing IAS") {
		t.Fatalf("unsafe customer summary: %v", summary)
	}
	if !strings.Contains(original, "occurrenceCount") {
		t.Fatal("persisted evidence was modified")
	}
	invalid := "not JSON"
	if customerAnalysisSummary(&invalid) != nil {
		t.Fatal("invalid evidence should fail closed")
	}
}

func TestNestedExceedancesRespectValidation(t *testing.T) {
	db := reportTestDB(t)
	for _, statement := range []string{
		`ALTER TABLE Exceedance ADD COLUMN description TEXT`,
		`ALTER TABLE Exceedance ADD COLUMN file TEXT`,
		`ALTER TABLE Exceedance ADD COLUMN comment TEXT`,
		`ALTER TABLE Exceedance ADD COLUMN updatedAt INTEGER`,
		`CREATE TABLE EventLog (id TEXT PRIMARY KEY, eventName TEXT, displayName TEXT, eventCode TEXT, eventDescription TEXT, eventParameter TEXT, eventTrigger TEXT, eventType TEXT, flightPhase TEXT)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	csv := NewCSVHandler(db)
	aircraft := NewAircraftHandler(db)
	for _, oversight := range []bool{false, true} {
		want := 1
		if oversight {
			want = 2
		}
		flights, err := csv.getCSVExceedances("flight-a", oversight)
		if err != nil || len(flights) != want {
			t.Fatalf("flight visibility oversight=%v: got %d, error=%v", oversight, len(flights), err)
		}
		items, err := aircraft.getAircraftExceedances("aircraft-a", oversight)
		if err != nil || len(items) != want {
			t.Fatalf("aircraft visibility oversight=%v: got %d, error=%v", oversight, len(items), err)
		}
	}
}

func TestCustomerNotificationsAreValidatedAndCompanyScoped(t *testing.T) {
	db := reportTestDB(t)
	_, err := db.Exec(`CREATE TABLE Csv (id TEXT PRIMARY KEY);
 INSERT INTO Csv VALUES ('recording-a');
 CREATE TABLE Notification (id TEXT, userId TEXT, exceedanceId TEXT, recordingId TEXT, message TEXT, level TEXT, isRead INTEGER, createdAt INTEGER, updatedAt INTEGER);
 INSERT INTO Notification VALUES
 ('valid','user-a','event-a-valid',NULL,'Validated event','High',0,1,1),
 ('pending','user-a','event-a-pending',NULL,'Unvalidated event','Critical',0,1,1),
 ('foreign','user-a','event-b-valid',NULL,'Other company','Low',0,1,1),
 ('upload','user-a',NULL,'recording-a','Customer upload','Review',0,1,1)`)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"user", "gatekeeper", "admin", "fda"} {
		ctx, recorder := reportContext(http.MethodGet, "/notifications/user-a", role, "company-a")
		ctx.Set("userId", "user-a")
		ctx.Params = gin.Params{{Key: "userId", Value: "user-a"}}
		NewNotificationHandler(db).GetUserNotifications(ctx)
		var items []map[string]any
		if recorder.Code != 200 || json.Unmarshal(recorder.Body.Bytes(), &items) != nil {
			t.Fatalf("notification request failed: %s", recorder.Body.String())
		}
		want := 1
		if role == "admin" || role == "fda" {
			want = 4
		}
		if len(items) != want {
			t.Fatalf("%s saw %d notifications, want %d", role, len(items), want)
		}
		if role == "admin" || role == "fda" {
			found := false
			for _, item := range items {
				if item["recordingId"] == "recording-a" {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s did not receive the customer upload notification", role)
			}
		}
	}
}
