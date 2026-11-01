ALTER TABLE artifacts ADD COLUMN sha256 text NOT NULL DEFAULT '' CHECK(sha256='' OR sha256~'^[0-9a-f]{64}$');
