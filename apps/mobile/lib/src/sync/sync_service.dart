import 'dart:async';
import 'dart:convert';

import 'package:evomem_mobile/src/audio/recording_controller.dart';
import 'package:evomem_mobile/src/audio/recording_store.dart';
import 'package:evomem_mobile/src/clusters/cluster_providers.dart';
import 'package:evomem_mobile/src/http/utf8_body.dart';
import 'package:evomem_mobile/src/rules/note.dart';
import 'package:evomem_mobile/src/state/notes_notifier.dart';
import 'package:evomem_mobile/src/storage/database.dart';
import 'package:evomem_mobile/src/storage/notes_store.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:http/http.dart' as http;
import 'package:sqflite/sqflite.dart';

/// Result of a sync operation.
class SyncResult {
  /// Use [SyncResult.success] or [SyncResult.failure] instead; this takes
  /// every field and checks none of them against each other.
  const new({
    required this.success,
    required this.notesPushed,
    required this.deletionsPushed,
    this.notesPulled = 0,
    this.deletionsApplied = 0,
    this.error,
    this.cursorUpdatedAt,
    this.cursorId,
  });

  /// A push that went through, with how much of it moved and where the
  /// cursor now stands.
  factory success({
    required int notesPushed,
    required int deletionsPushed,
    int notesPulled = 0,
    int deletionsApplied = 0,
    String? cursorUpdatedAt,
    String? cursorId,
  }) {
    return SyncResult(
      success: true,
      notesPushed: notesPushed,
      deletionsPushed: deletionsPushed,
      notesPulled: notesPulled,
      deletionsApplied: deletionsApplied,
      cursorUpdatedAt: cursorUpdatedAt,
      cursorId: cursorId,
    );
  }

  /// A push that did not go through. The counts say how much had already
  /// moved before it stopped.
  factory failure({
    required String error,
    int notesPushed = 0,
    int deletionsPushed = 0,
    int notesPulled = 0,
    int deletionsApplied = 0,
  }) {
    return SyncResult(
      success: false,
      error: error,
      notesPushed: notesPushed,
      deletionsPushed: deletionsPushed,
      notesPulled: notesPulled,
      deletionsApplied: deletionsApplied,
    );
  }

  /// Whether the push finished.
  final bool success;

  /// How many notes reached the server.
  final int notesPushed;

  /// How many deletions reached the server.
  final int deletionsPushed;

  /// How many of the server's notes this device did not have.
  ///
  /// Its own field rather than reusing [notesPushed]. The two directions are
  /// different facts and a screen that shows one as the other is lying
  /// quietly — which is what the `// reuse field` in _pushDeletions already
  /// did.
  final int notesPulled;

  /// How many of the server's deletions were applied here.
  final int deletionsApplied;

  /// Why it stopped, or null when it did not.
  final String? error;

  /// The `updated_at` half of the cursor to resume from.
  final String? cursorUpdatedAt;

  /// The `id` half of the cursor, which breaks ties within one timestamp.
  final String? cursorId;

  @override
  String toString() {
    if (success) {
      return 'SyncResult.success(notesPushed: $notesPushed, deletionsPushed: $deletionsPushed, cursor: $cursorUpdatedAt/$cursorId)';
    } else {
      return 'SyncResult.failure(error: $error)';
    }
  }
}

/// Configuration for the sync service.
class SyncConfig {
  /// Where to push and with what token.
  const new({
    required this.serverUrl,
    required this.apiToken,
    this.batchSize = 200,
    this.timeout = const Duration(seconds: 30),
  });

  /// Base URL of the Go backend, without a trailing path.
  final String serverUrl;

  /// Bearer token for `/ingest`.
  final String apiToken;

  /// How many rows go in one request.
  final int batchSize;

  /// How long one request may take before it is given up on.
  final Duration timeout;
}

