package sqlite

const schema = `
CREATE TABLE meta(key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE persons(id TEXT PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE aliases(alias TEXT PRIMARY KEY, person_id TEXT NOT NULL REFERENCES persons(id));
CREATE TABLE nominations(
  number INTEGER PRIMARY KEY,
  person_id TEXT NOT NULL REFERENCES persons(id),
  nominator TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('nominated', 'certified', 'rejected')),
  nominated_at TEXT NOT NULL DEFAULT '',
  locked_at TEXT NOT NULL DEFAULT '',
  finalized_at TEXT NOT NULL DEFAULT '',
  pitch TEXT NOT NULL DEFAULT '',
  photos TEXT NOT NULL DEFAULT '' CHECK(photos IN ('', 'requested', 'searching', 'found', 'failed')),
  strip TEXT NOT NULL DEFAULT '');
CREATE UNIQUE INDEX one_open_nomination_per_person ON nominations(person_id) WHERE status = 'nominated';
CREATE TRIGGER keep_outcomes_update BEFORE UPDATE OF number, person_id, nominator, status, finalized_at ON nominations
  WHEN OLD.status <> 'nominated' BEGIN SELECT RAISE(ABORT, 'Outcomes are permanent'); END;
CREATE TRIGGER keep_outcomes_delete BEFORE DELETE ON nominations
  WHEN OLD.status <> 'nominated' BEGIN SELECT RAISE(ABORT, 'Outcomes are permanent'); END;
CREATE TABLE votes(
  number INTEGER NOT NULL REFERENCES nominations(number),
  member TEXT NOT NULL,
  vote TEXT NOT NULL CHECK(vote IN ('yes', 'no')),
  PRIMARY KEY(number, member));
CREATE TABLE reactions(
  number INTEGER NOT NULL REFERENCES nominations(number),
  member TEXT NOT NULL,
  vote TEXT NOT NULL CHECK(vote IN ('yes', 'no')),
  PRIMARY KEY(number, member));
CREATE TABLE exhibits(
  id INTEGER PRIMARY KEY,
  number INTEGER NOT NULL REFERENCES nominations(number),
  text TEXT NOT NULL);
CREATE TABLE shames(
  id INTEGER PRIMARY KEY,
  number INTEGER NOT NULL REFERENCES nominations(number),
  shamer TEXT NOT NULL,
  target TEXT NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  target_vote TEXT CHECK(target_vote IN ('yes', 'no')),
  shamed_message_id TEXT,
  shamed_text TEXT,
  shamed_at TEXT,
  gif TEXT NOT NULL DEFAULT '',
  message_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(number, shamer));
CREATE TRIGGER keep_shames_update BEFORE UPDATE ON shames BEGIN SELECT RAISE(ABORT, 'Shames are permanent'); END;
CREATE TRIGGER keep_shames_delete BEFORE DELETE ON shames BEGIN SELECT RAISE(ABORT, 'Shames are permanent'); END;
CREATE TABLE feedback(
  id INTEGER PRIMARY KEY,
  message_id TEXT NOT NULL UNIQUE,
  sender TEXT NOT NULL,
  kind TEXT NOT NULL CHECK(kind IN ('feedback', 'suggestion')),
  text TEXT NOT NULL,
  said_at TEXT NOT NULL);
CREATE TABLE handled(message_id TEXT PRIMARY KEY);
CREATE TABLE outbox(
  seq INTEGER PRIMARY KEY,
  key TEXT NOT NULL UNIQUE,
  text TEXT NOT NULL,
  attachment TEXT NOT NULL DEFAULT '',
  announces INTEGER NOT NULL DEFAULT 0,
  relayed INTEGER NOT NULL DEFAULT 0);
`
