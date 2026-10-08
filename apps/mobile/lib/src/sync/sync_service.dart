import 'dart:async';
import 'dart:convert';

import 'package:evomem_mobile/src/rules/note.dart';
import 'package:evomem_mobile/src/storage/database.dart';
import 'package:evomem_mobile/src/storage/notes_dao.dart';
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

  final NotesDao _notesDao;
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

      final success = await _pushNotesBatch(notes);
      if (!success) {
        return SyncResult.failure(
          error: 'Failed to push notes batch',
          notesPushed: totalPushed,
        );
      }

      totalPushed += notes.length;

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
  Future<bool> _pushNotesBatch(List<Note> notes) async {
    final client = http.Client();
    try {
      final uri = Uri.parse('${config.serverUrl}/ingest');
      final headers = {
        'Content-Type': 'application/json',
        'Authorization': 'Bearer ${config.apiToken}',
      };

      // Process notes sequentially to maintain order
      for (final note in notes) {
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
          return false;
        }
      }
      return true;
    } finally {
      client.close();
    }
  }

  /// Pushes deletions (tombstones) to the remote.
  Future<SyncResult> _pushDeletions() async {
    // For now, we don't track deletions locally in the mobile app
    // The Go backend handles deletions via the sync worker
    // This would need a local deletions table to track
    return SyncResult.success(notesPushed: 0, deletionsPushed: 0);
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
    required NotesDao notesDao,
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
