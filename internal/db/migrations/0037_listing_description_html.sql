-- Formatted listing descriptions: a cleaned, safe subset of HTML (headings,
-- paragraphs, lists, simple tables, links). The plain description column is
-- kept, filled from it, for search, previews and CSV.
ALTER TABLE listings ADD COLUMN description_html TEXT NOT NULL DEFAULT '';
