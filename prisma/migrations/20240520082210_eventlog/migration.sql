/*
  Warnings:

  - You are about to drop the column `OEMLimit` on the `EventLog` table. All the data in the column will be lost.
  - You are about to drop the column `description` on the `EventLog` table. All the data in the column will be lost.
  - You are about to drop the column `eventMarker` on the `EventLog` table. All the data in the column will be lost.
  - You are about to drop the column `parameterName` on the `EventLog` table. All the data in the column will be lost.
  - You are about to drop the column `parameterRef` on the `EventLog` table. All the data in the column will be lost.
  - You are about to drop the column `range` on the `EventLog` table. All the data in the column will be lost.
  - You are about to drop the column `trigger` on the `EventLog` table. All the data in the column will be lost.
  - You are about to drop the column `userLimitHigh` on the `EventLog` table. All the data in the column will be lost.
  - You are about to drop the column `userLimitLow` on the `EventLog` table. All the data in the column will be lost.
  - You are about to drop the column `userLimitMedium` on the `EventLog` table. All the data in the column will be lost.
  - Added the required column `csvName` to the `EventLog` table without a default value. This is not possible if the table is not empty.
  - Added the required column `displayName` to the `EventLog` table without a default value. This is not possible if the table is not empty.
  - Added the required column `eventCode` to the `EventLog` table without a default value. This is not possible if the table is not empty.
  - Added the required column `eventDescription` to the `EventLog` table without a default value. This is not possible if the table is not empty.
  - Added the required column `eventParameter` to the `EventLog` table without a default value. This is not possible if the table is not empty.
  - Added the required column `eventTrigger` to the `EventLog` table without a default value. This is not possible if the table is not empty.
  - Added the required column `eventType` to the `EventLog` table without a default value. This is not possible if the table is not empty.
  - Added the required column `high` to the `EventLog` table without a default value. This is not possible if the table is not empty.
  - Added the required column `low` to the `EventLog` table without a default value. This is not possible if the table is not empty.
  - Added the required column `medium` to the `EventLog` table without a default value. This is not possible if the table is not empty.
  - Added the required column `sampleFrom` to the `EventLog` table without a default value. This is not possible if the table is not empty.
  - Added the required column `sampleTo` to the `EventLog` table without a default value. This is not possible if the table is not empty.
  - Added the required column `severity` to the `EventLog` table without a default value. This is not possible if the table is not empty.
  - Added the required column `sop` to the `EventLog` table without a default value. This is not possible if the table is not empty.

*/
-- RedefineTables
PRAGMA foreign_keys=OFF;
CREATE TABLE "new_EventLog" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "displayName" TEXT NOT NULL,
    "eventCode" TEXT NOT NULL,
    "eventDescription" TEXT NOT NULL,
    "eventParameter" TEXT NOT NULL,
    "eventStatus" TEXT NOT NULL,
    "eventTrigger" TEXT NOT NULL,
    "eventType" TEXT NOT NULL,
    "flightPhase" TEXT NOT NULL,
    "high" TEXT NOT NULL,
    "low" TEXT NOT NULL,
    "medium" TEXT NOT NULL,
    "sampleFrom" TEXT NOT NULL,
    "sampleTo" TEXT NOT NULL,
    "sop" TEXT NOT NULL,
    "csvName" TEXT NOT NULL,
    "severity" TEXT NOT NULL,
    "aircraftId" INTEGER NOT NULL,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" DATETIME NOT NULL,
    CONSTRAINT "EventLog_aircraftId_fkey" FOREIGN KEY ("aircraftId") REFERENCES "Aircraft" ("id") ON DELETE RESTRICT ON UPDATE CASCADE
);
INSERT INTO "new_EventLog" ("aircraftId", "createdAt", "eventStatus", "flightPhase", "id", "updatedAt") SELECT "aircraftId", "createdAt", "eventStatus", "flightPhase", "id", "updatedAt" FROM "EventLog";
DROP TABLE "EventLog";
ALTER TABLE "new_EventLog" RENAME TO "EventLog";
PRAGMA foreign_key_check;
PRAGMA foreign_keys=ON;
