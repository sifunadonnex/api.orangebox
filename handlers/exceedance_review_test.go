package handlers

import (
	"database/sql"
	"fdm-backend/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

func newExceedanceReviewTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schema := `
		CREATE TABLE User (id TEXT PRIMARY KEY, fullName TEXT, email TEXT);
		CREATE TABLE Aircraft (id TEXT PRIMARY KEY, companyId TEXT NOT NULL);
		CREATE TABLE Exceedance (
			id TEXT PRIMARY KEY,
			aircraftId TEXT NOT NULL,
			eventStatus TEXT NOT NULL,
			comment TEXT,
			isCurrent INTEGER NOT NULL DEFAULT 1,
			supersededAt INTEGER,
			updatedAt INTEGER NOT NULL
		);
		CREATE TABLE ExceedanceReview (
			id TEXT PRIMARY KEY,
			exceedanceId TEXT NOT NULL,
			action TEXT NOT NULL,
			previousStatus TEXT,
			newStatus TEXT,
			comment TEXT NOT NULL,
			reviewedBy TEXT NOT NULL,
			createdAt INTEGER NOT NULL
		);
		INSERT INTO User (id, fullName, email) VALUES ('reviewer-1', 'Test Reviewer', 'reviewer@example.com');
		INSERT INTO Aircraft (id, companyId) VALUES ('aircraft-1', 'company-1');
		INSERT INTO Exceedance (id, aircraftId, eventStatus, isCurrent, updatedAt) VALUES ('occurrence-1', 'aircraft-1', 'Pending', 1, 1);
	`
	if _, err = db.Exec(schema); err != nil {
		t.Fatal(err)
	}

	return db
}

func reviewContext(method, body string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: "occurrence-1"}}
	context.Set("userId", "reviewer-1")
	context.Set("userRole", models.RoleFDA)
	context.Request = httptest.NewRequest(method, "/api/exceedances/occurrence-1", strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")

	return context, recorder
}

func TestUpdateExceedanceRecordsReviewHistory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newExceedanceReviewTestDB(t)
	handler := NewExceedanceHandler(db)
	context, recorder := reviewContext(http.MethodPut, `{"eventStatus":"Valid","comment":"Evidence checked against the source data."}`)

	handler.UpdateExceedance(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var status, comment string
	if err := db.QueryRow("SELECT eventStatus, comment FROM Exceedance WHERE id = 'occurrence-1'").Scan(&status, &comment); err != nil {
		t.Fatal(err)
	}
	if status != "Valid" || comment == "" {
		t.Fatalf("unexpected occurrence state: status=%q comment=%q", status, comment)
	}

	var action, previousStatus, newStatus, reviewer string
	if err := db.QueryRow("SELECT action, previousStatus, newStatus, reviewedBy FROM ExceedanceReview").Scan(
		&action, &previousStatus, &newStatus, &reviewer,
	); err != nil {
		t.Fatal(err)
	}
	if action != "status_change" || previousStatus != "Pending" || newStatus != "Valid" || reviewer != "reviewer-1" {
		t.Fatalf("unexpected review row: %q %q %q %q", action, previousStatus, newStatus, reviewer)
	}
}

func TestArchiveExceedancePreservesRowAndRecordsReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newExceedanceReviewTestDB(t)
	handler := NewExceedanceHandler(db)
	context, recorder := reviewContext(http.MethodDelete, `{"reason":"Duplicate test occurrence retained for audit."}`)

	handler.DeleteExceedance(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var isCurrent int
	var supersededAt sql.NullInt64
	if err := db.QueryRow("SELECT isCurrent, supersededAt FROM Exceedance WHERE id = 'occurrence-1'").Scan(&isCurrent, &supersededAt); err != nil {
		t.Fatal(err)
	}
	if isCurrent != 0 || !supersededAt.Valid {
		t.Fatalf("expected archived row to be retained, got isCurrent=%d supersededAt=%v", isCurrent, supersededAt)
	}

	var action, reason string
	if err := db.QueryRow("SELECT action, comment FROM ExceedanceReview").Scan(&action, &reason); err != nil {
		t.Fatal(err)
	}
	if action != "archive" || reason == "" {
		t.Fatalf("unexpected archive review: action=%q reason=%q", action, reason)
	}
}

func TestUpdateExceedanceRejectsMissingComment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newExceedanceReviewTestDB(t)
	handler := NewExceedanceHandler(db)
	context, recorder := reviewContext(http.MethodPut, `{"eventStatus":"Valid","comment":""}`)

	handler.UpdateExceedance(context)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestGatekeeperCanCommentAndChangeValidatedExceedance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newExceedanceReviewTestDB(t)
	if _, err := db.Exec("UPDATE Exceedance SET eventStatus = 'Valid' WHERE id = 'occurrence-1'"); err != nil {
		t.Fatal(err)
	}
	handler := NewExceedanceHandler(db)

	commentContext, commentRecorder := reviewContext(http.MethodPut, `{"eventStatus":"Valid","comment":"Checked with the operator."}`)
	commentContext.Set("userRole", models.RoleGatekeeper)
	commentContext.Set("userCompanyId", "company-1")
	handler.UpdateExceedance(commentContext)
	if commentRecorder.Code != http.StatusOK {
		t.Fatalf("expected comment-only review to succeed, got %d: %s", commentRecorder.Code, commentRecorder.Body.String())
	}
	var action string
	if err := db.QueryRow("SELECT action FROM ExceedanceReview LIMIT 1").Scan(&action); err != nil || action != "comment" {
		t.Fatalf("expected a comment audit entry, got action=%q err=%v", action, err)
	}

	changeContext, changeRecorder := reviewContext(http.MethodPut, `{"eventStatus":"Under Review","comment":"Needs operational follow-up."}`)
	changeContext.Set("userRole", models.RoleGatekeeper)
	changeContext.Set("userCompanyId", "company-1")
	handler.UpdateExceedance(changeContext)
	if changeRecorder.Code != http.StatusOK {
		t.Fatalf("expected status change to succeed, got %d: %s", changeRecorder.Code, changeRecorder.Body.String())
	}

	// A previously validated occurrence remains available for a later gatekeeper decision.
	returnContext, returnRecorder := reviewContext(http.MethodPut, `{"eventStatus":"Valid","comment":"Follow-up complete."}`)
	returnContext.Set("userRole", models.RoleGatekeeper)
	returnContext.Set("userCompanyId", "company-1")
	handler.UpdateExceedance(returnContext)
	if returnRecorder.Code != http.StatusOK {
		t.Fatalf("expected follow-up review to succeed, got %d: %s", returnRecorder.Code, returnRecorder.Body.String())
	}
}

