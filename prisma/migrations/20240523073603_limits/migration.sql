-- RedefineTables
PRAGMA foreign_keys=OFF;
CREATE TABLE "new_EventLog" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "eventName" TEXT,
    "displayName" TEXT NOT NULL,
    "eventCode" TEXT NOT NULL,
    "eventDescription" TEXT NOT NULL,
    "eventParameter" TEXT NOT NULL,
    "eventStatus" TEXT NOT NULL,
    "eventTrigger" TEXT NOT NULL,
    "eventType" TEXT NOT NULL,
    "flightPhase" TEXT NOT NULL,
    "high" TEXT,
    "low" TEXT,
    "medium" TEXT,
    "sampleFrom" TEXT NOT NULL,
    "sampleTo" TEXT NOT NULL,
    "sop" TEXT NOT NULL,
    "csvName" TEXT NOT NULL,
    "flightID" TEXT,
    "severity" TEXT NOT NULL,
    "aircraftId" INTEGER NOT NULL,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" DATETIME NOT NULL,
    CONSTRAINT "EventLog_aircraftId_fkey" FOREIGN KEY ("aircraftId") REFERENCES "Aircraft" ("id") ON DELETE RESTRICT ON UPDATE CASCADE
);
INSERT INTO "new_EventLog" ("aircraftId", "createdAt", "csvName", "displayName", "eventCode", "eventDescription", "eventName", "eventParameter", "eventStatus", "eventTrigger", "eventType", "flightID", "flightPhase", "high", "id", "low", "medium", "sampleFrom", "sampleTo", "severity", "sop", "updatedAt") SELECT "aircraftId", "createdAt", "csvName", "displayName", "eventCode", "eventDescription", "eventName", "eventParameter", "eventStatus", "eventTrigger", "eventType", "flightID", "flightPhase", "high", "id", "low", "medium", "sampleFrom", "sampleTo", "severity", "sop", "updatedAt" FROM "EventLog";
DROP TABLE "EventLog";
ALTER TABLE "new_EventLog" RENAME TO "EventLog";
PRAGMA foreign_key_check;
PRAGMA foreign_keys=ON;
