package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"fdm-backend/models"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

func reportTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	statements := []string{
		`CREATE TABLE Company (id TEXT PRIMARY KEY, name TEXT NOT NULL, status TEXT NOT NULL)`,
		`CREATE TABLE Aircraft (id TEXT PRIMARY KEY, registration TEXT, serialNumber TEXT NOT NULL, aircraftMake TEXT NOT NULL, modelNumber TEXT, companyId TEXT NOT NULL)`,
		`CREATE TABLE FlightLeg (id TEXT PRIMARY KEY, name TEXT, status TEXT, departure TEXT, destination TEXT, flightHours TEXT, aircraftId TEXT NOT NULL, createdAt INTEGER NOT NULL)`,
		`CREATE TABLE EventDefinition (id TEXT PRIMARY KEY, eventCode TEXT NOT NULL)`,
		`CREATE TABLE EventDefinitionVersion (id TEXT PRIMARY KEY, definitionId TEXT NOT NULL, version INTEGER NOT NULL, eventName TEXT, displayName TEXT)`,
		`CREATE TABLE DetectionRun (id TEXT PRIMARY KEY, flightId TEXT NOT NULL, flightLegId TEXT, status TEXT NOT NULL)`,
		`CREATE TABLE DetectionRunDefinition (detectionRunId TEXT NOT NULL, definitionId TEXT NOT NULL, status TEXT NOT NULL, isCurrent INTEGER NOT NULL)`,
		`CREATE TABLE Exceedance (id TEXT PRIMARY KEY, flightId TEXT NOT NULL, flightLegId TEXT, aircraftId TEXT NOT NULL, flightPhase TEXT NOT NULL, parameterName TEXT, eventStatus TEXT NOT NULL, exceedanceLevel TEXT, eventId TEXT, isCurrent INTEGER NOT NULL, peakValue REAL, exceedanceValues TEXT, createdAt INTEGER)`,
		`CREATE TABLE ExceedanceLocation (exceedanceId TEXT PRIMARY KEY, latitude REAL NOT NULL, longitude REAL NOT NULL, altitude REAL, matchedTimeMs INTEGER NOT NULL, timeDeltaMs INTEGER NOT NULL, source TEXT NOT NULL, quality TEXT NOT NULL, createdAt INTEGER NOT NULL, updatedAt INTEGER NOT NULL)`,
		`INSERT INTO Company VALUES ('company-a', 'Alpha Air', 'active'), ('company-b', 'Bravo Air', 'active')`,
		`INSERT INTO Aircraft VALUES
			('aircraft-a', '5H-AAA', 'SN-A', 'Boeing', '737', 'company-a'),
			('aircraft-b', '5H-BBB', 'SN-B', 'Airbus', 'A320', 'company-b')`,
		`INSERT INTO FlightLeg VALUES
			('flight-a', 'Alpha 101', 'completed', 'HKJK', 'HTDA', '2.5', 'aircraft-a', CAST(strftime('%s', '2026-09-01T10:00:00Z') AS INTEGER) * 1000),
			('flight-b', 'Bravo 202', 'completed_with_warnings', 'HTDA', 'HKJK', '3.0', 'aircraft-b', CAST(strftime('%s', '2026-09-02T10:00:00Z') AS INTEGER) * 1000)`,
		`INSERT INTO EventDefinition VALUES ('definition-1', 'HIGH_SPEED'), ('definition-2', 'UNSTABLE')`,
		`INSERT INTO EventDefinitionVersion VALUES
			('version-1', 'definition-1', 1, 'High speed', 'High speed'),
			('version-2', 'definition-2', 1, 'Unstable approach', 'Unstable approach')`,
		`INSERT INTO DetectionRun VALUES
			('run-a', 'flight-a', 'flight-a', 'completed'),
			('run-b', 'flight-b', 'flight-b', 'completed_with_warnings')`,
		`INSERT INTO DetectionRunDefinition VALUES
			('run-a', 'definition-1', 'evaluated', 1),
			('run-a', 'definition-2', 'evaluated', 1),
			('run-b', 'definition-1', 'evaluated', 1)`,
		`INSERT INTO Exceedance VALUES
			('event-a-valid', 'flight-a', 'flight-a', 'aircraft-a', 'CLIMB', 'IAS', 'Valid', 'High', 'version-1', 1, 250, '{"unit":"kt"}', 1),
			('event-a-pending', 'flight-a', 'flight-a', 'aircraft-a', 'APPROACH', 'VSI', 'Pending', 'Critical', 'version-2', 1, 1800, '{"unit":"ft/min"}', 1),
			('event-b-valid', 'flight-b', 'flight-b', 'aircraft-b', 'APPROACH', 'IAS', 'Valid', 'Low', 'version-1', 1, 150, '{"unit":"kt"}', 1)`,
		`INSERT INTO ExceedanceLocation VALUES
			('event-a-valid', -1.2864, 36.8172, 5000, 10000, 0, 'replay_nearest_time', 'exact', 1, 1),
			('event-b-valid', -6.7924, 39.2083, 8000, 20000, 250, 'replay_nearest_time', 'near', 1, 1)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("setup failed for %q: %v", statement, err)
		}
	}
	return db
}

func reportContext(method, path, role, companyID string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, nil)
	ctx.Set("userRole", role)
	if companyID != "" {
		ctx.Set("userCompanyId", companyID)
	}
	return ctx, recorder
}

func TestReportOverviewUsesTemplateFlightLegsAndRoleScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewReportHandler(reportTestDB(t))
	ctx, recorder := reportContext(http.MethodGet, "/api/reports/overview", models.RoleAdmin, "")
	handler.GetOverview(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response models.ReportOverviewResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Flight.TotalFlights != 2 || response.Severity.TotalExceedances != 3 {
		t.Fatalf("expected global totals, got %+v", response)
	}

	tenantContext, tenantRecorder := reportContext(http.MethodGet, "/api/reports/overview", models.RoleGatekeeper, "company-a")
	handler.GetOverview(tenantContext)
	if tenantRecorder.Code != http.StatusOK {
		t.Fatalf("expected tenant 200, got %d: %s", tenantRecorder.Code, tenantRecorder.Body.String())
	}
	if err := json.Unmarshal(tenantRecorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Flight.TotalFlights != 1 || response.Severity.TotalExceedances != 1 {
		t.Fatalf("tenant scope leaked company or review-status data: %+v", response)
	}
}

func TestReportTenantCannotSelectAnotherCompany(t *testing.T) {
	handler := NewReportHandler(reportTestDB(t))
	ctx, recorder := reportContext(http.MethodGet, "/api/reports/options?companyId=company-b", models.RoleUser, "company-a")
	handler.GetOptions(ctx)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestEventAggregateUsesEvaluatedFlightDenominators(t *testing.T) {
	handler := NewReportHandler(reportTestDB(t))
	ctx, recorder := reportContext(http.MethodGet, "/api/reports/events/aggregate?metric=ratePer100Flights&topN=10", models.RoleAdmin, "")
	handler.GetEventAggregate(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response models.EventReportAggregateResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Coverage.EligibleFlights != 2 || response.Coverage.Occurrences != 3 {
		t.Fatalf("unexpected coverage: %+v", response.Coverage)
	}
	if len(response.Series) != 2 || response.Series[0].Key != "definition-1" || response.Series[0].Value != 100 {
		t.Fatalf("expected definition-1 rate of 100 per evaluated flights, got %+v", response.Series)
	}
}

func TestEventAggregateTenantSeesOnlyValidOwnCompanyData(t *testing.T) {
	handler := NewReportHandler(reportTestDB(t))
	ctx, recorder := reportContext(http.MethodGet, "/api/reports/events/aggregate?metric=count", models.RoleGatekeeper, "company-a")
	handler.GetEventAggregate(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response models.EventReportAggregateResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Coverage.Occurrences != 1 || response.Coverage.AffectedFlights != 1 {
		t.Fatalf("tenant aggregate leaked data: %+v", response.Coverage)
	}
}

func TestEventComparisonUsesIndependentEvaluatedCohorts(t *testing.T) {
	db := reportTestDB(t)
	if _, err := db.Exec(`INSERT INTO FlightLeg VALUES
		('flight-a-later', 'Alpha 102', 'completed', 'HKJK', 'HTZA', '1.5', 'aircraft-a', CAST(strftime('%s', '2026-09-03T10:00:00Z') AS INTEGER) * 1000);
		INSERT INTO DetectionRun VALUES ('run-a-later', 'flight-a-later', 'flight-a-later', 'completed');
		INSERT INTO DetectionRunDefinition VALUES ('run-a-later', 'definition-1', 'evaluated', 1)`); err != nil {
		t.Fatal(err)
	}

	handler := NewReportHandler(db)
	path := "/api/reports/events/comparison?metric=ratePer100Flights&topN=10" +
		"&baselineFrom=2026-09-01&baselineTo=2026-09-01" +
		"&comparisonFrom=2026-09-03&comparisonTo=2026-09-03"
	ctx, recorder := reportContext(http.MethodGet, path, models.RoleGatekeeper, "company-a")
	handler.GetEventComparison(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response models.EventReportComparisonResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Series) == 0 {
		t.Fatal("expected comparison series")
	}
	point := response.Series[0]
	if point.Key != "definition-1" || point.BaselineValue != 100 || point.ComparisonValue != 0 || point.AbsoluteDelta != -100 {
		t.Fatalf("unexpected comparison point: %+v", point)
	}
	if response.Baseline.Coverage.EligibleFlights != 1 || response.Comparison.Coverage.EligibleFlights != 1 {
		t.Fatalf("unexpected cohort coverage: baseline=%+v comparison=%+v", response.Baseline, response.Comparison)
	}
}

func TestEventComparisonRequiresValidDateRanges(t *testing.T) {
	handler := NewReportHandler(reportTestDB(t))
	ctx, recorder := reportContext(http.MethodGet, "/api/reports/events/comparison", models.RoleAdmin, "")
	handler.GetEventComparison(ctx)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestEventBenchmarkSuppressesSmallTenantCohorts(t *testing.T) {
	handler := NewReportHandler(reportTestDB(t))
	ctx, recorder := reportContext(http.MethodGet, "/api/reports/events/benchmark?eventDefinitionId=definition-1", models.RoleUser, "company-a")
	handler.GetEventBenchmark(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response models.EventBenchmarkResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Suppressed || response.Percentiles != nil || len(response.Peers) != 0 {
		t.Fatalf("small tenant cohort must be suppressed: %+v", response)
	}
}

func TestEventBenchmarkReturnsAnonymizedTenantPercentilesAndNamedOversightPeers(t *testing.T) {
	db := reportTestDB(t)

	// The focus company already has one evaluated flight in the base fixture.
	for flightIndex := 2; flightIndex <= minimumBenchmarkEligibleFlights; flightIndex++ {
		flightID := fmt.Sprintf("focus-flight-%02d", flightIndex)
		runID := fmt.Sprintf("focus-run-%02d", flightIndex)
		if _, err := db.Exec(`INSERT INTO FlightLeg VALUES (?, ?, 'completed', 'HKJK', 'HTDA', '1', 'aircraft-a', CAST(strftime('%s', '2026-09-05T10:00:00Z') AS INTEGER) * 1000)`, flightID, flightID); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO DetectionRun VALUES (?, ?, ?, 'completed')`, runID, flightID, flightID); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO DetectionRunDefinition VALUES (?, 'definition-1', 'evaluated', 1)`, runID); err != nil {
			t.Fatal(err)
		}
	}

	for peerIndex := 1; peerIndex <= minimumBenchmarkPeerCompanies; peerIndex++ {
		companyID := fmt.Sprintf("peer-%d", peerIndex)
		aircraftID := fmt.Sprintf("peer-aircraft-%d", peerIndex)
		if _, err := db.Exec(`INSERT INTO Company VALUES (?, ?, 'active')`, companyID, fmt.Sprintf("Peer %d", peerIndex)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO Aircraft VALUES (?, ?, ?, 'Boeing', '737', ?)`, aircraftID, fmt.Sprintf("P-%d", peerIndex), fmt.Sprintf("PSN-%d", peerIndex), companyID); err != nil {
			t.Fatal(err)
		}
		peerOccurrences := peerIndex * 3
		for flightIndex := 1; flightIndex <= minimumBenchmarkEligibleFlights; flightIndex++ {
			flightID := fmt.Sprintf("peer-%d-flight-%02d", peerIndex, flightIndex)
			runID := fmt.Sprintf("peer-%d-run-%02d", peerIndex, flightIndex)
			if _, err := db.Exec(`INSERT INTO FlightLeg VALUES (?, ?, 'completed', 'HKJK', 'HTDA', '1', ?, CAST(strftime('%s', '2026-09-05T10:00:00Z') AS INTEGER) * 1000)`, flightID, flightID, aircraftID); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO DetectionRun VALUES (?, ?, ?, 'completed')`, runID, flightID, flightID); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO DetectionRunDefinition VALUES (?, 'definition-1', 'evaluated', 1)`, runID); err != nil {
				t.Fatal(err)
			}
			if flightIndex <= peerOccurrences {
				eventID := fmt.Sprintf("peer-%d-event-%02d", peerIndex, flightIndex)
				if _, err := db.Exec(`INSERT INTO Exceedance VALUES (?, ?, ?, ?, 'CLIMB', 'IAS', 'Valid', 'High', 'version-1', 1, 200, '{"unit":"kt"}', 1)`, eventID, flightID, flightID, aircraftID); err != nil {
					t.Fatal(err)
				}
			}
		}
	}

	handler := NewReportHandler(db)
	tenantPath := "/api/reports/events/benchmark?eventDefinitionId=definition-1&metric=ratePer100Flights"
	tenantContext, tenantRecorder := reportContext(http.MethodGet, tenantPath, models.RoleGatekeeper, "company-a")
	handler.GetEventBenchmark(tenantContext)
	if tenantRecorder.Code != http.StatusOK {
		t.Fatalf("expected tenant 200, got %d: %s", tenantRecorder.Code, tenantRecorder.Body.String())
	}
	var tenantResponse models.EventBenchmarkResponse
	if err := json.Unmarshal(tenantRecorder.Body.Bytes(), &tenantResponse); err != nil {
		t.Fatal(err)
	}
	if tenantResponse.Suppressed || tenantResponse.Percentiles == nil || tenantResponse.Percentiles.P50 != 30 {
		t.Fatalf("unexpected tenant benchmark: %+v", tenantResponse)
	}
	if len(tenantResponse.Peers) != 0 {
		t.Fatalf("tenant benchmark exposed peer identities: %+v", tenantResponse.Peers)
	}

	adminPath := tenantPath + "&companyId=company-a"
	adminContext, adminRecorder := reportContext(http.MethodGet, adminPath, models.RoleAdmin, "")
	handler.GetEventBenchmark(adminContext)
	if adminRecorder.Code != http.StatusOK {
		t.Fatalf("expected admin 200, got %d: %s", adminRecorder.Code, adminRecorder.Body.String())
	}
	var adminResponse models.EventBenchmarkResponse
	if err := json.Unmarshal(adminRecorder.Body.Bytes(), &adminResponse); err != nil {
		t.Fatal(err)
	}
	if len(adminResponse.Peers) != minimumBenchmarkPeerCompanies || adminResponse.Peers[0].CompanyName == "" {
		t.Fatalf("oversight benchmark should include named peers: %+v", adminResponse.Peers)
	}
}

