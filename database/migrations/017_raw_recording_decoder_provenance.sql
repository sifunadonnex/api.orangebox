-- Preserve the immutable raw-recorder source and the exact published decoder
-- profile used to create the canonical CSV analysis derivative.
ALTER TABLE Csv ADD COLUMN rawSourceFormat TEXT
  CHECK (rawSourceFormat IS NULL OR rawSourceFormat IN ('ddf', 'dat', 'raw', 'bin'));
ALTER TABLE Csv ADD COLUMN rawSourceFile TEXT;
ALTER TABLE Csv ADD COLUMN decoderProfileId TEXT REFERENCES AircraftDecoderProfile(id) ON DELETE SET NULL;
ALTER TABLE Csv ADD COLUMN decoderProfileVersion INTEGER;
ALTER TABLE Csv ADD COLUMN decoderProfileChecksum TEXT;

CREATE INDEX idx_csv_decoder_profile
  ON Csv(decoderProfileId, decoderProfileVersion);
