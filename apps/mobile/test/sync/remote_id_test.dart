import 'dart:convert';
import 'dart:io';

import 'package:evomem_mobile/src/rules/note.dart';
import 'package:evomem_mobile/src/storage/database.dart';
import 'package:evomem_mobile/src/storage/notes_dao.dart';
import 'package:evomem_mobile/src/sync/sync_service.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:path/path.dart' as path;

import '../sqflite_test_setup.dart' as sqflite_setup;

/// A stand-in for `POST /ingest`, answering the way the Go side does.
///
/// A real server rather than a mocked client: `SyncService` builds its own
/// `http.Client`, and the thing under test is what it does with a reply.
class FakeIngest {
  new(this._server, this.replies) {
    _server.listen((request) async {
      paths.add(request.uri.path);
      queries.add(request.uri.query);
      contentTypes.add(request.headers.contentType?.mimeType ?? '');
      authorizations.add(request.headers.value('authorization') ?? '');

      if (request.method == 'PUT') {
        putBodies.add(await utf8.decoder.bind(request).join());
        bodies.add('');
        request.response.statusCode = putStatus;
        await request.response.close();
        return;
      }

      if (request.uri.path == '/ingest/audio') {
        // The body is the recording itself, so it is read as bytes.
        final chunks = <int>[];
        await request.forEach(chunks.addAll);
        audioBodies.add(chunks);
        bodies.add('');
        request.response.statusCode = audioStatus;
        await request.response.close();
        return;
      }

      bodies.add(await utf8.decoder.bind(request).join());

      final reply = replies.isEmpty
          ? _created('01M4D3H3HNMFM69N4MHNAYBZ1X')
          : replies.removeAt(0);
      request.response.statusCode = reply.status;
      request.response.write(reply.body);
      await request.response.close();
    });
  }

  static Future<FakeIngest> start({List<Reply> replies = const []}) async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    return FakeIngest(server, [...replies]);
  }

  final HttpServer _server;
  final List<Reply> replies;
  final List<String> paths = [];
  final List<String> queries = [];
  final List<String> bodies = [];
  final List<String> contentTypes = [];
  final List<String> authorizations = [];
  final List<List<int>> audioBodies = [];
  final List<String> putBodies = [];

  /// What `/ingest/audio` answers.
  int audioStatus = 204;

  /// What `PUT /notes/{id}` answers.
  int putStatus = 200;

  String get url => 'http://127.0.0.1:${_server.port}';
  int get requests => paths.length;

  Future<void> close() => _server.close(force: true);

  static Reply _created(String id) =>
      Reply(201, json.encode({'status': 'ok', 'id': id, 'project': 'evomem'}));
}

class Reply {
  const new(this.status, this.body);
  final int status;
  final String body;
}

