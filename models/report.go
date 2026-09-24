package models

// ReportFilters is the shared query contract used by every reporting endpoint.
type ReportFilters struct {
	CompanyID    *string  `json:"companyId,omitempty"`
	AircraftMake string   `json:"aircraftMake,omitempty"`
	ModelNumber  string   `json:"modelNumber,omitempty"`
	AircraftIDs  []string `json:"aircraftIds"`
	From         string   `json:"from,omitempty"`
	To           string   `json:"to,omitempty"`
	Phases       []string `json:"phases"`
}

type ReportScopeResponse struct {
	Mode             string  `json:"mode"`
	CompanyID        *string `json:"companyId,omitempty"`
	CompanyName      *string `json:"companyName,omitempty"`
	CanSelectCompany bool    `json:"canSelectCompany"`
}

type ReportCompanyOption struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type ReportFleetOption struct {
	AircraftMake string `json:"aircraftMake"`
	ModelNumber  string `json:"modelNumber"`
	Label        string `json:"label"`
}

type ReportAircraftOption struct {
	ID           string `json:"id"`
	Registration string `json:"registration"`
	SerialNumber string `json:"serialNumber"`
	Label        string `json:"label"`
}

type ReportOptionsResponse struct {
	Scope     ReportScopeResponse    `json:"scope"`
	Filters   ReportFilters          `json:"filters"`
	Companies []ReportCompanyOption  `json:"companies"`
	Fleets    []ReportFleetOption    `json:"fleets"`
	Aircraft  []ReportAircraftOption `json:"aircraft"`
	Phases    []string               `json:"phases"`
}

type FlightReportOverview struct {
	TotalFlights     int     `json:"totalFlights"`
	AnalyzedFlights  int     `json:"analyzedFlights"`
	TotalFlightHours float64 `json:"totalFlightHours"`
	TotalExceedances int     `json:"totalExceedances"`
}

type SeverityReportOverview struct {
	TotalExceedances int `json:"totalExceedances"`
	HighCritical     int `json:"highCritical"`
}

type ReportOverviewResponse struct {
	Scope    ReportScopeResponse    `json:"scope"`
	Filters  ReportFilters          `json:"filters"`
	Flight   FlightReportOverview   `json:"flight"`
	Severity SeverityReportOverview `json:"severity"`
}

type EventReportPoint struct {
	Key             string  `json:"key"`
	Label           string  `json:"label"`
	Occurrences     int     `json:"occurrences"`
	AffectedFlights int     `json:"affectedFlights"`
	EligibleFlights int     `json:"eligibleFlights"`
	Value           float64 `json:"value"`
}

type EventReportBreakdown struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

type EventReportCoverage struct {
	EligibleFlights    int `json:"eligibleFlights"`
	AffectedFlights    int `json:"affectedFlights"`
	Occurrences        int `json:"occurrences"`
	SkippedEvaluations int `json:"skippedEvaluations"`
	FailedFlights      int `json:"failedFlights"`
}

type EventReportAggregateResponse struct {
	Scope      ReportScopeResponse    `json:"scope"`
	Filters    ReportFilters          `json:"filters"`
	Metric     string                 `json:"metric"`
	Order      string                 `json:"order"`
	TopN       int                    `json:"topN"`
	Coverage   EventReportCoverage    `json:"coverage"`
	Series     []EventReportPoint     `json:"series"`
	Severities []EventReportBreakdown `json:"severities"`
	Phases     []EventReportBreakdown `json:"phases"`
	AsOf       string                 `json:"asOf"`
}

type EventReportComparisonCohort struct {
	Label    string              `json:"label"`
	From     string              `json:"from"`
	To       string              `json:"to"`
	Coverage EventReportCoverage `json:"coverage"`
}

type EventReportComparisonPoint struct {
	Key               string   `json:"key"`
	Label             string   `json:"label"`
	BaselineValue     float64  `json:"baselineValue"`
	ComparisonValue   float64  `json:"comparisonValue"`
	AbsoluteDelta     float64  `json:"absoluteDelta"`
	RelativeDelta     *float64 `json:"relativeDelta,omitempty"`
	BaselineFlights   int      `json:"baselineFlights"`
	ComparisonFlights int      `json:"comparisonFlights"`
}

