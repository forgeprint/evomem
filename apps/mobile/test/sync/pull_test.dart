import 'dart:convert';
import 'dart:io';

import 'package:evomem_mobile/src/audio/recording_store_io.dart';
import 'package:evomem_mobile/src/rules/note.dart';
import 'package:evomem_mobile/src/storage/database.dart';
import 'package:evomem_mobile/src/storage/notes_dao.dart';
import 'package:evomem_mobile/src/sync/sync_service.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import '../sqflite_test_setup.dart' as sqflite_setup;

/// A stand-in for the server's pull endpoints, with the cursor semantics the
/// Go side has: oldest first, and the cursor comes back even when nothing
/// did.
///
/// A real server rather than a mocked client, like the other sync tests:
/// `SyncService` builds its own `http.Client` and the thing under test is
/// what it does with a reply.
class FakePull {
  new(this._server) {
    _server.listen((request) async {
      requests.add(request.uri.toString());
      final since = request.uri.queryParameters['since'] ?? '';
      final sinceId = request.uri.queryParameters['since_id'] ?? '';

      final page = switch (request.uri.path) {
        '/sync/notes' => _after(notes, since, sinceId, (n) => n['updated_at']!),
        '/sync/deletions' => _after(
          deletions,
          since,
          sinceId,
          (d) => d['deleted_at']!,
        ),
        _ => <Map<String, String>>[],
      };

      final key = request.uri.path == '/sync/notes' ? 'notes' : 'deletions';
      final timeField = request.uri.path == '/sync/notes'
          ? 'updated_at'
          : 'deleted_at';

      final body = <String, Object?>{
        key: page
            .map(
              (row) => request.uri.path == '/sync/notes'
                  ? <String, Object?>{
                      'id': row['id'],
                      'project_id': 'evomem',
                      'content': row['content'],
                      'source_type': 'manual',
                      'created_at': row['updated_at'],
                      'updated_at': row['updated_at'],
                      'metadata': <String, Object?>{},
                    }
                  : <String, Object?>{
                      'id': row['id'],
                      'project_id': 'evomem',
                      'deleted_at': row['deleted_at'],
                    },
            )
            .toList(),
        // Caught up hands back where the client already is, never an empty
        // string — a client that stored one would start from the beginning.
        'cursor': page.isEmpty
            ? {'since': since, 'since_id': sinceId}
            : {'since': page.last[timeField], 'since_id': page.last['id']},
      };

      request.response.headers.contentType = ContentType.json;
      request.response.write(json.encode(body));
      await request.response.close();
    });
  }

  static List<Map<String, String>> _after(
    List<Map<String, String>> rows,
    String since,
    String sinceId,
    String Function(Map<String, String>) at,
  ) {
    final sorted = [...rows]
      ..sort((a, b) {
        final byTime = at(a).compareTo(at(b));
        return byTime != 0 ? byTime : a['id']!.compareTo(b['id']!);
      });
    if (since.isEmpty) return sorted;
    return sorted
        .where(
          (r) =>
              at(r).compareTo(since) > 0 ||
              (at(r) == since && r['id']!.compareTo(sinceId) > 0),
        )
        .toList();
  }

  static Future<FakePull> start() async =>
      FakePull(await HttpServer.bind(InternetAddress.loopbackIPv4, 0));

  final HttpServer _server;

  /// What the server holds: id, content, updated_at.
  final List<Map<String, String>> notes = [];

  /// Its tombstones: id, deleted_at.
  final List<Map<String, String>> deletions = [];

  final List<String> requests = [];

  String get url => 'http://127.0.0.1:${_server.port}';

  Future<void> close() => _server.close(force: true);
}

