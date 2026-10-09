import 'package:evomem_mobile/src/storage/database.dart';
import 'package:evomem_mobile/src/storage/notes_dao.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:sqflite/sqflite.dart';

import '../sqflite_test_setup.dart' as sqflite_setup;

/// The v3 `notes` table: what a phone that has been installed for a while
/// has on disk, with no `remote_id`.
const _v3Notes = '''
  CREATE TABLE notes (
    rowid INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL UNIQUE,
    project_id TEXT NOT NULL,
    content TEXT NOT NULL,
    source_type TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    metadata TEXT NOT NULL DEFAULT '{}'
  )
''';

void main() {
  sqflite_setup.setupSqfliteFfi();

  // The v3 file is written where the helper will look, before anything opens
  // it: this is the one test whose store has to be at a particular version
  // when the app's own open path runs.
  test(
    'upgrading an existing store adds remote_id and keeps the notes',
    () async {
      final dbPath = DatabaseHelper.databasePathOverride!;
      await deleteDatabase(dbPath);
      DatabaseHelper.instance.forgetConnection();

      // A store as version 3 left it.
      final old = await openDatabase(
        dbPath,
        version: 3,
        onCreate: (db, version) async {
          await db.execute(_v3Notes);
          await db.execute(
            'CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)',
          );
          await db.insert('meta', {'key': 'schema_version', 'value': '3'});
          await db.insert('notes', {
            'id': 'local-1',
            'project_id': 'evomem',
            'content': 'written before the upgrade',
            'source_type': 'manual',
            'created_at': '2026-01-01T00:00:00.000Z',
            'updated_at': '2026-01-01T00:00:00.000Z',
            'metadata': '{}',
          });
        },
      );
      await old.close();

      // Opened the way the app opens it, which runs the upgrade.
      final dao = NotesDao(DatabaseHelper.instance);
      final note = await dao.getById('local-1');

      expect(note, isNotNull, reason: 'the upgrade lost an existing note');
      expect(note!.content, 'written before the upgrade');
      // Empty, not null: the id the server minted for this note was discarded
      // and cannot be recovered, so it stays unattachable (ADR-0018).
      expect(note.remoteId, isEmpty);
      expect(note.isPushed, isFalse);
      // And nothing is known about what the mirror holds (ADR-0019).
      expect(note.remoteUpdatedAt, isNull);

      // And the column is writable, so the next push can record an id.
      await dao.setRemoteId(
        'local-1',
        '01M4D3H3HNMFM69N4MHNAYBZ1X',
        DateTime.utc(2026),
      );
      expect(
        (await dao.getById('local-1'))!.remoteId,
        '01M4D3H3HNMFM69N4MHNAYBZ1X',
      );

      final db = await DatabaseHelper.instance.database;
      expect(await db.getVersion(), 5);
      final meta = await db.query(
        'meta',
        where: 'key = ?',
        whereArgs: ['schema_version'],
      );
      expect(meta.first['value'], '5');
    },
  );
}
