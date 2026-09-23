-- Add logical flight legs without rewriting data-bearing audit tables.
-- Legacy flightId values continue to reference Csv for compatibility;
-- flightLegId is the authoritative logical-flight reference going forward.

CREATE TABLE FlightLeg (
    id TEXT NOT NULL PRIMARY KEY,
    recordingId TEXT NOT NULL,
    name TEXT NOT NULL,
    aircraftId TEXT NOT NULL,
    legIndex INTEGER NOT NULL CHECK (legIndex > 0),
    status TEXT,
    departure TEXT,
    pilot TEXT,
    destination TEXT,
    flightHours TEXT,
    startRow INTEGER NOT NULL CHECK (startRow > 0),
    endRow INTEGER CHECK (endRow IS NULL OR endRow >= startRow),
    startSample TEXT,
    endSample TEXT,
    boundarySource TEXT NOT NULL CHECK (boundarySource IN (
        'legacy', 'whole_recording', 'file_start', 'explicit_identifier',
        'sample_discontinuity', 'manual', 'phase_detector'
    )),
    analysisSummary TEXT CHECK (analysisSummary IS NULL OR json_valid(analysisSummary)),
    createdAt INTEGER NOT NULL,
    updatedAt INTEGER NOT NULL,
    FOREIGN KEY (recordingId) REFERENCES Csv(id) ON DELETE CASCADE,
    FOREIGN KEY (aircraftId) REFERENCES Aircraft(id) ON DELETE RESTRICT,
    UNIQUE (recordingId, legIndex)
);

INSERT INTO FlightLeg (
    id, recordingId, name, aircraftId, legIndex, status, departure, pilot,
    destination, flightHours, startRow, endRow, boundarySource,
    analysisSummary, createdAt, updatedAt
)
SELECT id, id, name, aircraftId, 1, status, departure, pilot,
       destination, flightHours, 2, NULL, 'legacy',
       analysisSummary, createdAt, updatedAt
FROM Csv;

ALTER TABLE DetectionRun ADD COLUMN flightLegId TEXT REFERENCES FlightLeg(id) ON DELETE CASCADE;
UPDATE DetectionRun SET flightLegId = flightId WHERE flightLegId IS NULL;

ALTER TABLE Exceedance ADD COLUMN flightLegId TEXT REFERENCES FlightLeg(id) ON DELETE CASCADE;
UPDATE Exceedance SET flightLegId = flightId WHERE flightLegId IS NULL;

CREATE INDEX FlightLeg_recordingId_idx ON FlightLeg(recordingId);
CREATE INDEX FlightLeg_aircraftId_idx ON FlightLeg(aircraftId);
CREATE INDEX FlightLeg_aircraft_created_idx ON FlightLeg(aircraftId, createdAt);
CREATE INDEX DetectionRun_flightLegId_idx ON DetectionRun(flightLegId);
CREATE INDEX DetectionRun_flightLeg_analysis_idx
    ON DetectionRun(flightLegId, engineVersion, inputHash, sampleIntervalMs, ruleSetHash, status);
CREATE INDEX Exceedance_flightLegId_idx ON Exceedance(flightLegId);
CREATE INDEX Exceedance_flightLeg_current_idx ON Exceedance(flightLegId, isCurrent);
