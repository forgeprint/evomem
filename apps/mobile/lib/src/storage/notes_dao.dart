import 'dart:async';
import 'dart:convert';

import 'package:evomem_mobile/src/rules/note.dart';
import 'package:evomem_mobile/src/storage/database.dart';
import 'package:sqflite/sqflite.dart';

/// Data Access Object for notes table.
class NotesDao {
  /// Reads and writes notes through the helper's connection.
  new(this._dbHelper);

  final DatabaseHelper _dbHelper;

  Future<Database> get _db async => await _dbHelper.database;

  /// Converts a database row to a Note.
  Note _noteFromMap(Map<String, dynamic> map) {
    final metadata = _decodeMetadata(map['metadata'] as String?);
    return Note(
      id: map['id'] as String,
      projectId: map['project_id'] as String,
      content: map['content'] as String,
      sourceType: map['source_type'] as String,
      createdAt: DateTime.parse(map['created_at'] as String),
      updatedAt: DateTime.parse(map['updated_at'] as String),
      metadata: metadata,
      remoteId: (map['remote_id'] as String?) ?? '',
      remoteUpdatedAt: _parseOptional(map['remote_updated_at'] as String?),
    );
  }

  static DateTime? _parseOptional(String? value) {
    if (value == null || value.isEmpty) return null;
    return DateTime.tryParse(value);
  }

  Map<String, dynamic> _decodeMetadata(String? metadataJson) {
    if (metadataJson == null || metadataJson.isEmpty || metadataJson == '{}') {
      return {};
    }
    try {
      return json.decode(metadataJson) as Map<String, dynamic>;
    } on FormatException {
      // Metadata is an open extension point, so a row written by another
      // version may not decode. An unreadable one is dropped, not fatal.
      return {};
    }
  }

  String _encodeMetadata(Map<String, dynamic> metadata) {
    if (metadata.isEmpty) return '{}';
    return json.encode(metadata);
  }

  /// Inserts a new note.
  Future<void> insert(Note note) async {
    final db = await _db;
    await db.insert('notes', {
      'id': note.id,
      'project_id': note.projectId,
      'content': note.content,
      'source_type': note.sourceType,
      'created_at': note.createdAt.toIso8601String(),
      'updated_at': note.updatedAt.toIso8601String(),
      'metadata': _encodeMetadata(note.metadata),
      'remote_id': note.remoteId,
      'remote_updated_at': note.remoteUpdatedAt?.toIso8601String() ?? '',
    }, conflictAlgorithm: ConflictAlgorithm.fail);
  }

  /// Updates an existing note.
  Future<void> update(Note note) async {
    final db = await _db;
    await db.update(
      'notes',
      {
        'content': note.content,
        'updated_at': note.updatedAt.toIso8601String(),
        'metadata': _encodeMetadata(note.metadata),
      },
      where: 'id = ?',
      whereArgs: [note.id],
    );
  }

  /// Records what the server called a note it accepted.
  ///
  /// Written on its own rather than through [update], which moves
  /// `updated_at` forward: this changes nothing about the note, and bumping
  /// the timestamp would push it back over the sync cursor it just crossed.
  Future<void> setRemoteId(
    String localId,
    String remoteId,
    DateTime remoteUpdatedAt,
  ) async {
    final db = await _db;
    await db.update(
      'notes',
      {
        'remote_id': remoteId,
        'remote_updated_at': remoteUpdatedAt.toIso8601String(),
      },
      where: 'id = ?',
      whereArgs: [localId],
    );
  }

  /// Records that the mirror now holds what this note says.
  ///
  /// Written on its own for the same reason as [setRemoteId]: going through
  /// [update] would move `updated_at` forward and push the note back over the
  /// cursor it just crossed.
  Future<void> setRemoteUpdatedAt(String localId, DateTime at) async {
    final db = await _db;
    await db.update(
      'notes',
      {'remote_updated_at': at.toIso8601String()},
      where: 'id = ?',
      whereArgs: [localId],
    );
  }

  /// Deletes a note by id.
  /// Deletes a note by id, and remembers to tell the mirror.
  ///
  /// A note the server has accepted leaves a tombstone, in the same
  /// transaction as the delete so the two cannot come apart. One it has never
  /// seen leaves none: there would be nothing to ask the server to remove,
  /// and it could only answer 404 (ADR-0020).
  Future<void> delete(String id) async {
    final db = await _db;
    final note = await getById(id);
    await db.transaction((txn) async {
      await txn.delete('notes', where: 'id = ?', whereArgs: [id]);
      if (note != null && note.isPushed) {
        await txn.insert('deletions', {
          'id': note.id,
          'project_id': note.projectId,
          'deleted_at': DateTime.now().toIso8601String(),
          'remote_id': note.remoteId,
        }, conflictAlgorithm: ConflictAlgorithm.replace);
      }
    });
  }

