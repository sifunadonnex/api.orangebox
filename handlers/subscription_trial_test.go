package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fdm-backend/models"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

func newSubscriptionTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE Subscription (
		id TEXT PRIMARY KEY,
		planName TEXT NOT NULL,
		planType TEXT NOT NULL,
		trialDays INTEGER NOT NULL DEFAULT 0,
		maxUsers INTEGER NOT NULL,
		maxAircraft INTEGER NOT NULL,
		maxFlightsPerMonth INTEGER NOT NULL,
		maxStorageGB INTEGER NOT NULL,
		price REAL NOT NULL,
		currency TEXT NOT NULL,
		startDate DATETIME NOT NULL,
		endDate DATETIME NOT NULL,
		isActive BOOLEAN NOT NULL,
		autoRenew BOOLEAN NOT NULL,
		lastPaymentDate DATETIME,
		nextPaymentDate DATETIME,
		alertSentAt DATETIME,
		createdAt DATETIME NOT NULL,
		updatedAt DATETIME NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}

	return db
}

func TestCreateTrialSubscriptionForcesFreeNonRenewingPeriod(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newSubscriptionTestDB(t)
	handler := NewSubscriptionHandler(db)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(`{
		"planName":"Evaluation",
		"planType":"trial",
		"trialDays":14,
		"price":99,
		"currency":"USD",
		"autoRenew":true
	}`))
	context.Request.Header.Set("Content-Type", "application/json")

	handler.CreateSubscription(context)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var subscription models.Subscription
	if err := json.Unmarshal(recorder.Body.Bytes(), &subscription); err != nil {
		t.Fatal(err)
	}
	if subscription.Price != 0 {
		t.Fatalf("expected a zero trial price, got %v", subscription.Price)
	}
	if subscription.AutoRenew {
		t.Fatal("expected trial auto-renew to be disabled")
	}
	if subscription.TrialDays != 14 {
		t.Fatalf("expected 14 trial days, got %d", subscription.TrialDays)
	}
	if days := int(subscription.EndDate.Sub(subscription.StartDate).Hours() / 24); days != 14 {
		t.Fatalf("expected a 14-day period, got %d days", days)
	}
	if !subscription.IsActive {
		t.Fatal("expected a newly-created trial plan to be active")
	}
}

func TestTrialSubscriptionWindowStartsWhenCompanyIsAssigned(t *testing.T) {
	db := newSubscriptionTestDB(t)
	now := time.Date(2026, time.September, 25, 12, 0, 0, 0, time.UTC)
	_, err := db.Exec(`INSERT INTO Subscription (
		id, planName, planType, trialDays, maxUsers, maxAircraft, maxFlightsPerMonth, maxStorageGB,
		price, currency, startDate, endDate, isActive, autoRenew, createdAt, updatedAt
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"trial", "Free Trial", "trial", 30, 3, 1, 50, 2, 0, "USD", now, now.AddDate(0, 0, 30), true, false, now, now)
	if err != nil {
		t.Fatal(err)
	}

	handler := NewCompanyHandler(db)
	startedAt, endsAt, err := handler.subscriptionWindow("trial", now.AddDate(0, 2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !startedAt.Equal(now.AddDate(0, 2, 0)) {
		t.Fatalf("unexpected start date: %s", startedAt)
	}
	if days := int(endsAt.Sub(*startedAt).Hours() / 24); days != 30 {
		t.Fatalf("expected a fresh 30-day company trial, got %d days", days)
	}
}
