import importlib.util
import json
import sqlite3
import tempfile
import unittest
from contextlib import closing
from pathlib import Path

spec = importlib.util.spec_from_file_location('migration', Path(__file__).with_name('migrate-config.py'))
migration = importlib.util.module_from_spec(spec)
spec.loader.exec_module(migration)


class MigrationTest(unittest.TestCase):
    def test_paths_json_credentials_and_key(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / 'source'
            source.mkdir()
            (source / 'secret.key').write_bytes(b'test-key')
            with closing(sqlite3.connect(source / 'app.db')) as db:
                db.execute('CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT, encrypted INTEGER)')
                db.executemany('INSERT INTO settings VALUES(?,?,?)', [
                    ('library.movies_path', '/data/movies', 0),
                    ('secret', '/data/do-not-touch', 1),
                    ('json', json.dumps({'path': '/config/thing', 'url': 'https://host/data/file'}), 0),
                ])
                db.commit()
            migration.migrate(source, root / 'dest', root / 'data')
            with closing(sqlite3.connect(root / 'dest/app.db')) as db:
                rows = dict(db.execute('SELECT key,value FROM settings'))
                self.assertEqual(rows['library.movies_path'], (root / 'data/movies').as_posix())
                self.assertEqual(rows['secret'], '/data/do-not-touch')
                self.assertEqual(json.loads(rows['json'])['path'], (root / 'dest/thing').as_posix())
                self.assertEqual(json.loads(rows['json'])['url'], 'https://host/data/file')
                self.assertEqual(db.execute('PRAGMA integrity_check').fetchone()[0], 'ok')
            self.assertEqual((root / 'dest/secret.key').read_bytes(), b'test-key')
            with self.assertRaises(ValueError):
                migration.migrate(source, root / 'dest', root / 'data')


if __name__ == '__main__':
    unittest.main()
