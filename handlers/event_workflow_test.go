package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"fdm-backend/models"
	"github.com/gin-gonic/gin"
)

func eventWorkflowDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE Company (id TEXT PRIMARY KEY);
		CREATE TABLE User (id TEXT PRIMARY KEY);
		CREATE TABLE Aircraft (id TEXT PRIMARY KEY, companyId TEXT, aircraftMake TEXT, modelNumber TEXT, parameters TEXT);
		INSERT INTO Company VALUES ('company-a'), ('company-b');
		INSERT INTO User VALUES ('gatekeeper-a'), ('gatekeeper-b');
		INSERT INTO Aircraft VALUES ('aircraft-a','company-a','Test','One','["VERTICAL ACCELERATION"]'),
		('aircraft-b','company-b','Test','One','["VERTICAL ACCELERATION"]');`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../database/migrations/005_event_definitions_v2.sql")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(migration), "CREATE TABLE EventDefinition (")
	end := strings.Index(string(migration), "CREATE TABLE Exceedance (")
	if start < 0 || end <= start {
		t.Fatal("event schema not found")
	}
	if _, err = db.Exec(string(migration)[start:end]); err != nil {
		t.Fatal(err)
	}
	return db
}

func workflowRequest(t *testing.T, handler *EventHandler, role, company, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userRole", role)
		c.Set("userCompanyId", company)
		c.Set("userId", "gatekeeper-a")
	})
	handler.RegisterRoutes(router.Group("/events"))
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func decodeWorkflowEvent(t *testing.T, response *httptest.ResponseRecorder, expected int) models.EventDefinitionResponse {
	t.Helper()
	if response.Code != expected {
		t.Fatalf("expected %d, got %d: %s", expected, response.Code, response.Body.String())
	}
	var event models.EventDefinitionResponse
	if err := json.Unmarshal(response.Body.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	return event
}

func TestGatekeeperEventWorkflowAndTenantIsolation(t *testing.T) {
	handler := NewEventHandler(eventWorkflowDB(t))
	payload := validDefinitionRequest()
	payload.Assignments[0].AircraftID = stringPointer("aircraft-a")
	request := func(role, company, method, path string, body any) *httptest.ResponseRecorder {
		return workflowRequest(t, handler, role, company, method, path, body)
	}
	created := decodeWorkflowEvent(t, request(models.RoleGatekeeper, "company-a", http.MethodPost, "/events", payload), http.StatusCreated)
	path := "/events/" + created.ID
	// All mutations remain unavailable to ordinary client users.
	for _, endpoint := range []struct{ method, suffix string }{{"PUT", ""}, {"POST", "/validate"}, {"POST", "/publish"}} {
		response := request(models.RoleUser, "company-a", endpoint.method, path+endpoint.suffix, payload)
		if response.Code != http.StatusForbidden {
			t.Fatalf("regular user reached %s: %d", endpoint.suffix, response.Code)
		}
	}
	// A second company's gatekeeper cannot change or release this definition.
	otherPayload := payload
	otherPayload.Assignments = []models.EventAssignmentInput{{ScopeType: "aircraft", AircraftID: stringPointer("aircraft-b")}}
	for _, endpoint := range []struct{ method, suffix string }{{"PUT", ""}, {"POST", "/validate"}, {"POST", "/publish"}} {
		response := request(models.RoleGatekeeper, "company-b", endpoint.method, path+endpoint.suffix, otherPayload)
		if response.Code != http.StatusForbidden {
			t.Fatalf("cross-company %s: %d %s", endpoint.suffix, response.Code, response.Body.String())
		}
	}
	payload.DisplayName = "Updated landing event"
	updated := decodeWorkflowEvent(t, request(models.RoleGatekeeper, "company-a", http.MethodPut, path, payload), http.StatusOK)
	if updated.DisplayName != payload.DisplayName {
		t.Fatal("gatekeeper edit was not saved")
	}
	// Publication requires the separate validation step.
	if response := request(models.RoleGatekeeper, "company-a", http.MethodPost, path+"/publish", nil); response.Code != http.StatusConflict {
		t.Fatalf("unvalidated publication returned %d", response.Code)
	}
	if response := request(models.RoleGatekeeper, "company-a", http.MethodPost, path+"/validate", nil); response.Code != http.StatusOK {
		t.Fatalf("validate: %d %s", response.Code, response.Body.String())
	}
	published := decodeWorkflowEvent(t, request(models.RoleGatekeeper, "company-a", http.MethodPost, path+"/publish", nil), http.StatusOK)
	if !published.IsActive || published.Status != models.EventVersionPublished {
		t.Fatal("publication did not activate definition")
	}
	// Editing a published definition preserves its published version until the replacement is validated and published.
	payload.DisplayName = "Replacement landing event"
	replacement := decodeWorkflowEvent(t, request(models.RoleGatekeeper, "company-a", http.MethodPut, path, payload), http.StatusOK)
	if replacement.Version != 2 || replacement.VersionID == published.VersionID {
		t.Fatal("published history was overwritten")
	}
	var status string
	if err := handler.db.QueryRow("SELECT status FROM EventDefinitionVersion WHERE id=?", published.VersionID).Scan(&status); err != nil || status != models.EventVersionPublished {
		t.Fatalf("previous version changed: %s %v", status, err)
	}
	if response := request(models.RoleGatekeeper, "company-a", http.MethodPost, path+"/validate", nil); response.Code != http.StatusOK {
		t.Fatalf("replacement validation: %s", response.Body.String())
	}
	decodeWorkflowEvent(t, request(models.RoleGatekeeper, "company-a", http.MethodPost, path+"/publish", nil), http.StatusOK)
	if err := handler.db.QueryRow("SELECT status FROM EventDefinitionVersion WHERE id=?", published.VersionID).Scan(&status); err != nil || status != models.EventVersionRetired {
		t.Fatalf("old version not retired: %s %v", status, err)
	}
}