/// Service for pushing local changes to the Evomem Go backend.
///
/// Uses the `/ingest` endpoint with Bearer token authentication.
/// Implements delta sync using cursor (updated_at, id) similar to Go backend.
class SyncService {
  new _({
    required this.config,
    required this._notesDao,
    required this._dbHelper,
    required this._recordings,
  });

  /// What this service was configured with.
  final SyncConfig config;

  final NotesStore _notesDao;
  final DatabaseHelper _dbHelper;

  /// Where a note's recording lives on this device. A port, because a phone
  /// has a file and a browser has a blob (ADR-0029).
  final RecordingStore _recordings;

  static const String _cursorKey = 'sync_cursor_notes';

  /// Where the pull has got to in the server's notes.
  ///
  /// Its own row, not [_cursorKey]. Both are positions in an
  /// `(updated_at, id)` ordering, but over two different stores: one counts
  /// local notes that have reached the server, the other the server's notes
  /// that have reached here. Sharing a row meant — on the other sync path,
  /// where it actually happened — a pull setting the push watermark to a
  /// position in somebody else's ordering, and every local note behind it
  /// silently never being sent again (ADR-0032).
  static const String _pullCursorKey = 'sync_cursor_pulled';

  /// Where the pull has got to in the server's tombstones.
  static const String _pullDeletionsCursorKey = 'sync_cursor_pulled_deletions';

  /// Pushes local changes to the remote server.
  Future<SyncResult> push() async {
    if (config.serverUrl.isEmpty || config.apiToken.isEmpty) {
      return SyncResult.failure(
        error: 'Server URL or API token not configured',
      );
    }

    var totalNotesPushed = 0;
    var totalDeletionsPushed = 0;
    String? finalCursorUpdatedAt;
    String? finalCursorId;

    try {
      // Push notes
      final notesResult = await _pushNotes();
      totalNotesPushed = notesResult.notesPushed;
      finalCursorUpdatedAt = notesResult.cursorUpdatedAt;
      finalCursorId = notesResult.cursorId;

      // Push deletions
      final deletionsResult = await _pushDeletions();
      totalDeletionsPushed = deletionsResult.notesPushed; // reuse field

      if (notesResult.success && deletionsResult.success) {
        return SyncResult.success(
          notesPushed: totalNotesPushed,
          deletionsPushed: totalDeletionsPushed,
          cursorUpdatedAt: finalCursorUpdatedAt,
          cursorId: finalCursorId,
        );
      } else {
        return SyncResult.failure(
          error: '${notesResult.error ?? ''} ${deletionsResult.error ?? ''}'
              .trim(),
          notesPushed: totalNotesPushed,
          deletionsPushed: totalDeletionsPushed,
        );
      }
    } on Exception catch (e) {
      return SyncResult.failure(
        error: 'Sync failed: $e',
        notesPushed: totalNotesPushed,
        deletionsPushed: totalDeletionsPushed,
      );
    }
  }

  /// Pushes notes in batches using cursor-based pagination.
  Future<SyncResult> _pushNotes() async {
    var totalPushed = 0;
    String? cursorUpdatedAt;
    String? cursorId;

    // Load cursor from sync_state
    final cursor = await _getCursor(_cursorKey);
    if (cursor != null) {
      final parts = cursor.split('|');
      if (parts.length == 2) {
        cursorUpdatedAt = parts[0];
        cursorId = parts[1];
      }
    }

    while (true) {
      final notes = await _notesDao.getNotesAfterCursor(
        cursorUpdatedAt ?? '',
        cursorId ?? '',
        config.batchSize,
      );

      if (notes.isEmpty) {
        break;
      }

      final pushed = await _pushNotesBatch(notes);
      if (pushed == null) {
        return SyncResult.failure(
          error: 'Failed to push notes batch',
          notesPushed: totalPushed,
        );
      }

      // What was actually posted, not how many were read: a batch may be
      // mostly notes the server already has.
      totalPushed += pushed;

      // Update cursor to last note in batch
      final lastNote = notes.last;
      cursorUpdatedAt = lastNote.updatedAt.toIso8601String();
      cursorId = lastNote.id;

      // Save cursor after each batch
      await _saveCursor(_cursorKey, '$cursorUpdatedAt|$cursorId');

      if (notes.length < config.batchSize) {
        break;
      }
    }

    return SyncResult.success(
      notesPushed: totalPushed,
      deletionsPushed: 0,
      cursorUpdatedAt: cursorUpdatedAt,
      cursorId: cursorId,
    );
  }

