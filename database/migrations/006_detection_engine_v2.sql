-- Deterministic exceedance detection audit trail.

ALTER TABLE Csv ADD COLUMN sampleIntervalMs INTEGER CHECK (sampleIntervalMs IS NULL OR sampleIntervalMs > 0);
ALTER TABLE Csv ADD COLUMN analysisSummary TEXT CHECK (analysisSummary IS NULL OR json_valid(analysisSummary));

CREATE TABLE DetectionRun (
    id TEXT NOT NULL PRIMARY KEY,
    flightId TEXT NOT NULL,
    aircraftId TEXT NOT NULL,
    engineVersion TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('processing', 'completed', 'completed_with_warnings', 'failed')),
    inputHash TEXT NOT NULL,
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
    FOREIGN KEY (aircraftId) REFERENCES Aircraft(id) ON DELETE RESTRICT,
    UNIQUE (flightId, engineVersion, inputHash)
);

CREATE INDEX DetectionRun_flightId_idx ON DetectionRun(flightId);
CREATE INDEX DetectionRun_aircraftId_idx ON DetectionRun(aircraftId);
CREATE INDEX DetectionRun_status_idx ON DetectionRun(status);

ALTER TABLE Notification RENAME TO _detection_v2_Notification;
ALTER TABLE Exceedance RENAME TO _detection_v2_Exceedance;

DROP INDEX Notification_userId_idx;
DROP INDEX Notification_exceedanceId_idx;
DROP INDEX Exceedance_eventId_idx;
DROP INDEX Exceedance_aircraftId_idx;
DROP INDEX Exceedance_flightId_idx;

CREATE TABLE Exceedance (
    id TEXT NOT NULL PRIMARY KEY,
    exceedanceValues TEXT NOT NULL CHECK (json_valid(exceedanceValues)),
    flightPhase TEXT NOT NULL,
    parameterName TEXT NOT NULL,
    description TEXT NOT NULL,
    eventStatus TEXT NOT NULL,
    aircraftId TEXT NOT NULL,
    flightId TEXT NOT NULL,
    file TEXT,
    eventId TEXT,
    comment TEXT,
    exceedanceLevel TEXT,
    detectionRunId TEXT,
    startTimeMs INTEGER,
    endTimeMs INTEGER,
    durationMs INTEGER CHECK (durationMs IS NULL OR durationMs >= 0),
    peakValue REAL,
    ruleHash TEXT,
    createdAt INTEGER NOT NULL,
    updatedAt INTEGER NOT NULL,
    FOREIGN KEY (eventId) REFERENCES EventDefinitionVersion(id) ON DELETE SET NULL,
    FOREIGN KEY (aircraftId) REFERENCES Aircraft(id) ON DELETE RESTRICT,
    FOREIGN KEY (flightId) REFERENCES Csv(id) ON DELETE CASCADE,
    FOREIGN KEY (detectionRunId) REFERENCES DetectionRun(id) ON DELETE SET NULL,
    UNIQUE (detectionRunId, eventId, startTimeMs, endTimeMs)
);

INSERT INTO Exceedance (
    id, exceedanceValues, flightPhase, parameterName, description, eventStatus,
    aircraftId, flightId, file, eventId, comment, exceedanceLevel, createdAt, updatedAt
)
SELECT id, exceedanceValues, flightPhase, parameterName, description, eventStatus,
       aircraftId, flightId, file, eventId, comment, exceedanceLevel, createdAt, updatedAt
FROM _detection_v2_Exceedance;

CREATE INDEX Exceedance_eventId_idx ON Exceedance(eventId);
CREATE INDEX Exceedance_aircraftId_idx ON Exceedance(aircraftId);
CREATE INDEX Exceedance_flightId_idx ON Exceedance(flightId);
CREATE INDEX Exceedance_detectionRunId_idx ON Exceedance(detectionRunId);
CREATE INDEX Exceedance_event_flight_idx ON Exceedance(eventId, flightId);

CREATE TABLE Notification (
    id TEXT NOT NULL PRIMARY KEY,
    userId TEXT NOT NULL,
    exceedanceId TEXT NOT NULL,
    message TEXT NOT NULL,
    level TEXT NOT NULL,
    isRead INTEGER NOT NULL DEFAULT 0,
    createdAt INTEGER NOT NULL,
    updatedAt INTEGER NOT NULL,
    FOREIGN KEY (userId) REFERENCES User(id) ON DELETE CASCADE,
    FOREIGN KEY (exceedanceId) REFERENCES Exceedance(id) ON DELETE CASCADE
);

INSERT INTO Notification (id, userId, exceedanceId, message, level, isRead, createdAt, updatedAt)
SELECT id, userId, exceedanceId, message, level, isRead, createdAt, updatedAt
FROM _detection_v2_Notification;

CREATE INDEX Notification_userId_idx ON Notification(userId);
CREATE INDEX Notification_exceedanceId_idx ON Notification(exceedanceId);

DROP TABLE _detection_v2_Notification;
DROP TABLE _detection_v2_Exceedance;
