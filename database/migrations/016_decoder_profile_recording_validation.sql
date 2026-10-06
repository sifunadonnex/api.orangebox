ALTER TABLE AircraftDecoderProfile ADD COLUMN validationRecordingName TEXT;
ALTER TABLE AircraftDecoderProfile ADD COLUMN validationRecordingChecksum TEXT;
ALTER TABLE AircraftDecoderProfile ADD COLUMN validatedBy TEXT REFERENCES User(id) ON DELETE SET NULL;
ALTER TABLE AircraftDecoderProfile ADD COLUMN validatedAt INTEGER;

CREATE INDEX idx_decoder_profile_validation_checksum
  ON AircraftDecoderProfile(validationRecordingChecksum);
