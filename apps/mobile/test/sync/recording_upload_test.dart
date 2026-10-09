import 'dart:io';
import 'dart:typed_data';

import 'package:evomem_mobile/src/audio/recording_controller.dart';
import 'package:evomem_mobile/src/audio/recording_store.dart';
import 'package:evomem_mobile/src/audio/voice_recorder.dart';
import 'package:evomem_mobile/src/rules/note.dart';
import 'package:evomem_mobile/src/storage/database.dart';
import 'package:evomem_mobile/src/storage/notes_dao.dart';
import 'package:evomem_mobile/src/sync/sync_service.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import '../sqflite_test_setup.dart' as sqflite_setup;

/// A store that holds whatever the test put in it, with whatever type the
/// test said. Stands in for a browser, which is the whole reason the media
/// type is not a constant (ADR-0029).
class FakeRecordingStore implements RecordingStore {
  final Map<String, RecordingBytes> held = {};

  @override
  Future<String> newTarget() async => 'target';

  @override
  Future<int?> claim(String handle) async => held[handle]?.bytes.length;

  @override
  Future<RecordingBytes?> read(String handle) async => held[handle];

  @override
  Future<void> remove(String handle) async => held.remove(handle);
}

/// Answers /ingest and /ingest/audio the way the Go side does, and keeps
/// what the audio request carried.
class FakeServer {
  new(this._server) {
    _server.listen((request) async {
      if (request.uri.path == '/ingest/audio') {
        audioContentTypes.add(request.headers.contentType?.toString() ?? '');
        audioBodies.add(await _bytesOf(request));
        request.response.statusCode = 204;
        await request.response.close();
        return;
      }
      await request.drain<void>();
      request.response.statusCode = 201;
      request.response.write('{"status":"ok","id":"$remoteId"}');
      await request.response.close();
    });
  }

  static Future<FakeServer> start() async =>
      FakeServer(await HttpServer.bind(InternetAddress.loopbackIPv4, 0));

  static Future<Uint8List> _bytesOf(HttpRequest request) async {
    final chunks = <int>[];
    await request.forEach(chunks.addAll);
    return Uint8List.fromList(chunks);
  }

  final HttpServer _server;
  final String remoteId = '01M4D3H3HNMFM69N4MHNAYBZ1X';
  final List<String> audioContentTypes = [];
  final List<Uint8List> audioBodies = [];

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
  });

  Future<SyncResult> pushWith(FakeServer server, RecordingStore store) async {
    final container = ProviderContainer();
    addTearDown(container.dispose);
    container
        .read(syncServiceProvider.notifier)
        .initialize(
          serverUrl: server.url,
          apiToken: 'tok',
          notesDao: dao,
          dbHelper: helper,
          recordings: store,
        );
    return await container.read(syncServiceProvider.notifier).push();
  }

  Future<Note> addRecordingNote(String handle) async {
    final now = DateTime.now();
    final note = Note(
      id: 'note-1',
      projectId: 'default',
      content: 'Voice note, 0:03',
      sourceType: 'audio',
      createdAt: now,
      updatedAt: now,
      metadata: {metaAwaitingTranscription: true, metaLocalPath: handle},
    );
    await dao.insert(note);
    return note;
  }

  // A browser records webm because it cannot encode AAC. Sending the
  // constant this app asked the encoder for would have the server store it
  // under the wrong name, and the failure would land on the transcription
  // service rather than here (ADR-0029).
  test('the upload declares what the bytes actually are', () async {
    final server = await FakeServer.start();
    addTearDown(server.close);

    final store = FakeRecordingStore()
      ..held['blob:whatever'] = RecordingBytes(
        Uint8List.fromList([1, 2, 3, 4]),
        'audio/webm;codecs=opus',
      );
    await addRecordingNote('blob:whatever');

    final result = await pushWith(server, store);
    expect(result.success, isTrue);

    expect(server.audioContentTypes.single, contains('audio/webm'));
    expect(server.audioBodies.single, [1, 2, 3, 4]);
  });

  test('a phone recording still goes as m4a', () async {
    final server = await FakeServer.start();
    addTearDown(server.close);

    final store = FakeRecordingStore()
      ..held['/tmp/note.m4a'] = RecordingBytes(
        Uint8List.fromList([9]),
        recordingMediaType,
      );
    await addRecordingNote('/tmp/note.m4a');

    await pushWith(server, store);
    expect(server.audioContentTypes.single, contains(recordingMediaType));
  });

  // The browser was reloaded, or the file was removed. A note left waiting
  // forever for audio nobody has is worse than one that says it is gone.
  test(
    'audio that is gone marks the note instead of waiting forever',
    () async {
      final server = await FakeServer.start();
      addTearDown(server.close);

      await addRecordingNote('blob:gone');
      await pushWith(server, FakeRecordingStore());

      expect(server.audioBodies, isEmpty, reason: 'it uploaded nothing');

      final after = await dao.getById('note-1');
      expect(after, isNotNull);
      expect(
        after!.metadata[metaAwaitingTranscription],
        isNull,
        reason: 'the note is still waiting for audio that cannot arrive',
      );
      expect(after.metadata[metaRecordingLost], isTrue);
      expect(after.content, 'Voice note, 0:03', reason: 'the note changed');
    },
  );
}
