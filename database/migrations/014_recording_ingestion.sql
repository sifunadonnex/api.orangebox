-- Track how an uploaded recorder file was normalized before authoritative
-- phase and exceedance analysis.
ALTER TABLE Csv ADD COLUMN sourceFormat TEXT NOT NULL DEFAULT 'csv'
  CHECK (sourceFormat IN ('csv', 'gzip', 'zip'));
ALTER TABLE Csv ADD COLUMN sourceEntry TEXT;
ALTER TABLE Csv ADD COLUMN normalizedBytes INTEGER;
