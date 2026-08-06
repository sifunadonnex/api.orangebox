package handlers

import (
	"database/sql/driver"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// nullableTimestamp accepts every timestamp representation used by the
// historical Prisma tables and the newer Go migrations. It prevents a valid
// database value from silently becoming Go's year-one zero time.
type nullableTimestamp struct {
	Time  time.Time
	Valid bool
}

func (timestamp *nullableTimestamp) Scan(value any) error {
	if value == nil {
		timestamp.Time = time.Time{}
		timestamp.Valid = false
		return nil
	}

	var (
		parsed time.Time
		err    error
	)
	switch typed := value.(type) {
	case time.Time:
		parsed = typed
	case int64:
		parsed, err = unixTimestamp(typed)
	case float64:
		if math.Trunc(typed) != typed {
			err = fmt.Errorf("timestamp number %v is not an integer", typed)
		} else {
			parsed, err = unixTimestamp(int64(typed))
		}
	case []byte:
		parsed, err = parseTimestamp(string(typed))
	case string:
		parsed, err = parseTimestamp(typed)
	default:
		err = fmt.Errorf("unsupported timestamp type %T", value)
	}
	if err != nil {
		timestamp.Time = time.Time{}
		timestamp.Valid = false
		return err
	}

	timestamp.Time = parsed
	timestamp.Valid = true
	return nil
}

func (timestamp nullableTimestamp) Value() (driver.Value, error) {
	if !timestamp.Valid {
		return nil, nil
	}
	return databaseTimestamp(timestamp.Time), nil
}

func databaseTimestamp(value time.Time) string {
	return value.UTC().Round(0).Format(time.RFC3339Nano)
}

func parseTimestamp(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("timestamp is empty")
	}
	if numeric, err := strconv.ParseInt(value, 10, 64); err == nil {
		return unixTimestamp(numeric)
	}

	// time.Time.String may contain a process-local monotonic suffix. It has no
	// calendar meaning and must be removed before parsing persisted legacy rows.
	if monotonicIndex := strings.Index(value, " m="); monotonicIndex >= 0 {
		value = value[:monotonicIndex]
	}

	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported timestamp %q", value)
}

func unixTimestamp(value int64) (time.Time, error) {
	absValue := value
	if absValue < 0 {
		absValue = -absValue
	}

	var parsed time.Time
	switch {
	case absValue >= 100_000_000_000_000_000:
		parsed = time.Unix(0, value)
	case absValue >= 100_000_000_000_000:
		parsed = time.UnixMicro(value)
	case absValue >= 100_000_000_000:
		parsed = time.UnixMilli(value)
	default:
		parsed = time.Unix(value, 0)
	}
	if parsed.Year() < 1970 || parsed.Year() > 9999 {
		return time.Time{}, fmt.Errorf("timestamp %d is outside the supported calendar range", value)
	}
	return parsed.UTC(), nil
}
