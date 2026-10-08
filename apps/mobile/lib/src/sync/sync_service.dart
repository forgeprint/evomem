import 'dart:async';
import 'dart:convert';

import 'package:http/http.dart' as http;
import 'package:sqflite/sqflite.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:evomem_mobile/src/rules/note.dart';
import 'package:evomem_mobile/src/storage/notes_dao.dart';
import 'package:evomem_mobile/src/storage/database.dart';

/// Result of a sync operation.
class SyncResult {
  const SyncResult({
    required this.success,
    required this.notesPushed,
    required this.deletionsPushed,
    this.error,
    this.cursorUpdatedAt,
    this.cursorId,
  });

  final bool success;
  final int notesPushed;
  final int deletionsPushed;
  final String? error;
  final String? cursorUpdatedAt;
  final String? cursorId;

  factory SyncResult.success({
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

  factory SyncResult.failure({
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
  const SyncConfig({
    required this.serverUrl,
    required this.apiToken,
    this.batchSize = 200,
    this.timeout = const Duration(seconds: 30),
  });

  final String serverUrl;
  final String apiToken;
  final int batchSize;
  final Duration timeout;
}

/// Service for pushing local changes to the Evomem Go backend.
///
/// Uses the `/ingest` endpoint with Bearer token authentication.
/// Implements delta sync using cursor (updated_at, id) similar to Go backend.
class SyncService {
  SyncService._({
    required this.config,
    required NotesDao notesDao,
    required DatabaseHelper dbHelper,
  }) : _notesDao = notesDao,
       _dbHelper = dbHelper;

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

    int totalNotesPushed = 0;
    int totalDeletionsPushed = 0;
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
    } catch (e) {
      return SyncResult.failure(
        error: 'Sync failed: $e',
        notesPushed: totalNotesPushed,
        deletionsPushed: totalDeletionsPushed,
      );
    }
  }

  /// Pushes notes in batches using cursor-based pagination.
  Future<SyncResult> _pushNotes() async {
    int totalPushed = 0;
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
  const SyncServiceState({this.config, this.isInitialized = false});

  final SyncConfig? config;
  final bool isInitialized;

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
    int batchSize = 200,
    Duration timeout = const Duration(seconds: 30),
    required NotesDao notesDao,
    required DatabaseHelper dbHelper,
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
    return _service!.push();
  }

  /// Get the current sync service instance.
  SyncService? get service => _service;

  bool get isInitialized => state.isInitialized;
  SyncConfig? get config => state.config;
}

/// Provider for the sync service state.
final syncServiceProvider =
    NotifierProvider<SyncServiceNotifier, SyncServiceState>(
      SyncServiceNotifier.new,
    );
