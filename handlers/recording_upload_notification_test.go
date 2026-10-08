package handlers

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestCustomerUploadNotifiesActiveOversightUsersAcrossCompanies(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE User (id TEXT PRIMARY KEY, companyId TEXT, role TEXT, isActive INTEGER);
		CREATE TABLE Notification (id TEXT PRIMARY KEY, userId TEXT, recordingId TEXT, message TEXT, level TEXT, isRead INTEGER, createdAt INTEGER, updatedAt INTEGER);
		INSERT INTO User VALUES
		('admin', 'company-a', 'admin', 1),
		('fda', 'company-b', 'fda', 1),
		('customer', 'company-a', 'user', 1),
		('inactive-fda', 'company-a', 'fda', 0);`)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	registration := "5Y-TEST"
	if err = createRecordingUploadNotifications(tx, "recording-a", "Logger recording", &registration, 2, 1234); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT userId, recordingId, message FROM Notification ORDER BY userId`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var recipients []string
	for rows.Next() {
		var userID, recordingID, message string
		if err = rows.Scan(&userID, &recordingID, &message); err != nil {
			t.Fatal(err)
		}
		if recordingID != "recording-a" || message == "" {
			t.Fatalf("invalid recording alert: %s %s %s", userID, recordingID, message)
		}
		recipients = append(recipients, userID)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(recipients) != 2 || recipients[0] != "admin" || recipients[1] != "fda" {
		t.Fatalf("unexpected recipients: %v", recipients)
	}
}