type EventReportComparisonResponse struct {
	Scope      ReportScopeResponse          `json:"scope"`
	Filters    ReportFilters                `json:"filters"`
	Metric     string                       `json:"metric"`
	TopN       int                          `json:"topN"`
	Baseline   EventReportComparisonCohort  `json:"baseline"`
	Comparison EventReportComparisonCohort  `json:"comparison"`
	Series     []EventReportComparisonPoint `json:"series"`
	AsOf       string                       `json:"asOf"`
}

type EventBenchmarkCompany struct {
	CompanyID       string  `json:"companyId,omitempty"`
	CompanyName     string  `json:"companyName,omitempty"`
	Value           float64 `json:"value"`
	Occurrences     int     `json:"occurrences"`
	AffectedFlights int     `json:"affectedFlights"`
	EligibleFlights int     `json:"eligibleFlights"`
}

type EventBenchmarkPercentiles struct {
	P25 float64 `json:"p25"`
	P50 float64 `json:"p50"`
	P75 float64 `json:"p75"`
	P90 float64 `json:"p90"`
}

type EventBenchmarkResponse struct {
	Scope                  ReportScopeResponse        `json:"scope"`
	Filters                ReportFilters              `json:"filters"`
	Metric                 string                     `json:"metric"`
	EventDefinitionID      string                     `json:"eventDefinitionId"`
	EventLabel             string                     `json:"eventLabel"`
	StatusPolicy           string                     `json:"statusPolicy"`
	Suppressed             bool                       `json:"suppressed"`
	SuppressionReason      string                     `json:"suppressionReason,omitempty"`
	MinimumPeerCompanies   int                        `json:"minimumPeerCompanies"`
	MinimumEligibleFlights int                        `json:"minimumEligibleFlights"`
	EligiblePeerCompanies  int                        `json:"eligiblePeerCompanies"`
	Focus                  EventBenchmarkCompany      `json:"focus"`
	Percentiles            *EventBenchmarkPercentiles `json:"percentiles,omitempty"`
	Peers                  []EventBenchmarkCompany    `json:"peers,omitempty"`
	AsOf                   string                     `json:"asOf"`
}

type EventLocationCoverage struct {
	EligibleFlights      int `json:"eligibleFlights"`
	AffectedFlights      int `json:"affectedFlights"`
	TotalOccurrences     int `json:"totalOccurrences"`
	LocatedOccurrences   int `json:"locatedOccurrences"`
	UnlocatedOccurrences int `json:"unlocatedOccurrences"`
}

type EventLocationCell struct {
	Key               string  `json:"key"`
	Latitude          float64 `json:"latitude"`
	Longitude         float64 `json:"longitude"`
	Occurrences       int     `json:"occurrences"`
	AffectedFlights   int     `json:"affectedFlights"`
	ExactMatches      int     `json:"exactMatches"`
	NearMatches       int     `json:"nearMatches"`
	RatePer100Flights float64 `json:"ratePer100Flights"`
}

type EventLocationBounds struct {
	West  float64 `json:"west"`
	South float64 `json:"south"`
	East  float64 `json:"east"`
	North float64 `json:"north"`
}

type EventLocationResponse struct {
	Scope     ReportScopeResponse   `json:"scope"`
	Filters   ReportFilters         `json:"filters"`
	Precision int                   `json:"precision"`
	Coverage  EventLocationCoverage `json:"coverage"`
	Bounds    *EventLocationBounds  `json:"bounds,omitempty"`
	Cells     []EventLocationCell   `json:"cells"`
	AsOf      string                `json:"asOf"`
}

type EventfulFlightSeverity struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Other    int `json:"other"`
}