func TestEventLocationHonorsScopeAndReportsUnlocatedOccurrences(t *testing.T) {
	handler := NewReportHandler(reportTestDB(t))
	tenantContext, tenantRecorder := reportContext(http.MethodGet, "/api/reports/events/location?precision=2", models.RoleGatekeeper, "company-a")
	handler.GetEventLocation(tenantContext)
	if tenantRecorder.Code != http.StatusOK {
		t.Fatalf("expected tenant 200, got %d: %s", tenantRecorder.Code, tenantRecorder.Body.String())
	}
	var tenantResponse models.EventLocationResponse
	if err := json.Unmarshal(tenantRecorder.Body.Bytes(), &tenantResponse); err != nil {
		t.Fatal(err)
	}
	if tenantResponse.Coverage.TotalOccurrences != 1 || tenantResponse.Coverage.LocatedOccurrences != 1 || tenantResponse.Coverage.UnlocatedOccurrences != 0 || len(tenantResponse.Cells) != 1 {
		t.Fatalf("tenant location report leaked company or review-status data: %+v", tenantResponse)
	}

	adminContext, adminRecorder := reportContext(http.MethodGet, "/api/reports/events/location?precision=2", models.RoleAdmin, "")
	handler.GetEventLocation(adminContext)
	if adminRecorder.Code != http.StatusOK {
		t.Fatalf("expected admin 200, got %d: %s", adminRecorder.Code, adminRecorder.Body.String())
	}
	var adminResponse models.EventLocationResponse
	if err := json.Unmarshal(adminRecorder.Body.Bytes(), &adminResponse); err != nil {
		t.Fatal(err)
	}
	if adminResponse.Coverage.TotalOccurrences != 3 || adminResponse.Coverage.LocatedOccurrences != 2 || adminResponse.Coverage.UnlocatedOccurrences != 1 || len(adminResponse.Cells) != 2 {
		t.Fatalf("admin location coverage should include all review states without dropping unlocated data: %+v", adminResponse)
	}
}