void main() {
  sqflite_setup.setupSqfliteFfi();

  late NotesDao dao;
  late DatabaseHelper helper;

  setUp(() async {
    helper = DatabaseHelper.instance;
    dao = NotesDao(helper);
    final db = await helper.database;
    await db.delete('notes');
    await db.delete('deletions');
    await db.delete('sync_state');
  });

  Future<SyncResult> pullWith(FakePull server) async {
    final container = ProviderContainer();
    addTearDown(container.dispose);
    container
        .read(syncServiceProvider.notifier)
        .initialize(
          serverUrl: server.url,
          apiToken: 'tok',
          notesDao: dao,
          dbHelper: helper,
          recordings: FileRecordingStore(
            directory: Future.value(Directory.systemTemp.createTempSync()),
          ),
        );
    return await container.read(syncServiceProvider.notifier).pull();
  }

  test('a note written on the server arrives on the phone', () async {
    final server = await FakePull.start();
    addTearDown(server.close);
    server.notes.add({
      'id': '01M4JQ111111111111111111AA',
      'content': 'written somewhere else',
      'updated_at': '2026-10-10T09:00:00.000Z',
    });

    final result = await pullWith(server);

    expect(result.success, isTrue, reason: result.error);
    expect(result.notesPulled, 1);
    final note = await dao.getById('01M4JQ111111111111111111AA');
    expect(note?.content, 'written somewhere else');
  });

  // The whole reason this was built. Before it the app only ever pushed, so
  // a note deleted on the server stayed on the phone for good.
  test('a note deleted on the server leaves the phone', () async {
    final server = await FakePull.start();
    addTearDown(server.close);
    const id = '01M4JQ222222222222222222BB';
    server.notes.add({
      'id': id,
      'content': 'deleted later',
      'updated_at': '2026-10-10T09:00:00.000Z',
    });

    await pullWith(server);
    expect(await dao.getById(id), isNotNull);

    // Somebody deletes it on the server.
    server.notes.clear();
    server.deletions.add({'id': id, 'deleted_at': '2026-10-10T10:00:00.000Z'});

    final result = await pullWith(server);

    expect(result.success, isTrue, reason: result.error);
    expect(result.deletionsApplied, 1);
    expect(
      await dao.getById(id),
      isNull,
      reason: 'the note is still on the phone after a server-side delete',
    );
  });

  // A tombstone that came from the server must not be recorded here, or it
  // would be pushed straight back to the server that reported it, on every
  // sync, for ever (ADR-0033 found the same echo on the other sync path).
  test('an applied tombstone is not queued to push back', () async {
    final server = await FakePull.start();
    addTearDown(server.close);
    const id = '01M4JQ333333333333333333CC';
    server.notes.add({
      'id': id,
      'content': 'deleted later',
      'updated_at': '2026-10-10T09:00:00.000Z',
    });
    await pullWith(server);

    server.notes.clear();
    server.deletions.add({'id': id, 'deleted_at': '2026-10-10T10:00:00.000Z'});
    await pullWith(server);

    expect(
      await dao.pendingDeletions(10),
      isEmpty,
      reason: 'the phone will push the deletion back to where it came from',
    );
  });

  test('a pull that has caught up is quiet and does not loop', () async {
    final server = await FakePull.start();
    addTearDown(server.close);
    server.notes.add({
      'id': '01M4JQ444444444444444444DD',
      'content': 'one note',
      'updated_at': '2026-10-10T09:00:00.000Z',
    });

    await pullWith(server);
    final after = server.requests.length;

    final second = await pullWith(server);
    expect(second.success, isTrue, reason: second.error);
    expect(second.notesPulled, 0);
    // Two requests: one page of notes, one of deletions. A walk that did
    // not notice the cursor had stopped moving would keep asking.
    expect(server.requests.length - after, 2);
  });

  // Local is the authority for what this device writes. The server's
  // ordering says nothing about an edit made here and not yet pushed, so
  // without a check an older server copy lands on top of a newer local one
  // — and is then pushed back over it (ADR-0032).
  test('a pull does not overwrite a newer local edit', () async {
    final server = await FakePull.start();
    addTearDown(server.close);
    const id = '01M4JQ555555555555555555EE';

    await dao.insert(
      Note(
        id: id,
        projectId: 'evomem',
        content: 'edited here, not pushed yet',
        sourceType: 'manual',
        createdAt: DateTime.utc(2026, 10, 10, 9),
        updatedAt: DateTime.utc(2026, 10, 10, 11),
      ),
    );
    server.notes.add({
      'id': id,
      'content': 'what the server still has',
      'updated_at': '2026-10-10T09:00:00.000Z',
    });

    await pullWith(server);

    final note = await dao.getById(id);
    expect(note?.content, 'edited here, not pushed yet');
  });

  test('an unreadable row drops out instead of wedging the pull', () async {
    final server = await FakePull.start();
    addTearDown(server.close);
    server.notes.addAll([
      {
        'id': '',
        'content': 'no identifier',
        'updated_at': '2026-10-10T09:00:00.000Z',
      },
      {
        'id': '01M4JQ666666666666666666FF',
        'content': 'fine',
        'updated_at': '2026-10-10T09:00:01.000Z',
      },
    ]);

    final result = await pullWith(server);

    expect(result.success, isTrue, reason: result.error);
    expect(await dao.getById('01M4JQ666666666666666666FF'), isNotNull);
  });

  test('nothing is pulled without a server configured', () async {
    final container = ProviderContainer();
    addTearDown(container.dispose);
    container
        .read(syncServiceProvider.notifier)
        .initialize(
          serverUrl: '',
          apiToken: '',
          notesDao: dao,
          dbHelper: helper,
          recordings: FileRecordingStore(
            directory: Future.value(Directory.systemTemp.createTempSync()),
          ),
        );

    final result = await container.read(syncServiceProvider.notifier).pull();
    expect(result.success, isFalse);
  });

  // A pulled note must not be pushed straight back as a new note: /ingest
  // creates one and hands back a fresh identifier, so an echo would be a
  // duplicate on the server rather than a no-op.
  //
  // What stops it is that absorb records the server's identifier and the
  // moment it had — so the push sees a note it has already sent and that has
  // not been edited since. That is load-bearing and easy to break, which is
  // why it is pinned here rather than left to the two comments that mention
  // it.
  test('a pulled note is not pushed back as a new one', () async {
    final server = await FakePull.start();
    addTearDown(server.close);
    const id = '01M4JQ777777777777777777GG';
    server.notes.add({
      'id': id,
      'content': 'written on the server',
      'updated_at': '2026-10-10T09:00:00.000Z',
    });

    await pullWith(server);

    final note = await dao.getById(id);
    expect(note, isNotNull);
    expect(
      note!.isPushed,
      isTrue,
      reason: 'the push would treat this as a note the server has never seen',
    );
    expect(
      note.isEditedSincePush,
      isFalse,
      reason: 'the push would send it up again and make a duplicate',
    );
  });
}