  /// Pushes a single batch of notes to the /ingest endpoint.
  ///
  /// `/ingest` always creates, so a note posted twice becomes two notes. The
  /// identifier it returns is therefore kept, and a note that has one is not
  /// posted again — which is what makes a batch that failed half way safe to
  /// retry. See ADR-0018.
  /// Returns how many notes were posted, or null when the batch failed.
  Future<int?> _pushNotesBatch(List<Note> notes) async {
    var posted = 0;
    final client = http.Client();
    try {
      final uri = Uri.parse('${config.serverUrl}/ingest');
      final headers = {
        'Content-Type': 'application/json',
        'Authorization': 'Bearer ${config.apiToken}',
      };

      // Sequentially, so the server's created_at order matches the phone's.
      for (final note in notes) {
        if (note.isPushed) {
          // Already accepted, under the id in remote_id. Posting it again
          // would make a second note, so an edit goes as a PUT (ADR-0019).
          //
          // Only when it has actually changed: a batch that failed half way
          // is retried whole, and writing an unchanged note to the mirror
          // would move its updated_at there — which is the cursor another
          // device pulls on.
          if (!note.isEditedSincePush) continue;
          final changed = await _putNote(client, note);
          if (changed == null) return null;
          if (changed) {
            await _notesDao.setRemoteUpdatedAt(note.id, note.updatedAt);
            posted++;
          }
          continue;
        }

        final body = json.encode({
          'project': note.projectId,
          'content': note.content,
          'source': note.sourceType,
          'metadata': note.metadata,
        });

        final request = http.Request('POST', uri)
          ..headers.addAll(headers)
          ..body = body;

        final streamedResponse = await client
            .send(request)
            .timeout(config.timeout);
        final response = await http.Response.fromStream(streamedResponse);

        if (response.statusCode != 201) {
          return null;
        }

        final remoteId = _remoteIdOf(utf8Body(response));
        if (remoteId == null) {
          // Accepted, but this cannot tell which note it became. Treated as
          // a failure: carrying on would advance the cursor past a note
          // nothing can ever attach a recording to, and the next run would
          // post it again.
          return null;
        }
        await _notesDao.setRemoteId(note.id, remoteId, note.updatedAt);
        posted++;

        // The recording, now that there is an identifier to attach it to.
        // A failure here does not fail the batch: the note is stored and
        // pushed, and the upload is retried by the next run because the
        // metadata still points at a file nobody has sent.
        await _uploadRecording(client, note.withRemoteId(remoteId));
      }
      return posted;
    } finally {
      client.close();
    }
  }

  /// Sends what a pushed note now says to `PUT /notes/{id}`.
  ///
  /// Returns whether the mirror was changed, or null when the batch should
  /// stop. A 404 is neither: the mirror no longer has this note, and posting
  /// it again would bring back something somebody deleted there, so the batch
  /// carries on and leaves the local copy alone (ADR-0019).
  Future<bool?> _putNote(http.Client client, Note note) async {
    final uri = Uri.parse(
      '${config.serverUrl}/notes/${Uri.encodeComponent(note.remoteId)}',
    );
    final request = http.Request('PUT', uri)
      ..headers.addAll({
        'Content-Type': 'application/json',
        'Authorization': 'Bearer ${config.apiToken}',
      })
      ..body = json.encode({
        'content': note.content,
        'metadata': note.metadata,
      });

    final streamed = await client.send(request).timeout(config.timeout);
    final response = await http.Response.fromStream(streamed);

    if (response.statusCode == 200) return true;
    if (response.statusCode == 404) return false;
    return null;
  }

