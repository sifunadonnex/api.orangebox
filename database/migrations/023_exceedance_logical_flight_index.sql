-- Match the logical-flight expression used by legacy-compatible read queries.
CREATE INDEX IF NOT EXISTS Exceedance_logical_flight_current_idx
ON Exceedance(COALESCE(flightLegId, flightId), isCurrent);
