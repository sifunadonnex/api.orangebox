CREATE TABLE AircraftDecoderProfile (
  id TEXT PRIMARY KEY,
  aircraftId TEXT NOT NULL REFERENCES Aircraft(id) ON DELETE CASCADE,
  version INTEGER NOT NULL,
  name TEXT NOT NULL,
  parameterFormat TEXT NOT NULL CHECK (parameterFormat IN ('tbx', 'prm', 'json', 'fred')),
  parameterFileName TEXT NOT NULL,
  parameterText TEXT NOT NULL,
  decoderConfig TEXT NOT NULL DEFAULT '{}',
  checksum TEXT NOT NULL,
  notes TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'retired')),
  validationStatus TEXT NOT NULL DEFAULT 'failed' CHECK (validationStatus IN ('passed', 'failed')),
  validationSummary TEXT NOT NULL,
  createdBy TEXT REFERENCES User(id) ON DELETE SET NULL,
  publishedBy TEXT REFERENCES User(id) ON DELETE SET NULL,
  createdAt INTEGER NOT NULL,
  publishedAt INTEGER,
  UNIQUE (aircraftId, version)
);

CREATE UNIQUE INDEX idx_decoder_profile_one_published
  ON AircraftDecoderProfile(aircraftId)
  WHERE status = 'published';

CREATE INDEX idx_decoder_profile_aircraft_version
  ON AircraftDecoderProfile(aircraftId, version DESC);