  /// Sends a note's recording to `POST /ingest/audio`, if it has one.
  ///
  /// Two requests rather than one because the identifier comes from the
  /// first; see ADR-0018. Nothing is sent for a note with no recording,
  /// which is almost all of them.
  Future<void> _uploadRecording(http.Client client, Note note) async {
    final handle = note.metadata[metaLocalPath];
    if (handle is! String || handle.isEmpty) return;
    if (note.metadata[metaAwaitingTranscription] != true) return;

    final recording = await _recordings.read(handle);
    if (recording == null) {
      // Removed since it was made, restored from a backup that did not
      // carry it, or recorded in a browser that has since been reloaded
      // (ADR-0029). Marked rather than left: a note waiting forever for
      // audio nobody has is worse than one that says the audio is gone.
      await _markRecordingLost(note);
      return;
    }

    final uri = Uri.parse(
      '${config.serverUrl}/ingest/audio?note=${Uri.encodeQueryComponent(note.remoteId)}',
    );
    final request = http.Request('POST', uri)
      ..headers.addAll({
        // What the bytes actually are, not what this app asked the encoder
        // for: a browser that cannot encode AAC records webm instead, and
        // the server decides how to store it by this header (ADR-0029).
        'Content-Type': recording.mediaType,
        'Authorization': 'Bearer ${config.apiToken}',
      })
      ..bodyBytes = recording.bytes;

    try {
      final response = await client.send(request).timeout(config.timeout);
      // Drained, or the connection is held open until it times out.
      await response.stream.drain<void>();
    } on Exception {
      // Left for the next run. Reporting it would fail a push that did
      // store the note, and the transcription queue on the server already
      // says this note is waiting for audio.
      return;
    }
  }

  /// Records that a note's audio is gone, so nothing waits for it again.
  ///
  /// The waiting flag comes off and the reason goes on. Only local: the
  /// server never received these bytes, so there is nothing there to
  /// correct.
  Future<void> _markRecordingLost(Note note) async {
    final metadata = Map<String, dynamic>.from(note.metadata)
      ..remove(metaAwaitingTranscription)
      ..[metaRecordingLost] = true;
    try {
      await _notesDao.update(
        Note(
          id: note.id,
          projectId: note.projectId,
          content: note.content,
          sourceType: note.sourceType,
          createdAt: note.createdAt,
          updatedAt: note.updatedAt,
          metadata: metadata,
          remoteId: note.remoteId,
          remoteUpdatedAt: note.remoteUpdatedAt,
        ),
      );
    } on Exception {
      // The upload is already not happening; failing the whole push over
      // a note that cannot be annotated would lose the notes that did go.
    }
  }

  /// Reads the id out of what /ingest answered.
  ///
  /// `{"status":"ok","id":"01M4D...","project":"evomem"}`. Null when the
  /// reply is not that shape, which is not something to guess at: the id is
  /// the only way back to the note the server stored.
  static String? _remoteIdOf(String responseBody) {
    try {
      final decoded = json.decode(responseBody);
      if (decoded is! Map<String, dynamic>) return null;
      final id = decoded['id'];
      if (id is! String || id.isEmpty) return null;
      return id;
    } on FormatException {
      return null;
    }
  }

