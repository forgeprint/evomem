import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:path/path.dart' as path;
import 'package:sqflite/sqflite.dart';

/// Database version.
///
/// Versions 1 to 3 match the Go backend's, because the tables were the same
/// ones. From 4 on they are the phone's alone, recording facts about this
/// device's sync rather than the store the backend keeps: `notes.remote_id`
/// (what the server called a note it accepted) and `notes.remote_updated_at`
/// (what the note said when it did), and `deletions.remote_id` (which note on
/// the mirror a local deletion is asking to remove).
const int _databaseVersion = 6;

/// Database file name.
const String _databaseName = 'evomem.db';

/// Database helper class managing the SQLite connection.
class DatabaseHelper {
  new _();

  /// The one helper for the process. The database is a single file and
  /// sqflite keeps one connection to it.
  static final DatabaseHelper instance = DatabaseHelper._();

  Database? _database;

  /// Overrides the file the store is kept in.
  ///
  /// Only a test sets this. `flutter test` runs files in parallel against one
  /// temporary directory, so two of them opening the same name share a file —
  /// and a test that needs a store at a particular schema version cannot have
  /// another one creating it first.
  @visibleForTesting
  static String? databasePathOverride;

  /// The open database, opening and migrating it on first use.
  Future<Database> get database async {
    if (_database != null) return _database!;
    _database = await _initDatabase();
    return _database!;
  }

  Future<Database> _initDatabase() async {
    final documentsDirectory = await getDatabasesPath();
    final dbPath =
        databasePathOverride ?? path.join(documentsDirectory, _databaseName);

    return await openDatabase(
      dbPath,
      version: _databaseVersion,
      onCreate: _onCreate,
      onUpgrade: _onUpgrade,
    );
  }

  Future<void> _onCreate(Database db, int version) async {
    await db.execute('''
      CREATE TABLE notes (
        rowid INTEGER PRIMARY KEY AUTOINCREMENT,
        id TEXT NOT NULL UNIQUE,
        project_id TEXT NOT NULL,
        content TEXT NOT NULL,
        source_type TEXT NOT NULL,
        created_at TEXT NOT NULL,
        updated_at TEXT NOT NULL,
        metadata TEXT NOT NULL DEFAULT '{}',
        remote_id TEXT NOT NULL DEFAULT '',
        remote_updated_at TEXT NOT NULL DEFAULT ''
      )
    ''');

    await db.execute('''
      CREATE INDEX idx_notes_project ON notes(project_id)
    ''');

    await db.execute('''
      CREATE INDEX idx_notes_project_created ON notes(project_id, created_at DESC)
    ''');

    await db.execute('''
      CREATE INDEX idx_notes_source ON notes(source_type)
    ''');

    await db.execute('''
      CREATE INDEX idx_notes_updated ON notes(updated_at, id)
    ''');

    // Schema v2: deletions table for sync
    await db.execute('''
      CREATE TABLE deletions (
        id TEXT PRIMARY KEY,
        project_id TEXT NOT NULL,
        deleted_at TEXT NOT NULL,
        remote_id TEXT NOT NULL DEFAULT ''
      )
    ''');

    await db.execute('''
      CREATE INDEX idx_deletions_at ON deletions(deleted_at, id)
    ''');

    // Schema v2: sync_state for watermark
    await db.execute('''
      CREATE TABLE sync_state (
        key TEXT PRIMARY KEY,
        value TEXT NOT NULL
      )
    ''');

    // Schema v3: proposals table
    await db.execute('''
      CREATE TABLE proposals (
        id TEXT PRIMARY KEY,
        project_id TEXT NOT NULL,
        content TEXT NOT NULL,
        source_type TEXT NOT NULL,
        metadata TEXT NOT NULL DEFAULT '{}',
        reason TEXT NOT NULL DEFAULT '',
        proposed_by TEXT NOT NULL DEFAULT '',
        proposed_at TEXT NOT NULL,
        status TEXT NOT NULL DEFAULT 'pending',
        decided_at TEXT,
        note_id TEXT NOT NULL DEFAULT ''
      )
    ''');

    await db.execute('''
      CREATE INDEX idx_proposals_pending ON proposals(status, proposed_at DESC)
    ''');

    // Meta table for schema version
    await db.execute('''
      CREATE TABLE meta (
        key TEXT PRIMARY KEY,
        value TEXT NOT NULL
      )
    ''');

    await db.insert('meta', {
      'key': 'schema_version',
      'value': _databaseVersion.toString(),
    });
  }

