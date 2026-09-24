-- Persist the best available recorded position for each detected occurrence.
-- Location is optional: missing or unreliable coordinates remain visible in
-- reporting coverage instead of being silently discarded.

CREATE TABLE ExceedanceLocation (
    exceedanceId TEXT NOT NULL PRIMARY KEY,
    latitude REAL NOT NULL CHECK (latitude >= -90 AND latitude <= 90),
    longitude REAL NOT NULL CHECK (longitude >= -180 AND longitude <= 180),
    altitude REAL,
    matchedTimeMs INTEGER NOT NULL,
    timeDeltaMs INTEGER NOT NULL CHECK (timeDeltaMs >= 0),
    source TEXT NOT NULL CHECK (source IN ('replay_nearest_time')),
    quality TEXT NOT NULL CHECK (quality IN ('exact', 'near')),
    createdAt INTEGER NOT NULL,
    updatedAt INTEGER NOT NULL,
    FOREIGN KEY (exceedanceId) REFERENCES Exceedance(id) ON DELETE CASCADE
);

CREATE INDEX ExceedanceLocation_coordinates_idx
    ON ExceedanceLocation(latitude, longitude);