  /// Tells the mirror about the notes this phone deleted.
  ///
  /// A deletion that never arrives is worse than a note that never arrives:
  /// it is content somebody deliberately removed, still readable by every
  /// agent the mirror feeds (ADR-0020).
  Future<SyncResult> _pushDeletions() async {
    final pending = await _notesDao.pendingDeletions(config.batchSize);
    if (pending.isEmpty) {
      return SyncResult.success(notesPushed: 0, deletionsPushed: 0);
    }

    final client = http.Client();
    var done = 0;
    try {
      for (final deletion in pending) {
        final uri = Uri.parse(
          '${config.serverUrl}/notes/${Uri.encodeComponent(deletion.remoteId)}',
        );
        final request = http.Request('DELETE', uri)
          ..headers['Authorization'] = 'Bearer ${config.apiToken}';

        final http.Response response;
        try {
          final streamed = await client.send(request).timeout(config.timeout);
          response = await http.Response.fromStream(streamed);
        } on Exception catch (e) {
          return SyncResult.failure(
            error: 'Failed to push deletions: $e',
            deletionsPushed: done,
          );
        }

        // 404 counts as done: the mirror does not have the note, which is
        // what this was asking for. Anything else leaves the row for the
        // next run rather than dropping it on the floor.
        if (response.statusCode != 204 && response.statusCode != 404) {
          return SyncResult.failure(
            error: 'Failed to push deletions: ${response.statusCode}',
            deletionsPushed: done,
          );
        }

        await _notesDao.forgetDeletion(deletion.localId);
        done++;
      }
    } finally {
      client.close();
    }

    return SyncResult.success(notesPushed: done, deletionsPushed: done);
  }

  /// Reads from the server what this device does not have.
  ///
  /// Two things, in this order: notes changed since the last pull, then the
  /// deletions. Deletions second, because a note written and then deleted on
  /// the server arrives in both pages and the deletion has to be the later
  /// word (ADR-0034).
  ///
  /// Until this existed the app only ever pushed, so a note deleted on the
  /// server stayed on the phone for good — there was nothing it could read
  /// that said otherwise.
  Future<SyncResult> pull() async {
    if (config.serverUrl.isEmpty || config.apiToken.isEmpty) {
      return SyncResult.failure(
        error: 'Server URL or API token not configured',
      );
    }

    final client = http.Client();
    var notes = 0;
    var deletions = 0;
    try {
      notes = await _pullNotes(client);
      deletions = await _pullDeletions(client);
    } on Object catch (e) {
      // Object, not Exception: a failure coming up through browser interop
      // arrives as an Error, which `on Exception` does not catch (ADR-0029).
      return SyncResult.failure(
        error: 'Pull failed: $e',
        notesPulled: notes,
        deletionsApplied: deletions,
      );
    } finally {
      client.close();
    }

    return SyncResult.success(
      notesPushed: 0,
      deletionsPushed: 0,
      notesPulled: notes,
      deletionsApplied: deletions,
    );
  }

  /// Walks `GET /sync/notes` until the server has nothing newer.
  Future<int> _pullNotes(http.Client client) async {
    var absorbed = 0;
    for (;;) {
      final body = await _getPage(client, '/sync/notes', _pullCursorKey);
      final rows = body['notes'];
      if (rows is! List || rows.isEmpty) return absorbed;

      for (final row in rows) {
        final note = _noteFromServer(row);
        // One unreadable row drops out rather than losing the page — and
        // the cursor still moves past it, so it does not wedge the pull.
        if (note == null) continue;
        if (await _notesDao.absorb(note)) absorbed++;
      }

      if (!await _advance(body, _pullCursorKey)) return absorbed;
      if (rows.length < config.batchSize) return absorbed;
    }
  }

  /// Walks `GET /sync/deletions` and applies each tombstone here.
  Future<int> _pullDeletions(http.Client client) async {
    var applied = 0;
    for (;;) {
      final body = await _getPage(
        client,
        '/sync/deletions',
        _pullDeletionsCursorKey,
      );
      final rows = body['deletions'];
      if (rows is! List || rows.isEmpty) return applied;

      for (final row in rows) {
        if (row is! Map<String, dynamic>) continue;
        final id = row['id'];
        if (id is! String || id.isEmpty) continue;
        // forgetByRemoteId, not delete: a tombstone recorded here would be
        // pushed straight back to the server that reported it.
        await _notesDao.forgetByRemoteId(id);
        applied++;
      }

      if (!await _advance(body, _pullDeletionsCursorKey)) return applied;
      if (rows.length < config.batchSize) return applied;
    }
  }

