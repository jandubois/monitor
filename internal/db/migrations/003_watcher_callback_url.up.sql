-- Add callback_url to watchers for direct trigger notifications
ALTER TABLE watchers ADD COLUMN callback_url TEXT;
