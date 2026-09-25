-- Persist compact, recording-relative phase timelines produced during upload.
-- Source CSV files remain immutable; reanalysis and replay use these row ranges.

ALTER TABLE Csv ADD COLUMN phaseEngineVersion TEXT;
ALTER TABLE Csv ADD COLUMN phaseProfile TEXT;
ALTER TABLE Csv ADD COLUMN phaseTimingSource TEXT;
ALTER TABLE Csv ADD COLUMN phaseSummary TEXT CHECK (phaseSummary IS NULL OR json_valid(phaseSummary));

CREATE TABLE FlightPhaseRun (
    id TEXT NOT NULL PRIMARY KEY,
    recordingId TEXT NOT NULL,
    flightIndex INTEGER NOT NULL DEFAULT 0 CHECK (flightIndex >= 0),
    flightStatus TEXT,
    phase TEXT NOT NULL,
    startRow INTEGER NOT NULL CHECK (startRow > 0),
    endRow INTEGER NOT NULL CHECK (endRow >= startRow),
    startSample TEXT,
    endSample TEXT,
    durationMs INTEGER NOT NULL CHECK (durationMs >= 0),
    createdAt INTEGER NOT NULL,
    FOREIGN KEY (recordingId) REFERENCES Csv(id) ON DELETE CASCADE,
    UNIQUE (recordingId, flightIndex, phase, startRow, endRow)
);

CREATE INDEX FlightPhaseRun_recording_row_idx
    ON FlightPhaseRun(recordingId, startRow, endRow);
CREATE INDEX FlightPhaseRun_recording_flight_idx
    ON FlightPhaseRun(recordingId, flightIndex);
