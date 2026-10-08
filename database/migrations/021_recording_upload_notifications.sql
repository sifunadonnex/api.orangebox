-- Recording review notifications share the existing inbox with event notifications.
-- A notification targets exactly one recording or one exceedance.
BEGIN TRANSACTION;

ALTER TABLE Notification RENAME TO _recording_upload_Notification;
DROP INDEX Notification_userId_idx;
DROP INDEX Notification_exceedanceId_idx;

CREATE TABLE Notification (
    id TEXT NOT NULL PRIMARY KEY,
    userId TEXT NOT NULL,
    exceedanceId TEXT,
    recordingId TEXT,
    message TEXT NOT NULL,
    level TEXT NOT NULL,
    isRead INTEGER NOT NULL DEFAULT 0,
    createdAt INTEGER NOT NULL,
    updatedAt INTEGER NOT NULL,
    CHECK ((exceedanceId IS NOT NULL) != (recordingId IS NOT NULL)),
    FOREIGN KEY (userId) REFERENCES User(id) ON DELETE CASCADE,
    FOREIGN KEY (exceedanceId) REFERENCES Exceedance(id) ON DELETE CASCADE,
    FOREIGN KEY (recordingId) REFERENCES Csv(id) ON DELETE CASCADE
);

INSERT INTO Notification (id, userId, exceedanceId, message, level, isRead, createdAt, updatedAt)
SELECT id, userId, exceedanceId, message, level, isRead, createdAt, updatedAt
FROM _recording_upload_Notification;

CREATE INDEX Notification_userId_idx ON Notification(userId);
CREATE INDEX Notification_exceedanceId_idx ON Notification(exceedanceId);
CREATE INDEX Notification_recordingId_idx ON Notification(recordingId);

DROP TABLE _recording_upload_Notification;
COMMIT;
