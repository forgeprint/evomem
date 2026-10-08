import 'dart:async';

import 'package:path/path.dart' as path;
import 'package:sqflite/sqflite.dart';

/// Database version — must match Go backend schema version.
const int _databaseVersion = 3;

/// Database file name.
const String _databaseName = 'evomem.db';

/// Database helper class managing the SQLite connection.
class DatabaseHelper {
  DatabaseHelper._();

  static final DatabaseHelper instance = DatabaseHelper._();

  Database? _database;

  Future<Database> get database async {
    if (_database != null) return _database!;
    _database = await _initDatabase();
    return _database!;
  }

  Future<Database> _initDatabase() async {
    final documentsDirectory = await getDatabasesPath();
    final dbPath = path.join(documentsDirectory, _databaseName);

    return openDatabase(
      dbPath,
      version: _databaseVersion,
      onCreate: _onCreate,
      onUpgrade: _onUpgrade,
      singleInstance: true,
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
        metadata TEXT NOT NULL DEFAULT '{}'
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
        deleted_at TEXT NOT NULL
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
        'CREATE INDEX idx_proposals_pending ON proposals(status, proposed_at DESC)',
      );
    }

    await db.update(
      'meta',
      {'value': newVersion.toString()},
      where: 'key = ?',
      whereArgs: ['schema_version'],
    );
  }

  Future<void> close() async {
    final db = await database;
    await db.close();
    _database = null;
  }
}