  /// One page from a pull endpoint, from wherever [cursorKey] left off.
  Future<Map<String, dynamic>> _getPage(
    http.Client client,
    String path,
    String cursorKey,
  ) async {
    final query = <String, String>{'limit': '${config.batchSize}'};
    final cursor = await _getCursor(cursorKey);
    if (cursor != null) {
      final parts = cursor.split('|');
      if (parts.length == 2 && parts[0].isNotEmpty && parts[1].isNotEmpty) {
        query['since'] = parts[0];
        query['since_id'] = parts[1];
      }
    }

    final uri = Uri.parse('${config.serverUrl}$path')
        .replace(queryParameters: query);
    final response = await client
        .get(uri, headers: {'Authorization': 'Bearer ${config.apiToken}'})
        .timeout(config.timeout);
    if (response.statusCode != 200) {
      throw http.ClientException(
        'the server answered ${response.statusCode}',
        uri,
      );
    }

    final decoded = json.decode(utf8Body(response));
    if (decoded is! Map<String, dynamic>) {
      throw http.ClientException(
        'the server answered something unreadable',
        uri,
      );
    }
    return decoded;
  }

  /// Stores the cursor the server handed back. False when it did not move,
  /// which is how a walk knows it has caught up rather than looping.
  Future<bool> _advance(Map<String, dynamic> body, String cursorKey) async {
    final cursor = body['cursor'];
    if (cursor is! Map<String, dynamic>) return false;
    final since = cursor['since'];
    final sinceId = cursor['since_id'];
    if (since is! String || sinceId is! String) return false;
    if (since.isEmpty || sinceId.isEmpty) return false;

    final next = '$since|$sinceId';
    if (await _getCursor(cursorKey) == next) return false;
    await _saveCursor(cursorKey, next);
    return true;
  }

  /// A note in the server's shape: `source_type`, `created_at`, `metadata`.
  static Note? _noteFromServer(Object? raw) {
    if (raw is! Map<String, dynamic>) return null;
    final id = raw['id'];
    final content = raw['content'];
    if (id is! String || id.isEmpty || content is! String) return null;
    final created = DateTime.tryParse(raw['created_at'] as String? ?? '');
    final updated = DateTime.tryParse(raw['updated_at'] as String? ?? '');
    if (created == null || updated == null) return null;
    return Note(
      id: id,
      projectId: raw['project_id'] as String? ?? '',
      content: content,
      sourceType: raw['source_type'] as String? ?? 'manual',
      createdAt: created,
      updatedAt: updated,
      metadata: raw['metadata'] is Map<String, dynamic>
          ? raw['metadata'] as Map<String, dynamic>
          : const {},
    );
  }

  /// Gets cursor from sync_state table.
  Future<String?> _getCursor(String key) async {
    final db = await _dbHelper.database;
    final result = await db.query(
      'sync_state',
      where: 'key = ?',
      whereArgs: [key],
      limit: 1,
    );
    if (result.isEmpty) return null;
    return result.first['value'] as String?;
  }

  /// Saves cursor to sync_state table.
  Future<void> _saveCursor(String key, String value) async {
    final db = await _dbHelper.database;
    await db.insert('sync_state', {
      'key': key,
      'value': value,
    }, conflictAlgorithm: ConflictAlgorithm.replace);
  }
}

/// State for the sync service provider.
class SyncServiceState {
  /// A state with no configuration is what the app starts in.
  const new({this.config, this.isInitialized = false});

  /// The configuration in force, or null before Settings has been filled in.
  final SyncConfig? config;

  /// Whether a service has been built from [config].
  final bool isInitialized;

  /// A copy with the given fields replaced.
  SyncServiceState copyWith({SyncConfig? config, bool? isInitialized}) {
    return SyncServiceState(
      config: config ?? this.config,
      isInitialized: isInitialized ?? this.isInitialized,
    );
  }
}

/// Notifier for the sync service.
class SyncServiceNotifier extends Notifier<SyncServiceState> {
  SyncService? _service;

