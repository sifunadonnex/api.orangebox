/*
  Warnings:

  - You are about to drop the column `aicraftId` on the `Csv` table. All the data in the column will be lost.
  - You are about to drop the column `name` on the `Aircraft` table. All the data in the column will be lost.
  - Added the required column `aircraftId` to the `Csv` table without a default value. This is not possible if the table is not empty.
  - Added the required column `file` to the `Csv` table without a default value. This is not possible if the table is not empty.
  - Added the required column `aircraftMake` to the `Aircraft` table without a default value. This is not possible if the table is not empty.
  - Added the required column `airline` to the `Aircraft` table without a default value. This is not possible if the table is not empty.
  - Added the required column `serialNumber` to the `Aircraft` table without a default value. This is not possible if the table is not empty.
  - Added the required column `tailNumner` to the `Aircraft` table without a default value. This is not possible if the table is not empty.

*/
-- AlterTable
ALTER TABLE "User" ADD COLUMN "image" TEXT;

-- CreateTable
CREATE TABLE "EventLog" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "flightPhase" TEXT NOT NULL,
    "parameterRef" TEXT NOT NULL,
    "parameterName" TEXT NOT NULL,
    "description" TEXT NOT NULL,
    "range" TEXT NOT NULL,
    "trigger" TEXT NOT NULL,
    "eventMarker" TEXT NOT NULL,
    "OEMLimit" TEXT NOT NULL,
    "userLimitLow" TEXT NOT NULL,
    "userLimitHigh" TEXT NOT NULL,
    "userLimitMedium" TEXT NOT NULL,
    "eventStatus" TEXT NOT NULL,
    "aircraftId" INTEGER NOT NULL,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" DATETIME NOT NULL,
    CONSTRAINT "EventLog_aircraftId_fkey" FOREIGN KEY ("aircraftId") REFERENCES "Aircraft" ("id") ON DELETE RESTRICT ON UPDATE CASCADE
);

-- CreateTable
CREATE TABLE "Exceedance" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "flightPhase" TEXT NOT NULL,
    "parameterRef" TEXT NOT NULL,
    "parameterName" TEXT NOT NULL,
    "description" TEXT NOT NULL,
    "range" TEXT NOT NULL,
    "trigger" TEXT NOT NULL,
    "eventMarker" TEXT NOT NULL,
    "OEMLimit" TEXT NOT NULL,
    "userLimitLow" TEXT NOT NULL,
    "userLimitHigh" TEXT NOT NULL,
    "userLimitMedium" TEXT NOT NULL,
    "eventStatus" TEXT NOT NULL,
    "aircraftId" INTEGER NOT NULL,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" DATETIME NOT NULL,
    CONSTRAINT "Exceedance_aircraftId_fkey" FOREIGN KEY ("aircraftId") REFERENCES "Aircraft" ("id") ON DELETE RESTRICT ON UPDATE CASCADE
);

-- RedefineTables
PRAGMA foreign_keys=OFF;
CREATE TABLE "new_Csv" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "name" TEXT NOT NULL,
    "file" TEXT NOT NULL,
    "aircraftId" INTEGER NOT NULL,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" DATETIME NOT NULL,
    CONSTRAINT "Csv_aircraftId_fkey" FOREIGN KEY ("aircraftId") REFERENCES "Aircraft" ("id") ON DELETE RESTRICT ON UPDATE CASCADE
);
INSERT INTO "new_Csv" ("createdAt", "id", "name", "updatedAt") SELECT "createdAt", "id", "name", "updatedAt" FROM "Csv";
DROP TABLE "Csv";
ALTER TABLE "new_Csv" RENAME TO "Csv";
CREATE TABLE "new_Aircraft" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "airline" TEXT NOT NULL,
    "aircraftMake" TEXT NOT NULL,
    "serialNumber" TEXT NOT NULL,
    "tailNumner" TEXT NOT NULL,
    "userId" INTEGER NOT NULL,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" DATETIME NOT NULL,
    CONSTRAINT "Aircraft_userId_fkey" FOREIGN KEY ("userId") REFERENCES "User" ("id") ON DELETE RESTRICT ON UPDATE CASCADE
);
INSERT INTO "new_Aircraft" ("createdAt", "id", "updatedAt", "userId") SELECT "createdAt", "id", "updatedAt", "userId" FROM "Aircraft";
DROP TABLE "Aircraft";
ALTER TABLE "new_Aircraft" RENAME TO "Aircraft";
PRAGMA foreign_key_check;
PRAGMA foreign_keys=ON;