func TestGatekeeperCannotValidatePendingOrReviewAnotherCompany(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newExceedanceReviewTestDB(t)
	handler := NewExceedanceHandler(db)

	pendingContext, pendingRecorder := reviewContext(http.MethodPut, `{"eventStatus":"Valid","comment":"Not authorized to validate."}`)
	pendingContext.Set("userRole", models.RoleGatekeeper)
	pendingContext.Set("userCompanyId", "company-1")
	handler.UpdateExceedance(pendingContext)
	if pendingRecorder.Code != http.StatusForbidden {
		t.Fatalf("expected pending validation to be forbidden, got %d: %s", pendingRecorder.Code, pendingRecorder.Body.String())
	}

	if _, err := db.Exec("UPDATE Exceedance SET eventStatus = 'Valid' WHERE id = 'occurrence-1'"); err != nil {
		t.Fatal(err)
	}
	otherContext, otherRecorder := reviewContext(http.MethodPut, `{"eventStatus":"False","comment":"Wrong tenant."}`)
	otherContext.Set("userRole", models.RoleGatekeeper)
	otherContext.Set("userCompanyId", "company-2")
	handler.UpdateExceedance(otherContext)
	if otherRecorder.Code != http.StatusNotFound {
		t.Fatalf("expected cross-company review to be hidden, got %d: %s", otherRecorder.Code, otherRecorder.Body.String())
	}
}

func TestRegularUserCannotReviewExceedance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newExceedanceReviewTestDB(t)
	handler := NewExceedanceHandler(db)
	context, recorder := reviewContext(http.MethodPut, `{"eventStatus":"Valid","comment":"Should be read-only."}`)
	context.Set("userRole", models.RoleUser)
	context.Set("userCompanyId", "company-1")
	handler.UpdateExceedance(context)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected regular user to be forbidden, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAccessFilterScopesNonPrivilegedUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Set("userRole", models.RoleUser)
	context.Set("userCompanyId", "company-1")

	filter, args, allowed := accessFilter(context)
	if !allowed || filter == "" || len(args) != 2 || args[0] != "company-1" || args[1] != models.ExceedanceStatusValid {
		t.Fatalf("unexpected user access filter: allowed=%v filter=%q args=%v", allowed, filter, args)
	}
}

func TestAccessFilterScopesGatekeeperToValidatedCompanyOccurrences(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newExceedanceReviewTestDB(t)
	if _, err := db.Exec(`UPDATE Exceedance SET eventStatus = 'Under Review' WHERE id = 'occurrence-1';
		INSERT INTO ExceedanceReview (id, exceedanceId, action, previousStatus, newStatus, comment, reviewedBy, createdAt)
		VALUES ('review-1', 'occurrence-1', 'status_change', 'Valid', 'Under Review', 'Follow-up needed', 'reviewer-1', 2)`); err != nil {
		t.Fatal(err)
	}
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Set("userRole", models.RoleGatekeeper)
	context.Set("userCompanyId", "company-1")

	filter, args, allowed := accessFilter(context)
	if !allowed || !strings.Contains(filter, "ExceedanceReview") || len(args) != 1 || args[0] != "company-1" {
		t.Fatalf("unexpected gatekeeper access filter: allowed=%v filter=%q args=%v", allowed, filter, args)
	}
	query := "SELECT COUNT(*) FROM Exceedance e JOIN Aircraft a ON a.id = e.aircraftId WHERE e.isCurrent = 1" + filter
	var count int
	if err := db.QueryRow(query, args...).Scan(&count); err != nil || count != 1 {
		t.Fatalf("gatekeeper should retain access to previously validated review: count=%d err=%v", count, err)
	}
	context.Set("userCompanyId", "company-2")
	filter, args, _ = accessFilter(context)
	query = "SELECT COUNT(*) FROM Exceedance e JOIN Aircraft a ON a.id = e.aircraftId WHERE e.isCurrent = 1" + filter
	if err := db.QueryRow(query, args...).Scan(&count); err != nil || count != 0 {
		t.Fatalf("gatekeeper must not see another company: count=%d err=%v", count, err)
	}
}

func TestAccessFilterAllowsPrivilegedCrossCompanyReview(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Set("userRole", models.RoleFDA)

	filter, args, allowed := accessFilter(context)
	if !allowed || filter != "" || len(args) != 0 {
		t.Fatalf("unexpected FDA access filter: allowed=%v filter=%q args=%v", allowed, filter, args)
	}
}
