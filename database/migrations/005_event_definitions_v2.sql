-- Event Definition v2
--
-- This is an intentionally destructive reset of event definitions and their
-- derived exceedances. The project owner confirmed that these records may be
-- cleared and that an external backup is available. Aircraft, flights, users,
-- companies, and uploaded CSV files are preserved.

PRAGMA foreign_keys = OFF;

ALTER TABLE Notification RENAME TO _legacy_Notification;
ALTER TABLE Exceedance RENAME TO _legacy_Exceedance;

DROP TABLE IF EXISTS EventDefinitionAssignment;
DROP TABLE IF EXISTS EventDefinitionVersion;
DROP TABLE IF EXISTS EventDefinition;
DROP TABLE IF EXISTS EventLog;

CREATE TABLE EventDefinition (
    id TEXT NOT NULL PRIMARY KEY,
    companyId TEXT,
    eventCode TEXT NOT NULL COLLATE NOCASE,
    eventType TEXT NOT NULL CHECK (eventType IN ('safety', 'fuel', 'maintenance')),
    lifecycleStatus TEXT NOT NULL DEFAULT 'active' CHECK (lifecycleStatus IN ('active', 'retired')),
    createdBy TEXT NOT NULL,
    createdAt INTEGER NOT NULL,
    updatedAt INTEGER NOT NULL,
    FOREIGN KEY (companyId) REFERENCES Company(id) ON DELETE CASCADE,
    FOREIGN KEY (createdBy) REFERENCES User(id) ON DELETE RESTRICT,
    UNIQUE (companyId, eventCode)
);

CREATE UNIQUE INDEX EventDefinition_global_eventCode_key
    ON EventDefinition(eventCode)
    WHERE companyId IS NULL;

CREATE INDEX EventDefinition_companyId_idx ON EventDefinition(companyId);
CREATE INDEX EventDefinition_lifecycleStatus_idx ON EventDefinition(lifecycleStatus);

CREATE TABLE EventDefinitionVersion (
    id TEXT NOT NULL PRIMARY KEY,
    definitionId TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'validated', 'published', 'retired')),
    schemaVersion INTEGER NOT NULL DEFAULT 1 CHECK (schemaVersion > 0),
    eventName TEXT NOT NULL,
    displayName TEXT NOT NULL,
    eventDescription TEXT NOT NULL,
    sop TEXT NOT NULL,
    ruleJson TEXT NOT NULL CHECK (json_valid(ruleJson)),
    ruleHash TEXT NOT NULL,
    changeSummary TEXT NOT NULL,
    eventParameter TEXT NOT NULL,
    eventTrigger TEXT NOT NULL,
    flightPhase TEXT NOT NULL,
    triggerType TEXT NOT NULL DEFAULT 'structured',
    detectionPeriod TEXT NOT NULL DEFAULT 'All Flight',
    severities TEXT NOT NULL CHECK (json_valid(severities)),
    primaryAircraftId TEXT,
    createdBy TEXT NOT NULL,
    approvedBy TEXT,
    validatedAt INTEGER,
    publishedAt INTEGER,
    effectiveFrom INTEGER,
    effectiveTo INTEGER,
    createdAt INTEGER NOT NULL,
    updatedAt INTEGER NOT NULL,
    FOREIGN KEY (definitionId) REFERENCES EventDefinition(id) ON DELETE CASCADE,
    FOREIGN KEY (primaryAircraftId) REFERENCES Aircraft(id) ON DELETE SET NULL,
    FOREIGN KEY (createdBy) REFERENCES User(id) ON DELETE RESTRICT,
    FOREIGN KEY (approvedBy) REFERENCES User(id) ON DELETE RESTRICT,
    UNIQUE (definitionId, version)
);

CREATE INDEX EventDefinitionVersion_definitionId_idx ON EventDefinitionVersion(definitionId);
CREATE INDEX EventDefinitionVersion_status_idx ON EventDefinitionVersion(status);
CREATE INDEX EventDefinitionVersion_ruleHash_idx ON EventDefinitionVersion(ruleHash);