type EventfulFlight struct {
	FlightID     string                 `json:"flightId"`
	FlightName   string                 `json:"flightName"`
	AircraftID   string                 `json:"aircraftId"`
	Registration string                 `json:"registration"`
	CompanyID    string                 `json:"companyId,omitempty"`
	CompanyName  string                 `json:"companyName,omitempty"`
	Departure    string                 `json:"departure,omitempty"`
	Destination  string                 `json:"destination,omitempty"`
	FlightHours  string                 `json:"flightHours,omitempty"`
	FlightStatus string                 `json:"flightStatus,omitempty"`
	OccurredAt   int64                  `json:"occurredAt"`
	Occurrences  int                    `json:"occurrences"`
	EventTypes   int                    `json:"eventTypes"`
	EventCodes   []string               `json:"eventCodes"`
	Phases       []string               `json:"phases"`
	Severity     EventfulFlightSeverity `json:"severity"`
}

type EventfulFlightsSummary struct {
	TotalEventfulFlights int `json:"totalEventfulFlights"`
	TotalOccurrences     int `json:"totalOccurrences"`
	HighCriticalFlights  int `json:"highCriticalFlights"`
}

type EventfulFlightsResponse struct {
	Scope      ReportScopeResponse    `json:"scope"`
	Filters    ReportFilters          `json:"filters"`
	Order      string                 `json:"order"`
	Page       int                    `json:"page"`
	PageSize   int                    `json:"pageSize"`
	TotalPages int                    `json:"totalPages"`
	Summary    EventfulFlightsSummary `json:"summary"`
	Flights    []EventfulFlight       `json:"flights"`
	AsOf       string                 `json:"asOf"`
}

type KPVOption struct {
	EventDefinitionID string `json:"eventDefinitionId"`
	EventLabel        string `json:"eventLabel"`
	ParameterName     string `json:"parameterName"`
	Unit              string `json:"unit"`
	SampleCount       int    `json:"sampleCount"`
	AffectedFlights   int    `json:"affectedFlights"`
}

type KPVOptionsResponse struct {
	Scope   ReportScopeResponse `json:"scope"`
	Filters ReportFilters       `json:"filters"`
	Options []KPVOption         `json:"options"`
}

type KPVStatistics struct {
	Count        int     `json:"count"`
	Minimum      float64 `json:"minimum"`
	Maximum      float64 `json:"maximum"`
	Mean         float64 `json:"mean"`
	Median       float64 `json:"median"`
	StandardDev  float64 `json:"standardDeviation"`
	P25          float64 `json:"p25"`
	P75          float64 `json:"p75"`
	IQR          float64 `json:"iqr"`
	LowerFence   float64 `json:"lowerFence"`
	UpperFence   float64 `json:"upperFence"`
	OutlierCount int     `json:"outlierCount"`
}

type KPVHistogramBin struct {
	LowerBound float64 `json:"lowerBound"`
	UpperBound float64 `json:"upperBound"`
	Count      int     `json:"count"`
	Percentage float64 `json:"percentage"`
}

type KPVGroup struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	SampleCount int     `json:"sampleCount"`
	Minimum     float64 `json:"minimum"`
	Maximum     float64 `json:"maximum"`
	Mean        float64 `json:"mean"`
	Median      float64 `json:"median"`
}

type KPVOutlier struct {
	OccurrenceID string  `json:"occurrenceId"`
	FlightID     string  `json:"flightId"`
	Registration string  `json:"registration"`
	CompanyName  string  `json:"companyName,omitempty"`
	Value        float64 `json:"value"`
	OccurredAt   int64   `json:"occurredAt"`
}

type KPVDistributionResponse struct {
	Scope             ReportScopeResponse `json:"scope"`
	Filters           ReportFilters       `json:"filters"`
	EventDefinitionID string              `json:"eventDefinitionId"`
	EventLabel        string              `json:"eventLabel"`
	ParameterName     string              `json:"parameterName"`
	Unit              string              `json:"unit"`
	SplitBy           string              `json:"splitBy"`
	BinCount          int                 `json:"binCount"`
	Statistics        KPVStatistics       `json:"statistics"`
	Histogram         []KPVHistogramBin   `json:"histogram"`
	Groups            []KPVGroup          `json:"groups"`
	Outliers          []KPVOutlier        `json:"outliers"`
	OutliersTruncated bool                `json:"outliersTruncated"`
	AsOf              string              `json:"asOf"`
}
