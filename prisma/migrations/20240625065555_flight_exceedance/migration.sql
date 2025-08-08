-- RedefineTables
PRAGMA foreign_keys=OFF;
CREATE TABLE "new_Exceedance" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "exceedanceValues" TEXT NOT NULL,
    "flightPhase" TEXT NOT NULL,
    "parameterName" TEXT NOT NULL,
    "description" TEXT NOT NULL,
    "eventStatus" TEXT NOT NULL,
    "aircraftId" INTEGER NOT NULL,
    "flightId" INTEGER NOT NULL,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" DATETIME NOT NULL,
    CONSTRAINT "Exceedance_aircraftId_fkey" FOREIGN KEY ("aircraftId") REFERENCES "Aircraft" ("id") ON DELETE RESTRICT ON UPDATE CASCADE,
    CONSTRAINT "Exceedance_flightId_fkey" FOREIGN KEY ("flightId") REFERENCES "Csv" ("id") ON DELETE RESTRICT ON UPDATE CASCADE
);
INSERT INTO "new_Exceedance" ("aircraftId", "createdAt", "description", "eventStatus", "exceedanceValues", "flightId", "flightPhase", "id", "parameterName", "updatedAt") SELECT "aircraftId", "createdAt", "description", "eventStatus", "exceedanceValues", "flightId", "flightPhase", "id", "parameterName", "updatedAt" FROM "Exceedance";
DROP TABLE "Exceedance";
ALTER TABLE "new_Exceedance" RENAME TO "Exceedance";
PRAGMA foreign_key_check;
PRAGMA foreign_keys=ON;