func TestEventfulFlightsAreScopedSummarizedAndPaginated(t *testing.T) {
	handler := NewReportHandler(reportTestDB(t))
	tenantContext, tenantRecorder := reportContext(http.MethodGet, "/api/reports/events/flights", models.RoleUser, "company-a")
	handler.GetEventfulFlights(tenantContext)
	if tenantRecorder.Code != http.StatusOK {
		t.Fatalf("expected tenant 200, got %d: %s", tenantRecorder.Code, tenantRecorder.Body.String())
	}
	var tenantResponse models.EventfulFlightsResponse
	if err := json.Unmarshal(tenantRecorder.Body.Bytes(), &tenantResponse); err != nil {
		t.Fatal(err)
	}
	if tenantResponse.Summary.TotalEventfulFlights != 1 || tenantResponse.Summary.TotalOccurrences != 1 || len(tenantResponse.Flights) != 1 {
		t.Fatalf("tenant eventful flights leaked company or review-status data: %+v", tenantResponse)
	}
	if tenantResponse.Flights[0].CompanyID != "company-a" || tenantResponse.Flights[0].FlightID != "flight-a" {
		t.Fatalf("unexpected tenant flight: %+v", tenantResponse.Flights[0])
	}

	adminContext, adminRecorder := reportContext(http.MethodGet, "/api/reports/events/flights?page=1&pageSize=1&order=recent", models.RoleFDA, "")
	handler.GetEventfulFlights(adminContext)
	if adminRecorder.Code != http.StatusOK {
		t.Fatalf("expected FDA 200, got %d: %s", adminRecorder.Code, adminRecorder.Body.String())
	}
	var adminResponse models.EventfulFlightsResponse
	if err := json.Unmarshal(adminRecorder.Body.Bytes(), &adminResponse); err != nil {
		t.Fatal(err)
	}
	if adminResponse.Summary.TotalEventfulFlights != 2 || adminResponse.Summary.TotalOccurrences != 3 || adminResponse.TotalPages != 2 || len(adminResponse.Flights) != 1 {
		t.Fatalf("unexpected FDA eventful-flight summary or pagination: %+v", adminResponse)
	}
	if adminResponse.Flights[0].FlightID != "flight-b" || adminResponse.Flights[0].CompanyName != "Bravo Air" {
		t.Fatalf("recent FDA result should include identified cross-company flight: %+v", adminResponse.Flights[0])
	}
}