  Future<void> _onUpgrade(Database db, int oldVersion, int newVersion) async {
    if (oldVersion < 2) {
      await db.execute('''
        CREATE TABLE deletions (
          id TEXT PRIMARY KEY,
          project_id TEXT NOT NULL,
          deleted_at TEXT NOT NULL
        )
      ''');
      await db.execute(
        'CREATE INDEX idx_deletions_at ON deletions(deleted_at, id)',
      );

      await db.execute('''
        CREATE TABLE sync_state (
          key TEXT PRIMARY KEY,
          value TEXT NOT NULL
        )
      ''');
    }

    if (oldVersion < 3) {
      await db.execute('''
        CREATE TABLE proposals (
          id TEXT PRIMARY KEY,
          project_id TEXT NOT NULL,
          content TEXT NOT NULL,
          source_type TEXT NOT NULL,
          metadata TEXT NOT NULL DEFAULT '{}',
          reason TEXT NOT NULL DEFAULT '',
          proposed_by TEXT NOT NULL DEFAULT '',
          proposed_at TEXT NOT NULL,
          status TEXT NOT NULL DEFAULT 'pending',
          decided_at TEXT,
          note_id TEXT NOT NULL DEFAULT ''
        )
      ''');
      await db.execute(
        'CREATE INDEX idx_proposals_pending '
        'ON proposals(status, proposed_at DESC)',
      );
    }

    if (oldVersion < 4) {
      // What the server called a note it accepted. Empty means it has not
      // been accepted yet, which is what every existing row gets: the ids
      // the server minted for them were discarded and cannot be recovered,
      // so those notes stay unattachable. See ADR-0018.
      await db.execute(
        "ALTER TABLE notes ADD COLUMN remote_id TEXT NOT NULL DEFAULT ''",
      );
    }

    if (oldVersion < 5) {
      // The note's own updated_at at the moment the server accepted it.
      // Without it a retried batch cannot tell a note that was edited since
      // from one that was not, and writes both to the mirror (ADR-0019).
      //
      // Empty for every existing row, which reads as "pushed, and we do not
      // know what it said then": those notes get one redundant PUT on their
      // next edit and are exact from then on.
      await db.execute(
        'ALTER TABLE notes '
        "ADD COLUMN remote_updated_at TEXT NOT NULL DEFAULT ''",
      );
    }

    if (oldVersion < 6) {
      // Which note on the mirror a local deletion is asking to remove. The
      // table's `id` is this phone's own, which the server has never heard
      // of, so the remote id gets a column rather than overloading that one
      // (ADR-0020).
      await db.execute(
        'ALTER TABLE deletions '
        "ADD COLUMN remote_id TEXT NOT NULL DEFAULT ''",
      );
    }

    await db.update(
      'meta',
      {'value': newVersion.toString()},
      where: 'key = ?',
      whereArgs: ['schema_version'],
    );
  }

  /// Forgets the open connection without closing it, so the next read of
  /// [database] opens the file again. Only a test needs this, after pointing
  /// [databasePathOverride] somewhere else.
  @visibleForTesting
  void forgetConnection() {
    _database = null;
  }

  /// Closes the connection. The next read of [database] reopens it.
  Future<void> close() async {
    final db = await database;
    await db.close();
    _database = null;
  }
}
