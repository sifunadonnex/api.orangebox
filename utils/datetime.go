package utils

import (
	"database/sql"
	"time"
)

// ConvertSQLiteTimestamp converts SQLite timestamp (int64 milliseconds) to time.Time
func ConvertSQLiteTimestamp(timestamp sql.NullInt64) time.Time {
	if timestamp.Valid {
		return time.Unix(timestamp.Int64/1000, 0)
	}
	return time.Time{}
}

// ConvertSQLiteTimestampPtr converts SQLite timestamp to *time.Time
func ConvertSQLiteTimestampPtr(timestamp sql.NullInt64) *time.Time {
	if timestamp.Valid {
		t := time.Unix(timestamp.Int64/1000, 0)
		return &t
	}
	return nil
}