void main() {
  sqflite_setup.setupSqfliteFfi();

  late NotesDao dao;
  late DatabaseHelper helper;

  setUp(() async {
    helper = DatabaseHelper.instance;
    dao = NotesDao(helper);
    // One file is shared by the whole process, so each test starts from an
    // empty table rather than whatever the last one left.
    final db = await helper.database;
    await db.delete('notes');
    await db.delete('sync_state');
  });

  Future<Note> storeNote(String id, String content) async {
    final note = Note(
      id: id,
      projectId: 'evomem',
      content: content,
      sourceType: 'manual',
      createdAt: DateTime.utc(2026, 10, 8),
      updatedAt: DateTime.utc(2026, 10, 8),
    );
    await dao.insert(note);
    return note;
  }

  Future<SyncResult> pushWith(FakeIngest server) async {
    final container = ProviderContainer();
    addTearDown(container.dispose);
    container
        .read(syncServiceProvider.notifier)
        .initialize(
          serverUrl: server.url,
          apiToken: 'tok',
          notesDao: dao,
          dbHelper: helper,
        );
    return await container.read(syncServiceProvider.notifier).push();
  }

  test('a pushed note keeps the id the server gave it', () async {
    await storeNote('local-1', 'buy milk');
    final server = await FakeIngest.start();
    addTearDown(server.close);

    final result = await pushWith(server);
    expect(result.success, isTrue);
    expect(result.notesPushed, 1);
    expect(server.paths, ['/ingest']);

    // The point of the whole change: the phone now knows what the server
    // called the note, which is what a recording is uploaded against.
    final stored = await dao.getById('local-1');
    expect(stored!.remoteId, '01M4D3H3HNMFM69N4MHNAYBZ1X');
    expect(stored.isPushed, isTrue);
  });

  test('recording the id does not move updated_at', () async {
    final note = await storeNote('local-1', 'buy milk');
    final server = await FakeIngest.start();
    addTearDown(server.close);
    await pushWith(server);

    // Bumping it would push the note back over the cursor it just crossed,
    // and the next run would post it again.
    final stored = await dao.getById('local-1');
    expect(stored!.updatedAt, note.updatedAt);
  });

  test('a note the server already has is never posted again', () async {
    await storeNote('local-1', 'buy milk');
    final first = await FakeIngest.start();
    addTearDown(first.close);
    await pushWith(first);

    // The cursor would hold it back on its own; this clears the cursor so
    // the note is read again, which is what happens after an edit.
    final db = await helper.database;
    await db.delete('sync_state');

    final second = await FakeIngest.start();
    addTearDown(second.close);
    final result = await pushWith(second);

    expect(result.success, isTrue);
    // /ingest always creates, so a second post would be a second note.
    expect(second.paths, isNot(contains('/ingest')));
  });

  test('an edit reaches the mirror as a PUT', () async {
    await storeNote('local-1', 'buy milk');
    final first = await FakeIngest.start();
    addTearDown(first.close);
    await pushWith(first);

    // Edited since. The cursor brings it back on (updated_at, id), which is
    // what makes an edit reach the mirror without a column to remember it.
    await dao.update(
      (await dao.getById('local-1'))!
          .withContent('buy oat milk', DateTime.utc(2026, 10, 9)),
    );
    final db = await helper.database;
    await db.delete('sync_state');

    final second = await FakeIngest.start();
    addTearDown(second.close);
    final result = await pushWith(second);

    expect(result.success, isTrue);
    expect(second.paths, ['/notes/01M4D3H3HNMFM69N4MHNAYBZ1X']);
    expect(
      json.decode(second.putBodies.single),
      containsPair('content', 'buy oat milk'),
    );
    expect(result.notesPushed, 1);
  });

  test('a 404 on an edit does not recreate the note', () async {
    await storeNote('local-1', 'buy milk');
    final first = await FakeIngest.start();
    addTearDown(first.close);
    await pushWith(first);

    await dao.update(
      (await dao.getById('local-1'))!
          .withContent('buy oat milk', DateTime.utc(2026, 10, 9)),
    );
    final db = await helper.database;
    await db.delete('sync_state');

    final second = await FakeIngest.start();
    addTearDown(second.close);
    second.putStatus = 404;

    final result = await pushWith(second);
    // Somebody deleted it on the mirror. Posting it again would undo that
    // delete on the next sync, which is the worst outcome available.
    expect(result.success, isTrue);
    expect(second.paths, isNot(contains('/ingest')));
    // The local copy and its id are left alone.
    expect(
      (await dao.getById('local-1'))!.remoteId,
      '01M4D3H3HNMFM69N4MHNAYBZ1X',
    );
    expect((await dao.getById('local-1'))!.content, 'buy oat milk');
  });

  test('an edit that failed for another reason fails the batch', () async {
    await storeNote('local-1', 'buy milk');
    final first = await FakeIngest.start();
    addTearDown(first.close);
    await pushWith(first);

    await dao.update(
      (await dao.getById('local-1'))!
          .withContent('buy oat milk', DateTime.utc(2026, 10, 9)),
    );
    final db = await helper.database;
    await db.delete('sync_state');

    final second = await FakeIngest.start();
    addTearDown(second.close);
    second.putStatus = 500;

    final result = await pushWith(second);
    expect(result.success, isFalse);
  });

  test('a batch that failed half way is safe to retry', () async {
    await storeNote('local-1', 'first');
    await storeNote('local-2', 'second');

    // The first is accepted, the second refused.
    final failing = await FakeIngest.start(
      replies: [
        FakeIngest._created('01M4D3H3HNMFM69N4MHNAYBZ1X'),
        const Reply(500, 'nope'),
      ],
    );
    addTearDown(failing.close);

    final first = await pushWith(failing);
    expect(first.success, isFalse);
    expect(failing.requests, 2);

    // Retried whole. Only the note without an id is posted, so the one that
    // got through is not duplicated.
    final retry = await FakeIngest.start(
      replies: [FakeIngest._created('01M4D3H3HNMFM69N4MHNAYBZ1Y')],
    );
    addTearDown(retry.close);

    final second = await pushWith(retry);
    expect(second.success, isTrue);
    expect(retry.requests, 1);
    expect(json.decode(retry.bodies.first), containsPair('content', 'second'));

    expect(
      (await dao.getById('local-1'))!.remoteId,
      '01M4D3H3HNMFM69N4MHNAYBZ1X',
    );
    expect(
      (await dao.getById('local-2'))!.remoteId,
      '01M4D3H3HNMFM69N4MHNAYBZ1Y',
    );
  });

  test('a 201 with no usable id is a failure', () async {
    await storeNote('local-1', 'buy milk');
    for (final body in ['{}', '{"id":""}', 'not json', '[]']) {
      final server = await FakeIngest.start(replies: [Reply(201, body)]);
      addTearDown(server.close);

      final result = await pushWith(server);
      // Carrying on would advance the cursor past a note nothing can ever
      // attach a recording to, and the next run would post it again.
      expect(result.success, isFalse, reason: 'body $body was accepted');
      expect((await dao.getById('local-1'))!.isPushed, isFalse);

      final db = await helper.database;
      await db.delete('sync_state');
    }
  });

  test('a recording is uploaded against the id the note was given', () async {
    final recording = File(
      path.join(Directory.systemTemp.createTempSync('evomem-up').path, 'a.m4a'),
    )..writeAsStringSync('aac-pretend');
    addTearDown(() => recording.parent.deleteSync(recursive: true));

    await dao.insert(
      Note(
        id: 'local-1',
        projectId: 'evomem',
        content: 'Voice note, 0:03',
        sourceType: 'audio',
        createdAt: DateTime.utc(2026, 10, 8),
        updatedAt: DateTime.utc(2026, 10, 8),
        metadata: {
          'awaiting_transcription': true,
          'local_audio_path': recording.path,
        },
      ),
    );

    final server = await FakeIngest.start();
    addTearDown(server.close);
    final result = await pushWith(server);

    expect(result.success, isTrue);
    // The note first, then the recording against the id that came back.
    expect(server.paths, ['/ingest', '/ingest/audio']);
    expect(server.queries.last, 'note=01M4D3H3HNMFM69N4MHNAYBZ1X');
    expect(server.contentTypes.last, 'audio/m4a');
    expect(utf8.decode(server.audioBodies.single), 'aac-pretend');
  });

  test('a note with no recording uploads nothing', () async {
    await storeNote('local-1', 'typed, not spoken');
    final server = await FakeIngest.start();
    addTearDown(server.close);

    await pushWith(server);
    expect(server.paths, ['/ingest']);
    expect(server.audioBodies, isEmpty);
  });

  test('a recording whose file is gone uploads nothing', () async {
    await dao.insert(
      Note(
        id: 'local-1',
        projectId: 'evomem',
        content: 'Voice note, 0:03',
        sourceType: 'audio',
        createdAt: DateTime.utc(2026, 10, 8),
        updatedAt: DateTime.utc(2026, 10, 8),
        metadata: {
          'awaiting_transcription': true,
          'local_audio_path': '/nowhere/gone.m4a',
        },
      ),
    );

    final server = await FakeIngest.start();
    addTearDown(server.close);
    final result = await pushWith(server);

    // The note still goes; only the recording is missing.
    expect(result.success, isTrue);
    expect(server.paths, ['/ingest']);
  });

  test('an upload that failed does not fail the push', () async {
    final recording = File(
      path.join(Directory.systemTemp.createTempSync('evomem-up').path, 'a.m4a'),
    )..writeAsStringSync('aac-pretend');
    addTearDown(() => recording.parent.deleteSync(recursive: true));

    await dao.insert(
      Note(
        id: 'local-1',
        projectId: 'evomem',
        content: 'Voice note, 0:03',
        sourceType: 'audio',
        createdAt: DateTime.utc(2026, 10, 8),
        updatedAt: DateTime.utc(2026, 10, 8),
        metadata: {
          'awaiting_transcription': true,
          'local_audio_path': recording.path,
        },
      ),
    );

    final server = await FakeIngest.start();
    addTearDown(server.close);
    server.audioStatus = 500;

    final result = await pushWith(server);
    // The note is stored and pushed; the recording is left for another run.
    expect(result.success, isTrue);
    expect(result.notesPushed, 1);
    expect((await dao.getById('local-1'))!.isPushed, isTrue);
  });
}
