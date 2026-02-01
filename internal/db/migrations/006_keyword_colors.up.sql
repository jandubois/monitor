-- Keyword colors table
CREATE TABLE IF NOT EXISTS keyword_colors (
    keyword TEXT PRIMARY KEY,
    color_index INTEGER NOT NULL DEFAULT 0
);
