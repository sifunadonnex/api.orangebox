-- Controlled self-service recording uploads: retain provenance and reject
-- identical recorder files for the same aircraft.
ALTER TABLE Csv ADD COLUMN originalFilename TEXT;
ALTER TABLE Csv ADD COLUMN contentHash TEXT;
ALTER TABLE Csv ADD COLUMN uploadedBy TEXT REFERENCES User(id) ON DELETE SET NULL;
ALTER TABLE Csv ADD COLUMN uploadSource TEXT NOT NULL DEFAULT 'legacy'
  CHECK (uploadSource IN ('legacy', 'client_portal', 'oversight_portal'));

CREATE UNIQUE INDEX idx_csv_aircraft_content_hash
  ON Csv(aircraftId, contentHash)
  WHERE contentHash IS NOT NULL;
