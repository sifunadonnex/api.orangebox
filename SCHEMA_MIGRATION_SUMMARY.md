# EventLog Schema Migration Summary

## Overview
This migration updates the EventLog table to support the new event trigger and severity configuration system from the frontend.

## Database Changes

### Migration File
**Location:** `database/migrations/002_update_eventlog_schema.sql`

### New Columns Added
1. `triggerType` (TEXT) - Stores "single" or "calculation" to indicate trigger type
2. `detectionPeriod` (TEXT) - Stores JSON string for detection period configuration
3. `severities` (TEXT) - Stores JSON array of severity configurations

### Default Values
- Existing records have been populated with default values:
  - `triggerType = 'single'`
  - `detectionPeriod = 'allFlight'`
  - `severities = '[]'`

## Backend Code Changes

### Models (`models/models.go`)

#### EventLog Struct
Added three new fields to the EventLog struct:
```go
TriggerType      *string   `json:"triggerType" db:"triggerType"`
DetectionPeriod  *string   `json:"detectionPeriod" db:"detectionPeriod"`
Severities       *string   `json:"severities" db:"severities"`
```

#### Request DTOs
Updated both `CreateEventRequest` and `UpdateEventRequest` with the same three fields:
```go
TriggerType      *string `json:"triggerType,omitempty"`
DetectionPeriod  *string `json:"detectionPeriod,omitempty"`
Severities       *string `json:"severities,omitempty"`
```

### Handlers (`handlers/event.go`)

#### CreateEvent
- **INSERT Query:** Added 3 new columns (triggerType, detectionPeriod, severities)
- **Parameter Binding:** Added req.TriggerType, req.DetectionPeriod, req.Severities
- **Response Object:** Includes all three new fields in returned event

#### GetEvents
- **SELECT Query:** Added triggerType, detectionPeriod, severities to SELECT clause
- **Row Scanning:** Added &event.TriggerType, &event.DetectionPeriod, &event.Severities

#### GetEventByID
- **SELECT Query:** Added triggerType, detectionPeriod, severities to SELECT clause
- **Row Scanning:** Added &event.TriggerType, &event.DetectionPeriod, &event.Severities

#### UpdateEvent
- **UPDATE Query:** Added triggerType = ?, detectionPeriod = ?, severities = ? to SET clause
- **Parameter Binding:** Added req.TriggerType, req.DetectionPeriod, req.Severities
- **Response Object:** Includes all three new fields in returned event

## Data Structure Examples

### Frontend Submission Format
```javascript
{
  // Existing fields...
  triggerType: "calculation", // or "single"
  eventTrigger: "(PARAM1 - PARAM2) > 5", // formula string
  detectionPeriod: "specificCondition", // or "allFlight"
  severities: JSON.stringify([
    {
      id: 1,
      level: "Low",
      operator: ">",
      value: "10",
      duration: "2",
      enabled: true
    },
    {
      id: 2,
      level: "Medium",
      operator: ">",
      value: "20",
      duration: "5",
      enabled: true
    }
  ])
}
```

### Database Storage
- `triggerType`: "calculation"
- `eventTrigger`: "(PARAM1 - PARAM2) > 5"
- `detectionPeriod`: "specificCondition"
- `severities`: "[{\"id\":1,\"level\":\"Low\",\"operator\":\">\",\"value\":\"10\",\"duration\":\"2\",\"enabled\":true}]"

## Backward Compatibility

The migration maintains backward compatibility:
- Old severity fields (high, high1, high2, low, low1, low2) are retained
- Existing records receive default values for new fields
- New fields are nullable (*string) allowing gradual migration

## Next Steps

### Exceedance Detection Logic
The next phase requires updating the exceedance detection logic to:

1. **Parse triggerType** and handle two execution paths:
   - Single: Evaluate simple comparisons (e.g., "ALT > 10000")
   - Calculation: Parse and evaluate complex formulas (e.g., "(PARAM1 - PARAM2) > 5")

2. **Parse eventTrigger** formula strings:
   - Extract parameter names
   - Extract operators (+, -, *, /, >, <, >=, <=, ==, !=)
   - Extract comparison values
   - Build evaluation logic

3. **Parse detectionPeriod** JSON:
   - Determine if check applies to all flight or specific conditions
   - If specific, evaluate condition parameter first

4. **Parse severities** JSON array:
   - Load all enabled severity levels
   - For each exceedance, determine which severity applies
   - Use operator, value, and duration to classify severity
   - Store appropriate exceedanceLevel in Exceedance table

5. **Implement formula evaluator**:
   - Create parser for mathematical expressions
   - Support parentheses and operator precedence
   - Validate parameter availability in CSV data
   - Handle missing or invalid parameter values gracefully

## Migration Execution

The migration was successfully executed:
```
2025/11/06 14:32:09 Running migration: database\migrations\002_update_eventlog_schema.sql
2025/11/06 14:32:10 Successfully executed migration: database\migrations\002_update_eventlog_schema.sql
```

<<<<<<< HEAD
Server is running on: https://api-orangebox.onrender.com
=======
Server is running on: http://localhost:8000
>>>>>>> f6ca660653316c3bef06d307e2aac058e5034247

## Testing Recommendations

1. **Create new event** with new fields via POST /events
2. **Retrieve events** and verify new fields are included
3. **Update existing event** with new configuration
4. **Verify backward compatibility** with old events that have null values
5. **Test formula parsing** before implementing exceedance detection