func TestKPVDistributionUsesAuthorizedReviewedNumericEvidence(t *testing.T) {
	handler := NewReportHandler(reportTestDB(t))
	tenantOptionsContext, tenantOptionsRecorder := reportContext(http.MethodGet, "/api/reports/kpv/options", models.RoleUser, "company-a")
	handler.GetKPVOptions(tenantOptionsContext)
	if tenantOptionsRecorder.Code != http.StatusOK {
		t.Fatalf("expected tenant options 200, got %d: %s", tenantOptionsRecorder.Code, tenantOptionsRecorder.Body.String())
	}
	var tenantOptions models.KPVOptionsResponse
	if err := json.Unmarshal(tenantOptionsRecorder.Body.Bytes(), &tenantOptions); err != nil {
		t.Fatal(err)
	}
	if len(tenantOptions.Options) != 1 || tenantOptions.Options[0].EventDefinitionID != "definition-1" || tenantOptions.Options[0].SampleCount != 1 {
		t.Fatalf("tenant KPV options leaked pending or cross-company evidence: %+v", tenantOptions.Options)
	}
	tenantPath := "/api/reports/kpv/distribution?eventDefinitionId=definition-1&parameterName=IAS&unit=kt&splitBy=company&binCount=5"
	tenantContext, tenantRecorder := reportContext(http.MethodGet, tenantPath, models.RoleGatekeeper, "company-a")
	handler.GetKPVDistribution(tenantContext)
	if tenantRecorder.Code != http.StatusOK {
		t.Fatalf("expected tenant distribution 200, got %d: %s", tenantRecorder.Code, tenantRecorder.Body.String())
	}
	var tenantDistribution models.KPVDistributionResponse
	if err := json.Unmarshal(tenantRecorder.Body.Bytes(), &tenantDistribution); err != nil {
		t.Fatal(err)
	}
	if tenantDistribution.Statistics.Count != 1 || len(tenantDistribution.Groups) != 1 || tenantDistribution.Groups[0].Key != "company-a" {
		t.Fatalf("tenant KPV distribution leaked cross-company samples: %+v", tenantDistribution)
	}

	adminPath := "/api/reports/kpv/distribution?eventDefinitionId=definition-1&parameterName=IAS&unit=kt&splitBy=company&binCount=5"
	adminContext, adminRecorder := reportContext(http.MethodGet, adminPath, models.RoleAdmin, "")
	handler.GetKPVDistribution(adminContext)
	if adminRecorder.Code != http.StatusOK {
		t.Fatalf("expected admin distribution 200, got %d: %s", adminRecorder.Code, adminRecorder.Body.String())
	}
	var response models.KPVDistributionResponse
	if err := json.Unmarshal(adminRecorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Statistics.Count != 2 || response.Statistics.Mean != 200 || response.Statistics.Median != 200 {
		t.Fatalf("unexpected KPV statistics: %+v", response.Statistics)
	}
	if len(response.Groups) != 2 || len(response.Histogram) != 2 {
		t.Fatalf("expected company comparison and bounded histogram, got groups=%+v histogram=%+v", response.Groups, response.Histogram)
	}
}
