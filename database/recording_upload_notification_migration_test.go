package database

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRecordingUploadNotificationMigrationPreservesEvents(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA foreign_keys = ON;
		CREATE TABLE User (id TEXT PRIMARY KEY);
		CREATE TABLE Csv (id TEXT PRIMARY KEY);
		CREATE TABLE Exceedance (id TEXT PRIMARY KEY);
		CREATE TABLE Notification (
			id TEXT PRIMARY KEY, userId TEXT NOT NULL, exceedanceId TEXT NOT NULL,
			message TEXT NOT NULL, level TEXT NOT NULL, isRead INTEGER NOT NULL,
			createdAt INTEGER NOT NULL, updatedAt INTEGER NOT NULL,
			FOREIGN KEY (userId) REFERENCES User(id) ON DELETE CASCADE,
			FOREIGN KEY (exceedanceId) REFERENCES Exceedance(id) ON DELETE CASCADE
		);
		CREATE INDEX Notification_userId_idx ON Notification(userId);
		CREATE INDEX Notification_exceedanceId_idx ON Notification(exceedanceId);
		INSERT INTO User VALUES ('reviewer');
		INSERT INTO Csv VALUES ('recording-a');
		INSERT INTO Exceedance VALUES ('event-a');
		INSERT INTO Notification VALUES ('existing','reviewer','event-a','Existing event','High',0,1,1);`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("migrations/021_recording_upload_notifications.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	if _, err = db.Exec(`INSERT INTO Notification
		(id, userId, recordingId, message, level, isRead, createdAt, updatedAt)
		VALUES ('upload','reviewer','recording-a','Review recording','Review',0,2,2)`); err != nil {
		t.Fatalf("recording alert insert failed: %v", err)
	}
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM Notification`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("notifications were not preserved: count=%d err=%v", count, err)
	}
	if _, err = db.Exec(`INSERT INTO Notification
		(id, userId, exceedanceId, recordingId, message, level, isRead, createdAt, updatedAt)
		VALUES ('invalid','reviewer','event-a','recording-a','Invalid','Review',0,2,2)`); err == nil {
		t.Fatal("notification should not target both an exceedance and a recording")
	}
}
