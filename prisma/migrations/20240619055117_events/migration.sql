/*
  Warnings:

  - You are about to drop the column `OEMLimit` on the `Exceedance` table. All the data in the column will be lost.
  - You are about to drop the column `eventMarker` on the `Exceedance` table. All the data in the column will be lost.
  - You are about to drop the column `parameterRef` on the `Exceedance` table. All the data in the column will be lost.
  - You are about to drop the column `range` on the `Exceedance` table. All the data in the column will be lost.
  - You are about to drop the column `trigger` on the `Exceedance` table. All the data in the column will be lost.
  - You are about to drop the column `userLimitHigh` on the `Exceedance` table. All the data in the column will be lost.
  - You are about to drop the column `userLimitLow` on the `Exceedance` table. All the data in the column will be lost.
  - You are about to drop the column `userLimitMedium` on the `Exceedance` table. All the data in the column will be lost.
  - You are about to drop the column `csvName` on the `EventLog` table. All the data in the column will be lost.
  - You are about to drop the column `eventStatus` on the `EventLog` table. All the data in the column will be lost.
  - You are about to drop the column `flightID` on the `EventLog` table. All the data in the column will be lost.
  - You are about to drop the column `sampleFrom` on the `EventLog` table. All the data in the column will be lost.
  - You are about to drop the column `sampleTo` on the `EventLog` table. All the data in the column will be lost.
  - You are about to drop the column `severity` on the `EventLog` table. All the data in the column will be lost.
  - Added the required column `flightId` to the `Exceedance` table without a default value. This is not possible if the table is not empty.

*/
-- RedefineTables
PRAGMA foreign_keys=OFF;
CREATE TABLE "new_Exceedance" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "flightPhase" TEXT NOT NULL,
    "parameterName" TEXT NOT NULL,
    "description" TEXT NOT NULL,
    "eventStatus" TEXT NOT NULL,
    "aircraftId" INTEGER NOT NULL,
    "flightId" INTEGER NOT NULL,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" DATETIME NOT NULL,
    CONSTRAINT "Exceedance_aircraftId_fkey" FOREIGN KEY ("aircraftId") REFERENCES "Aircraft" ("id") ON DELETE RESTRICT ON UPDATE CASCADE
);
INSERT INTO "new_Exceedance" ("aircraftId", "createdAt", "description", "eventStatus", "flightPhase", "id", "parameterName", "updatedAt") SELECT "aircraftId", "createdAt", "description", "eventStatus", "flightPhase", "id", "parameterName", "updatedAt" FROM "Exceedance";
DROP TABLE "Exceedance";
ALTER TABLE "new_Exceedance" RENAME TO "Exceedance";
CREATE TABLE "new_EventLog" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "eventName" TEXT,
    "displayName" TEXT NOT NULL,
    "eventCode" TEXT NOT NULL,
    "eventDescription" TEXT NOT NULL,
    "eventParameter" TEXT NOT NULL,
    "eventTrigger" TEXT NOT NULL,
    "eventType" TEXT NOT NULL,
    "flightPhase" TEXT NOT NULL,
    "high" TEXT,
    "high1" TEXT,
    "high2" TEXT,
    "low" TEXT,
    "low1" TEXT,
    "low2" TEXT,
    "sop" TEXT NOT NULL,
    "aircraftId" INTEGER NOT NULL,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" DATETIME NOT NULL,
    CONSTRAINT "EventLog_aircraftId_fkey" FOREIGN KEY ("aircraftId") REFERENCES "Aircraft" ("id") ON DELETE RESTRICT ON UPDATE CASCADE
);
INSERT INTO "new_EventLog" ("aircraftId", "createdAt", "displayName", "eventCode", "eventDescription", "eventName", "eventParameter", "eventTrigger", "eventType", "flightPhase", "high", "high1", "high2", "id", "low", "low1", "low2", "sop", "updatedAt") SELECT "aircraftId", "createdAt", "displayName", "eventCode", "eventDescription", "eventName", "eventParameter", "eventTrigger", "eventType", "flightPhase", "high", "high1", "high2", "id", "low", "low1", "low2", "sop", "updatedAt" FROM "EventLog";
DROP TABLE "EventLog";
ALTER TABLE "new_EventLog" RENAME TO "EventLog";
PRAGMA foreign_key_check;
PRAGMA foreign_keys=ON;