  @override
  SyncServiceState build() {
    // Configured from what is already stored, not only when somebody
    // presses Save. Until this, `initialize` was called from exactly one
    // place — the settings screen's save handler — so a reload with
    // perfectly good settings left the sync service unconfigured, and the
    // status screen said "set the server URL in Settings" to somebody who
    // just had. The other screens read the stored settings directly, which
    // is why they worked and this did not.
    unawaited(_restore());
    return const SyncServiceState();
  }

  /// Builds the service from the settings already on this device.
  Future<void> _restore() async {
    final settings = await ref.read(serverSettingsProvider.future);
    if (!ref.mounted || state.isInitialized) return;
    if (settings.url.isEmpty || settings.token.isEmpty) return;

    initialize(
      serverUrl: settings.url,
      apiToken: settings.token,
      notesDao: ref.read(notesDaoProvider),
      dbHelper: ref.read(databaseHelperProvider),
      recordings: ref.read(recordingStoreProvider),
    );
  }

  /// Initialize the sync service with configuration.
  void initialize({
    required String serverUrl,
    required String apiToken,
    required NotesStore notesDao,
    required DatabaseHelper dbHelper,
    required RecordingStore recordings,
    int batchSize = 200,
    Duration timeout = const Duration(seconds: 30),
  }) {
    final config = SyncConfig(
      serverUrl: serverUrl,
      apiToken: apiToken,
      batchSize: batchSize,
      timeout: timeout,
    );

    _service = SyncService._(
      config: config,
      notesDao: notesDao,
      dbHelper: dbHelper,
      recordings: recordings,
    );

    state = state.copyWith(config: config, isInitialized: true);
  }

  /// Dispose the sync service (e.g., on clear data).
  void dispose() {
    _service = null;
    state = const SyncServiceState();
  }

  /// Push local changes to the remote server.
  Future<SyncResult> push() async {
    if (_service == null) {
      return SyncResult.failure(
        error: 'Sync service not initialized. Configure in Settings.',
      );
    }
    return await _service!.push();
  }

  /// Read from the server what this device does not have (ADR-0034).
  Future<SyncResult> pull() async {
    if (_service == null) {
      return SyncResult.failure(
        error: 'Sync service not initialized. Configure in Settings.',
      );
    }
    return await _service!.pull();
  }

  /// Push, then read back: one round of sync, which is what a person who
  /// presses the button means.
  ///
  /// Push first, so an edit made here reaches the server before the server's
  /// copy is read back — otherwise the pull would see the older version and
  /// skip it, which is correct but wastes a round.
  Future<SyncResult> sync() async {
    final pushed = await push();
    if (!pushed.success) return pushed;
    final pulled = await pull();
    if (!pulled.success) {
      return SyncResult.failure(
        // The push half did happen, so what it moved is reported even
        // though the round as a whole did not finish.
        error: pulled.error ?? 'the pull did not finish',
        notesPushed: pushed.notesPushed,
        deletionsPushed: pushed.deletionsPushed,
      );
    }
    // A pull writes straight to sqflite, which the list does not watch —
    // so without this the notes are there and nobody can see them. Found by
    // running it: the pull reported ten notes and the screen said "No notes
    // yet".
    if (pulled.notesPulled > 0 || pulled.deletionsApplied > 0) {
      await ref.read(notesProvider.notifier).loadNotes();
    }

    return SyncResult.success(
      notesPushed: pushed.notesPushed,
      deletionsPushed: pushed.deletionsPushed,
      notesPulled: pulled.notesPulled,
      deletionsApplied: pulled.deletionsApplied,
      cursorUpdatedAt: pushed.cursorUpdatedAt,
      cursorId: pushed.cursorId,
    );
  }

  /// Get the current sync service instance.
  SyncService? get service => _service;

  /// Whether a service has been built and can push.
  bool get isInitialized => state.isInitialized;

  /// The configuration in force, or null before Settings has been filled in.
  SyncConfig? get config => state.config;
}

/// Provider for the sync service state.
final syncServiceProvider =
    NotifierProvider<SyncServiceNotifier, SyncServiceState>(
      SyncServiceNotifier.new,
    );
