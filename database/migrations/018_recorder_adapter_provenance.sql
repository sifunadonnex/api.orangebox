-- Record the original recorder container separately from the normalized raw
-- stream category and identify the adapter used before FRED decoding.
ALTER TABLE Csv ADD COLUMN recorderContainerFormat TEXT;
ALTER TABLE Csv ADD COLUMN recorderAdapter TEXT;

CREATE INDEX idx_csv_recorder_adapter
  ON Csv(recorderAdapter);
