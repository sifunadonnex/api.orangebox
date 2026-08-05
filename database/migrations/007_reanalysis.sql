-- Rerunnable, rule-set-aware flight analysis.

PRAGMA foreign_keys = OFF;

CREATE TABLE _reanalysis_DetectionRun (
    id TEXT NOT NULL PRIMARY KEY,
    flightId TEXT NOT NULL,
    aircraftId TEXT NOT NULL,
    engineVersion TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('processing', 'completed', 'completed_with_warnings', 'failed')),
    inputHash TEXT NOT NULL,
    ruleSetHash TEXT NOT NULL,
    triggerType TEXT NOT NULL DEFAULT 'upload' CHECK (triggerType IN ('upload', 'manual', 'event_backfill')),
    triggeredBy TEXT,
    timingSource TEXT,
    sampleIntervalMs INTEGER CHECK (sampleIntervalMs IS NULL OR sampleIntervalMs > 0),
    rowCount INTEGER NOT NULL DEFAULT 0 CHECK (rowCount >= 0),
    applicableRuleCount INTEGER NOT NULL DEFAULT 0 CHECK (applicableRuleCount >= 0),
    evaluatedRuleCount INTEGER NOT NULL DEFAULT 0 CHECK (evaluatedRuleCount >= 0),
    occurrenceCount INTEGER NOT NULL DEFAULT 0 CHECK (occurrenceCount >= 0),
    diagnosticsJson TEXT NOT NULL CHECK (json_valid(diagnosticsJson)),
    errorMessage TEXT,
    startedAt INTEGER NOT NULL,
    completedAt INTEGER,
    FOREIGN KEY (flightId) REFERENCES Csv(id) ON DELETE CASCADE,
    FOREIGN KEY (aircraftId) REFERENCES Aircraft(id) ON DELETE RESTRICT
);

INSERT INTO _reanalysis_DetectionRun (
    id, flightId, aircraftId, engineVersion, status, inputHash, ruleSetHash,
    triggerType, timingSource, sampleIntervalMs, rowCount, applicableRuleCount,
    evaluatedRuleCount, occurrenceCount, diagnosticsJson, errorMessage, startedAt, completedAt
)
SELECT id, flightId, aircraftId, engineVersion, status, inputHash, 'legacy:' || id,
       'upload', timingSource, sampleIntervalMs, rowCount, applicableRuleCount,
       evaluatedRuleCount, occurrenceCount, diagnosticsJson, errorMessage, startedAt, completedAt
FROM DetectionRun;

DROP TABLE DetectionRun;
ALTER TABLE _reanalysis_DetectionRun RENAME TO DetectionRun;

CREATE INDEX DetectionRun_flightId_idx ON DetectionRun(flightId);
CREATE INDEX DetectionRun_aircraftId_idx ON DetectionRun(aircraftId);
CREATE INDEX DetectionRun_status_idx ON DetectionRun(status);
CREATE INDEX DetectionRun_analysis_key_idx
    ON DetectionRun(flightId, engineVersion, inputHash, sampleIntervalMs, ruleSetHash, status);

CREATE TABLE DetectionRunDefinition (
    detectionRunId TEXT NOT NULL,
    definitionId TEXT NOT NULL,
    definitionVersionId TEXT NOT NULL,
    ruleHash TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('evaluated', 'skipped')),
    occurrenceCount INTEGER NOT NULL DEFAULT 0 CHECK (occurrenceCount >= 0),
    diagnosticsJson TEXT NOT NULL CHECK (json_valid(diagnosticsJson)),
    isCurrent INTEGER NOT NULL DEFAULT 1 CHECK (isCurrent IN (0, 1)),
    createdAt INTEGER NOT NULL,
    supersededAt INTEGER,
    PRIMARY KEY (detectionRunId, definitionVersionId),
    FOREIGN KEY (detectionRunId) REFERENCES DetectionRun(id) ON DELETE CASCADE,
    FOREIGN KEY (definitionId) REFERENCES EventDefinition(id) ON DELETE CASCADE,
    FOREIGN KEY (definitionVersionId) REFERENCES EventDefinitionVersion(id) ON DELETE CASCADE
);

CREATE INDEX DetectionRunDefinition_definition_current_idx
    ON DetectionRunDefinition(definitionId, isCurrent);
CREATE INDEX DetectionRunDefinition_version_idx
    ON DetectionRunDefinition(definitionVersionId);

ALTER TABLE Exceedance ADD COLUMN isCurrent INTEGER NOT NULL DEFAULT 1 CHECK (isCurrent IN (0, 1));
ALTER TABLE Exceedance ADD COLUMN supersededAt INTEGER;
CREATE INDEX Exceedance_flight_current_idx ON Exceedance(flightId, isCurrent);

PRAGMA foreign_keys = ON;
