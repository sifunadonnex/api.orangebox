/*
  Warnings:

  - You are about to drop the column `tailNumner` on the `Aircraft` table. All the data in the column will be lost.

*/
-- RedefineTables
PRAGMA foreign_keys=OFF;
CREATE TABLE "new_Aircraft" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "airline" TEXT NOT NULL,
    "aircraftMake" TEXT NOT NULL,
    "serialNumber" TEXT NOT NULL,
    "userId" INTEGER NOT NULL,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" DATETIME NOT NULL,
    CONSTRAINT "Aircraft_userId_fkey" FOREIGN KEY ("userId") REFERENCES "User" ("id") ON DELETE RESTRICT ON UPDATE CASCADE
);
INSERT INTO "new_Aircraft" ("aircraftMake", "airline", "createdAt", "id", "serialNumber", "updatedAt", "userId") SELECT "aircraftMake", "airline", "createdAt", "id", "serialNumber", "updatedAt", "userId" FROM "Aircraft";
DROP TABLE "Aircraft";
ALTER TABLE "new_Aircraft" RENAME TO "Aircraft";
PRAGMA foreign_key_check;
PRAGMA foreign_keys=ON;
