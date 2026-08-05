package handlers

import (
	"database/sql"
	"testing"

	"fdm-backend/detection"
	"fdm-backend/models"

	_ "modernc.org/sqlite"
)

func TestHashRuleSetIsOrderIndependent(t *testing.T) {
	left := []detection.Definition{{VersionID: "v2", RuleHash: "b"}, {VersionID: "v1", RuleHash: "a"}}
	right := []detection.Definition{{VersionID: "v1", RuleHash: "a"}, {VersionID: "v2", RuleHash: "b"}}
	if hashRuleSet(left) != hashRuleSet(right) {
		t.Fatal("rule-set hash must not depend on query order")
	}
	right[1].RuleHash = "changed"
	if hashRuleSet(left) == hashRuleSet(right) {
		t.Fatal("rule-set hash must change when a rule changes")
	}
}

func TestPersistDetectionResultSupersedesOnlyAffectedDefinition(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	schema := `
		CREATE TABLE Csv (id TEXT PRIMARY KEY, status TEXT, analysisSummary TEXT, updatedAt INTEGER);
		CREATE TABLE User (id TEXT PRIMARY KEY, companyId TEXT, isActive INTEGER, role TEXT);
		CREATE TABLE EventDefinitionVersion (id TEXT PRIMARY KEY, definitionId TEXT NOT NULL);
		CREATE TABLE DetectionRun (
			id TEXT PRIMARY KEY, flightId TEXT, status TEXT, timingSource TEXT, rowCount INTEGER,
			evaluatedRuleCount INTEGER, occurrenceCount INTEGER, diagnosticsJson TEXT, completedAt INTEGER
		);
		CREATE TABLE DetectionRunDefinition (
			detectionRunId TEXT, definitionId TEXT, definitionVersionId TEXT, ruleHash TEXT,
			status TEXT, occurrenceCount INTEGER, diagnosticsJson TEXT, isCurrent INTEGER,
			createdAt INTEGER, supersededAt INTEGER
		);
		CREATE TABLE Exceedance (
			id TEXT PRIMARY KEY, exceedanceValues TEXT, flightPhase TEXT, parameterName TEXT,
			description TEXT, eventStatus TEXT, aircraftId TEXT, flightId TEXT, file TEXT,
			eventId TEXT, exceedanceLevel TEXT, detectionRunId TEXT, startTimeMs INTEGER,
			endTimeMs INTEGER, durationMs INTEGER, peakValue REAL, ruleHash TEXT,
			isCurrent INTEGER, supersededAt INTEGER, createdAt INTEGER, updatedAt INTEGER
		);
		CREATE TABLE Notification (
			id TEXT PRIMARY KEY, userId TEXT, exceedanceId TEXT, message TEXT, level TEXT,
			isRead INTEGER, createdAt INTEGER, updatedAt INTEGER
		);
		INSERT INTO Csv VALUES ('flight-1', 'completed', '{}', 1);
		INSERT INTO EventDefinitionVersion VALUES ('a-v1', 'definition-a'), ('a-v2', 'definition-a'), ('a-v3', 'definition-a'), ('b-v1', 'definition-b');
		INSERT INTO DetectionRun VALUES ('old-a-run', 'flight-1', 'completed', 'sample', 1, 1, 1, '[]', 1);
		INSERT INTO DetectionRun VALUES ('old-b-run', 'flight-1', 'completed', 'sample', 1, 1, 1, '[]', 1);
		INSERT INTO DetectionRun VALUES ('new-run', 'flight-1', 'processing', NULL, 0, 0, 0, '[]', NULL);
		INSERT INTO DetectionRunDefinition VALUES ('old-a-run', 'definition-a', 'a-v1', 'old-a', 'evaluated', 1, '[]', 1, 1, NULL);
		INSERT INTO DetectionRunDefinition VALUES ('old-b-run', 'definition-b', 'b-v1', 'old-b', 'evaluated', 1, '[]', 1, 1, NULL);
		INSERT INTO Exceedance VALUES ('old-a', '{}', 'LANDING', 'A', 'old a', 'Valid', 'aircraft-1', 'flight-1', 'flight.csv', 'a-v1', 'Low', 'old-a-run', 0, 1, 1, 1, 'old-a', 1, NULL, 1, 1);
		INSERT INTO Exceedance VALUES ('old-b', '{}', 'LANDING', 'B', 'old b', 'Valid', 'aircraft-1', 'flight-1', 'flight.csv', 'b-v1', 'Low', 'old-b-run', 0, 1, 1, 1, 'old-b', 1, NULL, 1, 1);
	`
	if _, err = db.Exec(schema); err != nil {
		t.Fatal(err)
	}

	handler := NewCSVHandler(db)
	flight := models.CSV{ID: "flight-1"}
	aircraft := models.Aircraft{ID: "aircraft-1", CompanyID: "company-1"}
	definitions := []detection.Definition{{DefinitionID: "definition-a", VersionID: "a-v2", RuleHash: "new-a"}}
	response := flightAnalysisResponse{
		RunID: "new-run", Status: "completed", EngineVersion: detection.EngineVersion,
		TimingSource: "sample", RowCount: 2, ApplicableRuleCount: 1, EvaluatedRuleCount: 1,
		OccurrenceCount: 1, RuleSetHash: hashRuleSet(definitions), TriggerType: "event_backfill",
		Occurrences: []detection.Occurrence{{
			DefinitionID: "definition-a", VersionID: "a-v2", EventCode: "A", EventName: "A",
			Description: "new a", RuleHash: "new-a", ParameterID: "A", Aggregation: "MAX",
			Value: 2, Severity: "LOW", Phase: "LANDING", StartTimeMs: 2, EndTimeMs: 3,
			DurationMs: 1, PointCount: 1,
		}},
		Diagnostics: []detection.Diagnostic{},
	}
	if err = handler.persistDetectionResult("new-run", "flight.csv", &flight, aircraft, definitions, response); err != nil {
		t.Fatal(err)
	}

	assertCurrent := func(id string, want int) {
		t.Helper()
		var current int
		if err := db.QueryRow("SELECT isCurrent FROM Exceedance WHERE id = ?", id).Scan(&current); err != nil {
			t.Fatal(err)
		}
		if current != want {
			t.Fatalf("exceedance %s current=%d, want %d", id, current, want)
		}
	}
	assertCurrent("old-a", 0)
	assertCurrent("old-b", 1)
	var newCount int
	if err = db.QueryRow("SELECT COUNT(1) FROM Exceedance WHERE eventId = 'a-v2' AND isCurrent = 1").Scan(&newCount); err != nil || newCount != 1 {
		t.Fatalf("expected one current replacement occurrence, count=%d err=%v", newCount, err)
	}
	var oldEvaluationCurrent, unrelatedEvaluationCurrent int
	if err = db.QueryRow("SELECT isCurrent FROM DetectionRunDefinition WHERE detectionRunId = 'old-a-run'").Scan(&oldEvaluationCurrent); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT isCurrent FROM DetectionRunDefinition WHERE detectionRunId = 'old-b-run'").Scan(&unrelatedEvaluationCurrent); err != nil {
		t.Fatal(err)
	}
	if oldEvaluationCurrent != 0 || unrelatedEvaluationCurrent != 1 {
		t.Fatalf("unexpected evaluation state: affected=%d unrelated=%d", oldEvaluationCurrent, unrelatedEvaluationCurrent)
	}

	if _, err = db.Exec(`INSERT INTO DetectionRun VALUES
		('skipped-run', 'flight-1', 'processing', NULL, 0, 0, 0, '[]', NULL)`); err != nil {
		t.Fatal(err)
	}
	skippedDefinitions := []detection.Definition{{DefinitionID: "definition-a", VersionID: "a-v3", RuleHash: "newer-a"}}
	skippedResponse := flightAnalysisResponse{
		RunID: "skipped-run", Status: "completed_with_warnings", EngineVersion: detection.EngineVersion,
		ApplicableRuleCount: 1, EvaluatedRuleCount: 0, RuleSetHash: hashRuleSet(skippedDefinitions),
		TriggerType: "event_backfill", Occurrences: []detection.Occurrence{},
		Diagnostics: []detection.Diagnostic{{
			Code: "RULE_REQUIRED_COLUMNS_MISSING", Severity: "error",
			RuleVersionID: "a-v3", Message: "required column missing", Count: 1,
		}},
	}
	if err = handler.persistDetectionResult("skipped-run", "flight.csv", &flight, aircraft, skippedDefinitions, skippedResponse); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT COUNT(1) FROM Exceedance WHERE eventId = 'a-v2' AND isCurrent = 1").Scan(&newCount); err != nil || newCount != 1 {
		t.Fatalf("skipped rule must preserve the prior current finding, count=%d err=%v", newCount, err)
	}
	var skippedCurrent int
	if err = db.QueryRow("SELECT isCurrent FROM DetectionRunDefinition WHERE detectionRunId = 'skipped-run'").Scan(&skippedCurrent); err != nil {
		t.Fatal(err)
	}
	if skippedCurrent != 0 {
		t.Fatalf("skipped evaluation must not become current, got %d", skippedCurrent)
	}
}
