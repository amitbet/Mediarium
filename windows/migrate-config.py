"""Copy a stopped Linux configuration to Windows, translating stored paths.

Usage: python windows/migrate-config.py SOURCE DESTINATION --data D:/Mediarium/data
SOURCE must contain a consistent app.db and its matching secret.key.
"""
import argparse
import json
import shutil
import sqlite3
from contextlib import closing
from pathlib import Path


def translate(value, mappings):
    if not isinstance(value, str):
        if isinstance(value, list):
            return [translate(v, mappings) for v in value]
        if isinstance(value, dict):
            return {k: translate(v, mappings) for k, v in value.items()}
        return value
    for old, new in mappings:
        if value == old or value.startswith(old + '/'):
            return new + value[len(old):]
    if value.startswith(('{', '[')):
        try:
            parsed = json.loads(value)
            changed = translate(parsed, mappings)
            if parsed != changed:
                return json.dumps(changed, ensure_ascii=False)
        except (ValueError, TypeError):
            pass
    return value


def migrate(source, destination, data, helper_url=None):
    if (destination / 'app.db').exists():
        raise ValueError('Destination already has a database; back it up first.')
    if not (source / 'app.db').is_file() or not (source / 'secret.key').is_file():
        raise ValueError('Both app.db and secret.key are required.')
    destination.mkdir(parents=True, exist_ok=True)
    for item in source.iterdir():
        if item.name in {'update', 'backups', 'app.db', 'app.db-wal', 'app.db-shm'} or item.name.startswith('pre-migration-'):
            continue
        if item.is_dir():
            shutil.copytree(item, destination / item.name, dirs_exist_ok=True)
        elif item.is_file():
            shutil.copy2(item, destination / item.name)
    with closing(sqlite3.connect(source / 'app.db')) as src, closing(sqlite3.connect(destination / 'app.db')) as db:
        src.backup(db)
        if db.execute('PRAGMA integrity_check').fetchone()[0] != 'ok':
            raise ValueError('Source database failed integrity check.')
        mappings = [('/data', data.as_posix()), ('/config', destination.as_posix())]
        mappings += [(f'/{name}', (data / name).as_posix()) for name in ('music', 'ebooks', 'audiobooks', 'movies', 'downloads')]
        mappings.append(('/tv', (data / 'series').as_posix()))
        quote = lambda s: '"' + s.replace('"', '""') + '"'
        total = 0
        tables = [r[0] for r in db.execute("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")]
        for table in tables:
            columns = [r[1] for r in db.execute('PRAGMA table_info(' + quote(table) + ')')]
            for row in db.execute('SELECT rowid,* FROM ' + quote(table)).fetchall():
                values = dict(zip(columns, row[1:]))
                if table == 'settings' and values.get('encrypted'):
                    continue
                changes = {k: translate(v, mappings) for k, v in values.items() if isinstance(v, str)}
                changes = {k: v for k, v in changes.items() if v != values[k]}
                if changes:
                    db.execute('UPDATE ' + quote(table) + ' SET ' + ','.join(quote(k) + '=?' for k in changes) + ' WHERE rowid=?', [*changes.values(), row[0]])
                    total += 1
        if helper_url:
            db.execute("UPDATE settings SET value=? WHERE key='flaresolverr.url' AND encrypted=0", (helper_url,))
        db.commit()
        assert db.execute('PRAGMA integrity_check').fetchone()[0] == 'ok'
    print(f'Migrated configuration; translated paths in {total} database rows. Encryption key preserved.')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path)
    parser.add_argument('destination', type=Path)
    parser.add_argument('--data', type=Path, required=True)
    parser.add_argument('--helper-url')
    args = parser.parse_args()
    migrate(args.source.resolve(), args.destination.resolve(), args.data.resolve(), args.helper_url)
