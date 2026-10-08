import 'package:evomem_mobile/src/rules/note.dart';
import 'package:evomem_mobile/src/storage/database.dart';
import 'package:evomem_mobile/src/storage/notes_dao.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:path/path.dart' as path;
import 'package:sqflite/sqflite.dart';

import '../sqflite_test_setup.dart' as sqflite_setup;

void main() {
  sqflite_setup.setupSqfliteFfi();

  late NotesDao dao;
  late DatabaseHelper helper;

  setUpAll(() async {
    // A file of its own: `flutter test` runs test files in parallel against
    // one temporary directory, so sharing the default name means sharing a
    // store with whatever else is running.
    DatabaseHelper.databasePathOverride = path.join(
      await getDatabasesPath(),
      'remote_id_column.db',
    );
    DatabaseHelper.instance.forgetConnection();
  });

  tearDownAll(() {
    DatabaseHelper.databasePathOverride = null;
    DatabaseHelper.instance.forgetConnection();
  });

  setUp(() async {
    helper = DatabaseHelper.instance;
    dao = NotesDao(helper);
    final db = await helper.database;
    await db.delete('notes');
  });

  Note note(String id, {String remoteId = ''}) => Note(
    id: id,
    projectId: 'evomem',
    content: 'buy milk',
    sourceType: 'manual',
    createdAt: DateTime.utc(2026, 10, 8),
    updatedAt: DateTime.utc(2026, 10, 8),
    remoteId: remoteId,
  );

  test('a note starts with no remote id', () async {
    await dao.insert(note('local-1'));
    final stored = await dao.getById('local-1');
    expect(stored!.remoteId, isEmpty);
    expect(stored.isPushed, isFalse);
  });

  test('a remote id survives a round trip', () async {
    await dao.insert(note('local-1', remoteId: '01M4D3H3HNMFM69N4MHNAYBZ1X'));
    final stored = await dao.getById('local-1');
    expect(stored!.remoteId, '01M4D3H3HNMFM69N4MHNAYBZ1X');
  });

  test('setRemoteId writes only that column', () async {
    final original = note('local-1');
    await dao.insert(original);
    await dao.setRemoteId('local-1', '01M4D3H3HNMFM69N4MHNAYBZ1X');

    final stored = await dao.getById('local-1');
    expect(stored!.remoteId, '01M4D3H3HNMFM69N4MHNAYBZ1X');
    expect(stored.content, original.content);
    expect(stored.updatedAt, original.updatedAt);
  });

  test('the schema has the column with an empty default', () async {
    final db = await helper.database;
    // Written without the column, the way an older row was.
    await db.rawInsert(
      'INSERT INTO notes (id, project_id, content, source_type, '
      'created_at, updated_at, metadata) VALUES (?, ?, ?, ?, ?, ?, ?)',
      [
        'legacy-1',
        'evomem',
        'older',
        'manual',
        '2026-01-01',
        '2026-01-01',
        '{}',
      ],
    );
    final stored = await dao.getById('legacy-1');
    expect(stored!.remoteId, isEmpty);
    expect(stored.isPushed, isFalse);
  });

  test('withContent and withRemoteId each keep the other', () async {
    final pushed = note('local-1', remoteId: '01M4D3H3HNMFM69N4MHNAYBZ1X');
    final edited = pushed.withContent('walk dog', DateTime.utc(2026, 10, 9));
    expect(edited.remoteId, pushed.remoteId);
    expect(edited.content, 'walk dog');

    final named = pushed.withRemoteId('01M4D3H3HNMFM69N4MHNAYBZ1Y');
    expect(named.content, pushed.content);
    expect(named.updatedAt, pushed.updatedAt);
    expect(named.remoteId, '01M4D3H3HNMFM69N4MHNAYBZ1Y');
  });
}
