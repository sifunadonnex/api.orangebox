/*
  Warnings:

  - The primary key for the `Aircraft` table will be changed. If it partially fails, the table could be left without primary key constraint.
  - The primary key for the `Csv` table will be changed. If it partially fails, the table could be left without primary key constraint.
  - The primary key for the `Exceedance` table will be changed. If it partially fails, the table could be left without primary key constraint.
  - The primary key for the `EventLog` table will be changed. If it partially fails, the table could be left without primary key constraint.
  - The primary key for the `Flight` table will be changed. If it partially fails, the table could be left without primary key constraint.
  - The primary key for the `User` table will be changed. If it partially fails, the table could be left without primary key constraint.

*/
-- RedefineTables
PRAGMA foreign_keys=OFF;
CREATE TABLE "new_Aircraft" (
    "id" TEXT NOT NULL PRIMARY KEY,
    "airline" TEXT NOT NULL,
    "aircraftMake" TEXT NOT NULL,
    "serialNumber" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" DATETIME NOT NULL,
    CONSTRAINT "Aircraft_userId_fkey" FOREIGN KEY ("userId") REFERENCES "User" ("id") ON DELETE RESTRICT ON UPDATE CASCADE
);
INSERT INTO "new_Aircraft" ("aircraftMake", "airline", "createdAt", "id", "serialNumber", "updatedAt", "userId") SELECT "aircraftMake", "airline", "createdAt", "id", "serialNumber", "updatedAt", "userId" FROM "Aircraft";
DROP TABLE "Aircraft";
ALTER TABLE "new_Aircraft" RENAME TO "Aircraft";
CREATE TABLE "new_Csv" (
    "id" TEXT NOT NULL PRIMARY KEY,
    "name" TEXT NOT NULL,
    "file" TEXT NOT NULL,
    "status" TEXT,
    "departure" TEXT,
    "pilot" TEXT,
    "destination" TEXT,
    "flightHours" TEXT,
    "aircraftId" TEXT NOT NULL,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" DATETIME NOT NULL,
    CONSTRAINT "Csv_aircraftId_fkey" FOREIGN KEY ("aircraftId") REFERENCES "Aircraft" ("id") ON DELETE RESTRICT ON UPDATE CASCADE
);
INSERT INTO "new_Csv" ("aircraftId", "createdAt", "departure", "destination", "file", "flightHours", "id", "name", "pilot", "status", "updatedAt") SELECT "aircraftId", "createdAt", "departure", "destination", "file", "flightHours", "id", "name", "pilot", "status", "updatedAt" FROM "Csv";
DROP TABLE "Csv";
ALTER TABLE "new_Csv" RENAME TO "Csv";
CREATE TABLE "new_Exceedance" (
    "id" TEXT NOT NULL PRIMARY KEY,
    "exceedanceValues" TEXT NOT NULL,
    "flightPhase" TEXT NOT NULL,
    "parameterName" TEXT NOT NULL,
    "description" TEXT NOT NULL,
    "eventStatus" TEXT NOT NULL,
    "aircraftId" TEXT NOT NULL,
    "flightId" TEXT NOT NULL,
    "file" TEXT,
    "eventId" TEXT,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" DATETIME NOT NULL,
    CONSTRAINT "Exceedance_eventId_fkey" FOREIGN KEY ("eventId") REFERENCES "EventLog" ("id") ON DELETE SET NULL ON UPDATE CASCADE,
    CONSTRAINT "Exceedance_aircraftId_fkey" FOREIGN KEY ("aircraftId") REFERENCES "Aircraft" ("id") ON DELETE RESTRICT ON UPDATE CASCADE,
    CONSTRAINT "Exceedance_flightId_fkey" FOREIGN KEY ("flightId") REFERENCES "Csv" ("id") ON DELETE RESTRICT ON UPDATE CASCADE
);
INSERT INTO "new_Exceedance" ("aircraftId", "createdAt", "description", "eventId", "eventStatus", "exceedanceValues", "file", "flightId", "flightPhase", "id", "parameterName", "updatedAt") SELECT "aircraftId", "createdAt", "description", "eventId", "eventStatus", "exceedanceValues", "file", "flightId", "flightPhase", "id", "parameterName", "updatedAt" FROM "Exceedance";
DROP TABLE "Exceedance";
ALTER TABLE "new_Exceedance" RENAME TO "Exceedance";
CREATE TABLE "new_EventLog" (
    "id" TEXT NOT NULL PRIMARY KEY,
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
    "aircraftId" TEXT NOT NULL,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" DATETIME NOT NULL,
    CONSTRAINT "EventLog_aircraftId_fkey" FOREIGN KEY ("aircraftId") REFERENCES "Aircraft" ("id") ON DELETE RESTRICT ON UPDATE CASCADE
);
INSERT INTO "new_EventLog" ("aircraftId", "createdAt", "displayName", "eventCode", "eventDescription", "eventName", "eventParameter", "eventTrigger", "eventType", "flightPhase", "high", "high1", "high2", "id", "low", "low1", "low2", "sop", "updatedAt") SELECT "aircraftId", "createdAt", "displayName", "eventCode", "eventDescription", "eventName", "eventParameter", "eventTrigger", "eventType", "flightPhase", "high", "high1", "high2", "id", "low", "low1", "low2", "sop", "updatedAt" FROM "EventLog";
DROP TABLE "EventLog";
ALTER TABLE "new_EventLog" RENAME TO "EventLog";
CREATE TABLE "new_Flight" (
    "id" TEXT NOT NULL PRIMARY KEY,
    "name" TEXT NOT NULL,
    "aircraftId" TEXT NOT NULL,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" DATETIME NOT NULL,
    CONSTRAINT "Flight_aircraftId_fkey" FOREIGN KEY ("aircraftId") REFERENCES "Aircraft" ("id") ON DELETE RESTRICT ON UPDATE CASCADE
);
INSERT INTO "new_Flight" ("aircraftId", "createdAt", "id", "name", "updatedAt") SELECT "aircraftId", "createdAt", "id", "name", "updatedAt" FROM "Flight";
DROP TABLE "Flight";
ALTER TABLE "new_Flight" RENAME TO "Flight";
CREATE TABLE "new_User" (
    "id" TEXT NOT NULL PRIMARY KEY,
    "email" TEXT NOT NULL,
    "role" TEXT DEFAULT 'client',
    "fullName" TEXT,
    "username" TEXT,
    "password" TEXT,
    "image" TEXT,
    "company" TEXT,
    "phone" TEXT,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" DATETIME NOT NULL
);
INSERT INTO "new_User" ("company", "createdAt", "email", "fullName", "id", "image", "password", "phone", "role", "updatedAt", "username") SELECT "company", "createdAt", "email", "fullName", "id", "image", "password", "phone", "role", "updatedAt", "username" FROM "User";
DROP TABLE "User";
ALTER TABLE "new_User" RENAME TO "User";
CREATE UNIQUE INDEX "User_email_key" ON "User"("email");
PRAGMA foreign_key_check;
PRAGMA foreign_keys=ON;
