package handlers

import (
	"bytes"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"fdm-backend/models"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

func tenantTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	statements := []string{
		`CREATE TABLE Aircraft (id TEXT PRIMARY KEY, companyId TEXT NOT NULL)`,
		`CREATE TABLE FlightLeg (id TEXT PRIMARY KEY, aircraftId TEXT NOT NULL)`,
		`CREATE TABLE Exceedance (id TEXT PRIMARY KEY, aircraftId TEXT NOT NULL, eventStatus TEXT, isCurrent INTEGER, supersededAt INTEGER, updatedAt INTEGER)`,
		`CREATE TABLE Notification (id TEXT PRIMARY KEY, userId TEXT NOT NULL, isRead INTEGER, updatedAt INTEGER)`,
		`CREATE TABLE ExceedanceReview (id TEXT PRIMARY KEY, exceedanceId TEXT, action TEXT, previousStatus TEXT, newStatus TEXT, comment TEXT, reviewedBy TEXT, createdAt INTEGER)`,
		`INSERT INTO Aircraft(id, companyId) VALUES ('aircraft-a', 'company-a'), ('aircraft-b', 'company-b')`,
		`INSERT INTO FlightLeg(id, aircraftId) VALUES ('flight-a', 'aircraft-a'), ('flight-b', 'aircraft-b')`,
		`INSERT INTO Exceedance(id, aircraftId, eventStatus, isCurrent) VALUES ('exceedance-a', 'aircraft-a', 'pending', 1), ('exceedance-b', 'aircraft-b', 'pending', 1)`,
		`INSERT INTO Notification(id, userId, isRead) VALUES ('notification-a', 'user-a', 0), ('notification-b', 'user-b', 0)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("setup failed for %q: %v", statement, err)
		}
	}
	return db
}

func tenantContext(method, path, companyID, userID, role string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("userCompanyId", companyID)
	ctx.Set("userId", userID)
	ctx.Set("userRole", role)
	return ctx, recorder
}

func TestAdminAndFDAExceedanceFiltersAreGlobal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, role := range []string{models.RoleAdmin, models.RoleFDA} {
		ctx, _ := tenantContext(http.MethodGet, "/api/exceedances", "", "oversight-user", role, nil)
		filter, args, allowed := accessFilter(ctx)
		if !allowed || filter != "" || len(args) != 0 {
			t.Fatalf("%s filter was not global: allowed=%v filter=%q args=%v", role, allowed, filter, args)
		}
	}
}

func TestTenantRoleCannotDeleteAircraftAcrossCompanies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := tenantTestDB(t)
	handler := NewAircraftHandler(db)
	ctx, recorder := tenantContext(http.MethodDelete, "/api/aircrafts/aircraft-b", "company-a", "user-a", models.RoleGatekeeper, nil)
	ctx.Params = gin.Params{{Key: "id", Value: "aircraft-b"}}
	handler.DeleteAircraft(ctx)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for another tenant's aircraft, got %d", recorder.Code)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(1) FROM Aircraft WHERE id = 'aircraft-b'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("cross-tenant aircraft was deleted: count=%d err=%v", count, err)
	}
}

func TestTenantRoleCannotArchiveExceedanceAcrossCompanies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := tenantTestDB(t)
	handler := NewExceedanceHandler(db)
	ctx, recorder := tenantContext(http.MethodDelete, "/api/exceedances/exceedance-b", "company-a", "user-a", models.RoleGatekeeper, []byte(`{"reason":"duplicate"}`))
	ctx.Params = gin.Params{{Key: "id", Value: "exceedance-b"}}
	handler.DeleteExceedance(ctx)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for another tenant's exceedance, got %d", recorder.Code)
	}
	var isCurrent bool
	if err := db.QueryRow(`SELECT isCurrent FROM Exceedance WHERE id = 'exceedance-b'`).Scan(&isCurrent); err != nil || !isCurrent {
		t.Fatalf("cross-tenant exceedance was archived: current=%v err=%v", isCurrent, err)
	}
}

func TestAdminCanDeleteAircraftAcrossCompanies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := tenantTestDB(t)
	handler := NewAircraftHandler(db)
	ctx, recorder := tenantContext(http.MethodDelete, "/api/aircrafts/aircraft-b", "", "admin-user", models.RoleAdmin, nil)
	ctx.Params = gin.Params{{Key: "id", Value: "aircraft-b"}}
	handler.DeleteAircraft(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected global admin delete to succeed, got %d", recorder.Code)
	}
}

func TestFDACanArchiveExceedanceAcrossCompanies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := tenantTestDB(t)
	handler := NewExceedanceHandler(db)
	ctx, recorder := tenantContext(http.MethodDelete, "/api/exceedances/exceedance-b", "", "fda-user", models.RoleFDA, []byte(`{"reason":"duplicate"}`))
	ctx.Params = gin.Params{{Key: "id", Value: "exceedance-b"}}
	handler.DeleteExceedance(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected global FDA archive to succeed, got %d", recorder.Code)
	}
}

func TestNotificationCanOnlyBeChangedByItsUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := tenantTestDB(t)
	handler := NewNotificationHandler(db)
	ctx, recorder := tenantContext(http.MethodPut, "/api/notifications/notification-b/read", "company-a", "user-a", models.RoleAdmin, nil)
	ctx.Params = gin.Params{{Key: "id", Value: "notification-b"}}
	handler.MarkNotificationAsRead(ctx)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for another user's notification, got %d", recorder.Code)
	}
	var isRead bool
	if err := db.QueryRow(`SELECT isRead FROM Notification WHERE id = 'notification-b'`).Scan(&isRead); err != nil || isRead {
		t.Fatalf("another user's notification was modified: isRead=%v err=%v", isRead, err)
	}
}
