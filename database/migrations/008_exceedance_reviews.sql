-- Append-only occurrence review history and canonical review statuses.

UPDATE Exceedance SET eventStatus = 'Pending' WHERE eventStatus = 'Under Review';
UPDATE Exceedance SET eventStatus = 'False' WHERE eventStatus = 'Invalid';

CREATE TABLE ExceedanceReview (
    id TEXT NOT NULL PRIMARY KEY,
    exceedanceId TEXT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('status_change', 'archive')),
    previousStatus TEXT,
    newStatus TEXT,
    comment TEXT NOT NULL CHECK (length(trim(comment)) > 0),
    reviewedBy TEXT NOT NULL,
    createdAt INTEGER NOT NULL,
    FOREIGN KEY (exceedanceId) REFERENCES Exceedance(id) ON DELETE CASCADE,
    FOREIGN KEY (reviewedBy) REFERENCES User(id) ON DELETE RESTRICT
);

CREATE INDEX ExceedanceReview_exceedance_created_idx
    ON ExceedanceReview(exceedanceId, createdAt DESC);
CREATE INDEX ExceedanceReview_reviewer_idx
    ON ExceedanceReview(reviewedBy);
