CREATE TABLE IF NOT EXISTS SafetyIndicatorTarget (
    id TEXT PRIMARY KEY,
    companyId TEXT NOT NULL,
    indicatorKey TEXT NOT NULL,
    target REAL,
    alert REAL,
    createdAt INTEGER NOT NULL,
    updatedAt INTEGER NOT NULL,
    FOREIGN KEY (companyId) REFERENCES Company(id) ON DELETE CASCADE,
    UNIQUE(companyId, indicatorKey)
);

CREATE INDEX IF NOT EXISTS SafetyIndicatorTarget_company_idx
    ON SafetyIndicatorTarget(companyId);
