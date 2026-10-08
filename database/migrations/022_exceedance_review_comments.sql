-- Allow an audited review comment without requiring a status change.
-- SQLite cannot alter a CHECK constraint in place, so copy the existing rows.
BEGIN TRANSACTION;

CREATE TABLE ExceedanceReview_new (
    id TEXT NOT NULL PRIMARY KEY,
    exceedanceId TEXT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('status_change', 'comment', 'archive')),
    previousStatus TEXT,
    newStatus TEXT,
    comment TEXT NOT NULL CHECK (length(trim(comment)) > 0),
    reviewedBy TEXT NOT NULL,
    createdAt INTEGER NOT NULL,
    FOREIGN KEY (exceedanceId) REFERENCES Exceedance(id) ON DELETE CASCADE,
    FOREIGN KEY (reviewedBy) REFERENCES User(id) ON DELETE RESTRICT
);

INSERT INTO ExceedanceReview_new
    (id, exceedanceId, action, previousStatus, newStatus, comment, reviewedBy, createdAt)
SELECT id, exceedanceId, action, previousStatus, newStatus, comment, reviewedBy, createdAt
FROM ExceedanceReview;

DROP TABLE ExceedanceReview;
ALTER TABLE ExceedanceReview_new RENAME TO ExceedanceReview;

CREATE INDEX ExceedanceReview_exceedance_created_idx
    ON ExceedanceReview(exceedanceId, createdAt DESC);
CREATE INDEX ExceedanceReview_reviewer_idx
    ON ExceedanceReview(reviewedBy);

COMMIT;
