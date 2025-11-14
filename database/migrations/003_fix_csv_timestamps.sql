-- Fix CSV timestamps that were stored as milliseconds instead of datetime strings
-- This migration converts existing Unix millisecond timestamps to proper datetime format
-- This migration is idempotent (safe to run multiple times)

-- Convert existing millisecond timestamps to datetime strings directly
-- Only update rows where the values look like millisecond timestamps (large integers > 1000000000000)
UPDATE Csv 
SET createdAt = datetime(CAST(createdAt AS INTEGER) / 1000, 'unixepoch', 'localtime')
WHERE (typeof(createdAt) = 'integer' AND CAST(createdAt AS INTEGER) > 1000000000000) 
   OR (typeof(createdAt) = 'text' AND length(createdAt) > 12 AND CAST(createdAt AS INTEGER) > 1000000000000);

UPDATE Csv 
SET updatedAt = datetime(CAST(updatedAt AS INTEGER) / 1000, 'unixepoch', 'localtime')
WHERE (typeof(updatedAt) = 'integer' AND CAST(updatedAt AS INTEGER) > 1000000000000)
   OR (typeof(updatedAt) = 'text' AND length(updatedAt) > 12 AND CAST(updatedAt AS INTEGER) > 1000000000000);
