package handlers

import (
	"database/sql"
	"testing"

	"fdm-backend/models"

	_ "modernc.org/sqlite"
)

func TestEventCoverageIncludesPublishedFREDCanonicalParameters(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE Aircraft (id TEXT PRIMARY KEY, companyId TEXT NOT NULL, aircraftMake TEXT, modelNumber TEXT, parameters TEXT);
		CREATE TABLE AircraftDecoderProfile (id TEXT PRIMARY KEY, aircraftId TEXT, status TEXT,
			validationStatus TEXT, parameterFormat TEXT, parameterText TEXT);
		INSERT INTO Aircraft VALUES ('aircraft-a', 'company-a', 'Test', 'One', '[]');
		INSERT INTO AircraftDecoderProfile VALUES ('profile-a', 'aircraft-a', 'published', 'passed', 'fred', ?);`, handlerFRED)
	if err != nil {
		t.Fatal(err)
	}
	companyID, aircraftID := "company-a", "aircraft-a"
	errorsFound := NewEventHandler(db).validateParameterCoverage(
		[]models.EventAssignmentInput{{ScopeType: "aircraft", CompanyID: &companyID, AircraftID: &aircraftID}},
		[]models.RuleParameter{
			{ID: "Altitude", DataType: "numeric", Required: true},
			{ID: "Vertical Speed", DataType: "numeric", Required: true},
		},
	)
	if len(errorsFound) != 0 {
		t.Fatalf("expected FRED-derived canonical coverage, got %v", errorsFound)
	}
}
