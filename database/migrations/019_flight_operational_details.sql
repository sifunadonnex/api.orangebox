-- Optional operational metadata entered after automatic flight detection.
-- Values remain attached to the logical flight leg so multi-flight recorder
-- uploads can carry different crew, load-sheet, and reference-speed details.

ALTER TABLE FlightLeg ADD COLUMN flightDate TEXT;
ALTER TABLE FlightLeg ADD COLUMN flightNumber TEXT;
ALTER TABLE FlightLeg ADD COLUMN picCrewCode TEXT;
ALTER TABLE FlightLeg ADD COLUMN foCrewCode TEXT;
ALTER TABLE FlightLeg ADD COLUMN techLogReference TEXT;
ALTER TABLE FlightLeg ADD COLUMN loadSheetNumber TEXT;
ALTER TABLE FlightLeg ADD COLUMN takeoffWeight REAL CHECK (takeoffWeight IS NULL OR takeoffWeight >= 0);
ALTER TABLE FlightLeg ADD COLUMN landingWeight REAL CHECK (landingWeight IS NULL OR landingWeight >= 0);
ALTER TABLE FlightLeg ADD COLUMN weightUnit TEXT CHECK (weightUnit IS NULL OR weightUnit IN ('kg', 'lb'));
ALTER TABLE FlightLeg ADD COLUMN v1 REAL CHECK (v1 IS NULL OR v1 >= 0);
ALTER TABLE FlightLeg ADD COLUMN vr REAL CHECK (vr IS NULL OR vr >= 0);
ALTER TABLE FlightLeg ADD COLUMN v2 REAL CHECK (v2 IS NULL OR v2 >= 0);
ALTER TABLE FlightLeg ADD COLUMN vref REAL CHECK (vref IS NULL OR vref >= 0);
ALTER TABLE FlightLeg ADD COLUMN vapp REAL CHECK (vapp IS NULL OR vapp >= 0);
ALTER TABLE FlightLeg ADD COLUMN notes TEXT;

UPDATE FlightLeg
SET picCrewCode = UPPER(TRIM(pilot))
WHERE picCrewCode IS NULL AND pilot IS NOT NULL AND TRIM(pilot) <> '';
