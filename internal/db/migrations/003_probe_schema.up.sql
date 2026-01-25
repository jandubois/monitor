-- Add output schema and default interval to probe types
ALTER TABLE probe_types ADD COLUMN output TEXT;  -- JSON output schema
ALTER TABLE probe_types ADD COLUMN default_interval TEXT;

-- Add summary to probe results
ALTER TABLE probe_results ADD COLUMN summary TEXT;