CREATE TABLE EventDefinitionAssignment (
    id TEXT NOT NULL PRIMARY KEY,
    definitionVersionId TEXT NOT NULL,
    scopeType TEXT NOT NULL CHECK (scopeType IN ('company', 'model', 'aircraft')),
    companyId TEXT,
    aircraftId TEXT,
    aircraftMake TEXT,
    modelNumber TEXT,
    createdAt INTEGER NOT NULL,
    FOREIGN KEY (definitionVersionId) REFERENCES EventDefinitionVersion(id) ON DELETE CASCADE,
    FOREIGN KEY (companyId) REFERENCES Company(id) ON DELETE CASCADE,
    FOREIGN KEY (aircraftId) REFERENCES Aircraft(id) ON DELETE CASCADE,
    CHECK (
        (scopeType = 'company' AND companyId IS NOT NULL AND aircraftId IS NULL AND aircraftMake IS NULL AND modelNumber IS NULL)
        OR
        (scopeType = 'model' AND companyId IS NOT NULL AND aircraftId IS NULL AND aircraftMake IS NOT NULL AND modelNumber IS NOT NULL)
        OR
        (scopeType = 'aircraft' AND companyId IS NOT NULL AND aircraftId IS NOT NULL AND aircraftMake IS NULL AND modelNumber IS NULL)
    )
);

CREATE INDEX EventDefinitionAssignment_version_idx ON EventDefinitionAssignment(definitionVersionId);
CREATE INDEX EventDefinitionAssignment_company_idx ON EventDefinitionAssignment(companyId);
CREATE INDEX EventDefinitionAssignment_aircraft_idx ON EventDefinitionAssignment(aircraftId);
CREATE INDEX EventDefinitionAssignment_model_idx ON EventDefinitionAssignment(aircraftMake, modelNumber);

CREATE TABLE Exceedance (
    id TEXT NOT NULL PRIMARY KEY,
    exceedanceValues TEXT NOT NULL,
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
    createdAt INTEGER NOT NULL,
    updatedAt INTEGER NOT NULL,
    FOREIGN KEY (eventId) REFERENCES EventDefinitionVersion(id) ON DELETE SET NULL,
    FOREIGN KEY (aircraftId) REFERENCES Aircraft(id) ON DELETE RESTRICT,
    FOREIGN KEY (flightId) REFERENCES Csv(id) ON DELETE CASCADE
);

CREATE INDEX Exceedance_eventId_idx ON Exceedance(eventId);
CREATE INDEX Exceedance_aircraftId_idx ON Exceedance(aircraftId);
CREATE INDEX Exceedance_flightId_idx ON Exceedance(flightId);

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

CREATE INDEX Notification_userId_idx ON Notification(userId);
CREATE INDEX Notification_exceedanceId_idx ON Notification(exceedanceId);

-- Compatibility view for read paths that still enrich an exceedance with the
-- definition version that produced it. New event CRUD writes only to the v2
-- tables above.
CREATE VIEW EventLog AS
SELECT
    v.id AS id,
    v.eventName AS eventName,
    v.displayName AS displayName,
    d.eventCode AS eventCode,
    v.eventDescription AS eventDescription,
    v.eventParameter AS eventParameter,
    v.eventTrigger AS eventTrigger,
    d.eventType AS eventType,
    v.flightPhase AS flightPhase,
    NULL AS high,
    NULL AS high1,
    NULL AS high2,
    NULL AS low,
    NULL AS low1,
    NULL AS low2,
    v.triggerType AS triggerType,
    v.detectionPeriod AS detectionPeriod,
    v.severities AS severities,
    v.sop AS sop,
    COALESCE(v.primaryAircraftId, '') AS aircraftId,
    v.createdAt AS createdAt,
    v.updatedAt AS updatedAt
FROM EventDefinitionVersion v
JOIN EventDefinition d ON d.id = v.definitionId;

DROP TABLE _legacy_Notification;
DROP TABLE _legacy_Exceedance;

PRAGMA foreign_keys = ON;
