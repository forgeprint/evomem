import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:evomem_mobile/src/audio/recording_controller.dart';
import 'package:evomem_mobile/src/audio/voice_recorder.dart';
import 'package:evomem_mobile/src/rules/note.dart';
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
    this.error,
    this.cursorUpdatedAt,
    this.cursorId,
  });

  /// A push that went through, with how much of it moved and where the
  /// cursor now stands.
  factory success({
    required int notesPushed,
    required int deletionsPushed,
    String? cursorUpdatedAt,
    String? cursorId,
  }) {
    return SyncResult(
      success: true,
      notesPushed: notesPushed,
      deletionsPushed: deletionsPushed,
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
  }) {
    return SyncResult(
      success: false,
      error: error,
      notesPushed: notesPushed,
      deletionsPushed: deletionsPushed,
    );
  }

  /// Whether the push finished.
  final bool success;

  /// How many notes reached the server.
  final int notesPushed;

  /// How many deletions reached the server.
  final int deletionsPushed;

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
  });

  /// What this service was configured with.
  final SyncConfig config;

  final NotesStore _notesDao;
  final DatabaseHelper _dbHelper;

  static const String _cursorKey = 'sync_cursor_notes';

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

        final remoteId = _remoteIdOf(response.body);
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
    final path = note.metadata[metaLocalPath];
    if (path is! String || path.isEmpty) return;
    if (note.metadata[metaAwaitingTranscription] != true) return;

    final file = File(path);
    if (!file.existsSync()) {
      // Recorded on this phone and since removed, or restored from a backup
      // that did not carry the file. Nothing to send and nothing to fix.
      return;
    }

    final uri = Uri.parse(
      '${config.serverUrl}/ingest/audio?note=${Uri.encodeQueryComponent(note.remoteId)}',
    );
    final request = http.Request('POST', uri)
      ..headers.addAll({
        'Content-Type': recordingMediaType,
        'Authorization': 'Bearer ${config.apiToken}',
      })
      ..bodyBytes = await file.readAsBytes();

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
    return const SyncServiceState();
  }

  /// Initialize the sync service with configuration.
  void initialize({
    required String serverUrl,
    required String apiToken,
    required NotesStore notesDao,
    required DatabaseHelper dbHelper,
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