  /// The deletions waiting to be told to the mirror, oldest first.
  Future<List<Deletion>> pendingDeletions(int limit) async {
    final db = await _db;
    final maps = await db.query(
      'deletions',
      orderBy: 'deleted_at ASC, id ASC',
      limit: limit,
    );
    return maps
        .map(
          (map) => Deletion(
            localId: map['id']! as String,
            remoteId: (map['remote_id'] as String?) ?? '',
            projectId: map['project_id']! as String,
          ),
        )
        .toList();
  }

  /// Forgets a deletion the mirror has taken.
  ///
  /// `deletions` is a queue, not a log: keeping a row after the server has
  /// acted on it would mean every sync re-sending every deletion this phone
  /// has ever made (ADR-0020).
  Future<void> forgetDeletion(String localId) async {
    final db = await _db;
    await db.delete('deletions', where: 'id = ?', whereArgs: [localId]);
  }

  /// Gets a note by id.
  Future<Note?> getById(String id) async {
    final db = await _db;
    final maps = await db.query(
      'notes',
      where: 'id = ?',
      whereArgs: [id],
      limit: 1,
    );
    if (maps.isEmpty) return null;
    return _noteFromMap(maps.first);
  }

  /// Lists notes for a project, newest first.
  Future<List<Note>> listByProject({
    required String projectId,
    String? sourceType,
    int? limit,
    int? offset,
  }) async {
    final db = await _db;
    final where = StringBuffer('project_id = ?');
    final whereArgs = <Object>[projectId];

    if (sourceType != null && sourceType.isNotEmpty) {
      where.write(' AND source_type = ?');
      whereArgs.add(sourceType);
    }

    final maps = await db.query(
      'notes',
      where: where.toString(),
      whereArgs: whereArgs,
      orderBy: 'created_at DESC',
      limit: limit,
      offset: offset,
    );

    return maps.map(_noteFromMap).toList();
  }

  /// Counts notes for a project.
  Future<int> countByProject({
    required String projectId,
    String? sourceType,
  }) async {
    final db = await _db;
    final where = StringBuffer('project_id = ?');
    final whereArgs = <Object>[projectId];

    if (sourceType != null && sourceType.isNotEmpty) {
      where.write(' AND source_type = ?');
      whereArgs.add(sourceType);
    }

    final result = await db.rawQuery(
      'SELECT COUNT(*) as count FROM notes WHERE $where',
      whereArgs,
    );
    return Sqflite.firstIntValue(result) ?? 0;
  }

  /// Searches notes by content (simple LIKE search).
  Future<List<Note>> search({
    required String query,
    required String projectId,
    String? sourceType,
    int? limit,
  }) async {
    final db = await _db;
    final where = StringBuffer('project_id = ? AND content LIKE ?');
    final whereArgs = <Object>[projectId, '%$query%'];

    if (sourceType != null && sourceType.isNotEmpty) {
      where.write(' AND source_type = ?');
      whereArgs.add(sourceType);
    }

    final maps = await db.query(
      'notes',
      where: where.toString(),
      whereArgs: whereArgs,
      orderBy: 'updated_at DESC',
      limit: limit,
    );

    return maps.map(_noteFromMap).toList();
  }

  /// Replaces all notes for a project (used for initial load/sync).
  Future<void> replaceAllForProject(String projectId, List<Note> notes) async {
    final db = await _db;
    await db.transaction((txn) async {
      await txn.delete(
        'notes',
        where: 'project_id = ?',
        whereArgs: [projectId],
      );
      for (final note in notes) {
        if (note.projectId != projectId) continue;
        await txn.insert('notes', {
          'id': note.id,
          'project_id': note.projectId,
          'content': note.content,
          'source_type': note.sourceType,
          'created_at': note.createdAt.toIso8601String(),
          'updated_at': note.updatedAt.toIso8601String(),
          'metadata': _encodeMetadata(note.metadata),
        }, conflictAlgorithm: ConflictAlgorithm.replace);
      }
    });
  }

  /// Gets all notes across all projects (for sync).
  Future<List<Note>> getAllNotes({int? limit, int? offset}) async {
    final db = await _db;
    final maps = await db.query(
      'notes',
      orderBy: 'updated_at ASC, id ASC',
      limit: limit,
      offset: offset,
    );
    return maps.map(_noteFromMap).toList();
  }

  /// Gets notes updated after a cursor (for delta sync).
  Future<List<Note>> getNotesAfterCursor(
    String cursorUpdatedAt,
    String cursorId,
    int limit,
  ) async {
    final db = await _db;
    final maps = await db.query(
      'notes',
      where: '(updated_at > ?) OR (updated_at = ? AND id > ?)',
      whereArgs: [cursorUpdatedAt, cursorUpdatedAt, cursorId],
      orderBy: 'updated_at ASC, id ASC',
      limit: limit,
    );
    return maps.map(_noteFromMap).toList();
  }
}

/// One note this phone deleted that the mirror has not been told about.
class Deletion {
  /// Creates a pending deletion.
  const new({
    required this.localId,
    required this.remoteId,
    required this.projectId,
  });

  /// This phone's identifier for the note that went.
  final String localId;

  /// What the server called it, which is what a DELETE is addressed with.
  final String remoteId;

  /// The project it belonged to.
  final String projectId;
}
