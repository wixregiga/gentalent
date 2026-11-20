package sqlite

const whoSearchSchema = `
CREATE TABLE who_searches(
  number INTEGER PRIMARY KEY,
  searcher TEXT NOT NULL,
  description TEXT NOT NULL CHECK(description <> ''),
  started_at TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('requested', 'searching', 'found', 'no-match', 'failed')),
  figure_name TEXT NOT NULL DEFAULT '',
  figure_spelling TEXT NOT NULL DEFAULT '',
  blurb TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  photo TEXT NOT NULL DEFAULT '',
  provider TEXT NOT NULL DEFAULT '',
  person_id TEXT REFERENCES persons(id),
  CHECK((status = 'found') = (figure_name <> '' AND figure_spelling <> '')),
  CHECK(person_id IS NULL OR status = 'found'));
CREATE INDEX who_searches_by_searcher ON who_searches(searcher, started_at);
CREATE TRIGGER keep_who_searches_update BEFORE UPDATE ON who_searches
  WHEN OLD.status IN ('found', 'no-match', 'failed') BEGIN SELECT RAISE(ABORT, 'Ended who searches are permanent'); END;
CREATE TRIGGER keep_who_searches_delete BEFORE DELETE ON who_searches
  BEGIN SELECT RAISE(ABORT, 'Who searches are permanent'); END;
`
