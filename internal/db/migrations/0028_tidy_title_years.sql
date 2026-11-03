-- Titles sent with the year twice ("2025 2025-26 Topps Chrome"): keep the
-- set name's own year. New and updated listings are tidied the same way.
UPDATE listings SET title = regexp_replace(title, '^(19|20)[0-9]{2}\s+((19|20)[0-9]{2})', '\2'), updated_at = now()
WHERE title ~ '^(19|20)[0-9]{2}\s+(19|20)[0-9]{2}';
