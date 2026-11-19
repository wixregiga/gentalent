package sqlite

const nameFixSchema = `
CREATE TABLE name_fixes(
  number INTEGER PRIMARY KEY,
  nomination INTEGER NOT NULL REFERENCES nominations(number),
  person_id TEXT NOT NULL REFERENCES persons(id),
  from_name TEXT NOT NULL,
  to_name TEXT NOT NULL,
  spelling TEXT NOT NULL,
  proposer TEXT NOT NULL,
  with_photos INTEGER NOT NULL DEFAULT 0 CHECK(with_photos IN (0, 1)),
  proposed_at TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('pending', 'fixed', 'stopped', 'withdrawn', 'lapsed')),
  ended_at TEXT NOT NULL DEFAULT '');
CREATE UNIQUE INDEX one_pending_name_fix_per_person ON name_fixes(person_id) WHERE status = 'pending';
CREATE UNIQUE INDEX name_fixed_once_per_person ON name_fixes(person_id) WHERE status = 'fixed';
CREATE TRIGGER keep_name_fixes_update BEFORE UPDATE ON name_fixes
  WHEN OLD.status <> 'pending' BEGIN SELECT RAISE(ABORT, 'Ended name fixes are permanent'); END;
CREATE TRIGGER keep_name_fixes_delete BEFORE DELETE ON name_fixes
  WHEN OLD.status <> 'pending' BEGIN SELECT RAISE(ABORT, 'Ended name fixes are permanent'); END;
CREATE TABLE name_fix_answers(
  number INTEGER NOT NULL REFERENCES name_fixes(number),
  member TEXT NOT NULL,
  answer TEXT NOT NULL CHECK(answer IN ('yes', 'no')),
  PRIMARY KEY(number, member));
CREATE TRIGGER keep_name_fix_answers_insert BEFORE INSERT ON name_fix_answers
  WHEN (SELECT status FROM name_fixes WHERE number = NEW.number) <> 'pending'
  BEGIN SELECT RAISE(ABORT, 'Answers to an ended name fix are permanent'); END;
CREATE TRIGGER keep_name_fix_answers_update BEFORE UPDATE ON name_fix_answers
  WHEN (SELECT status FROM name_fixes WHERE number = OLD.number) <> 'pending'
    OR (SELECT status FROM name_fixes WHERE number = NEW.number) <> 'pending'
  BEGIN SELECT RAISE(ABORT, 'Answers to an ended name fix are permanent'); END;
CREATE TRIGGER keep_name_fix_answers_delete BEFORE DELETE ON name_fix_answers
  WHEN (SELECT status FROM name_fixes WHERE number = OLD.number) <> 'pending'
  BEGIN SELECT RAISE(ABORT, 'Answers to an ended name fix are permanent'); END;
`
