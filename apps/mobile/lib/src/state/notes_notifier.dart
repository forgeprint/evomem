import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:evomem_mobile/src/rules/note.dart';
import 'package:evomem_mobile/src/rules/note_rules.dart';
import 'package:evomem_mobile/src/storage/database.dart';
import 'package:evomem_mobile/src/storage/notes_dao.dart';

/// The notes list for a project, and the only thing allowed to change it.
final notesProvider = NotifierProvider<NotesNotifier, List<Note>>(
  NotesNotifier.new,
);

/// Current project ID filter.
/// Using a simple provider since the app currently supports a single project.
/// For multi-project support, this would become a StateProvider or similar.
final currentProjectProvider = Provider<String>((ref) => 'default');

/// Database helper provider.
final databaseHelperProvider = Provider<DatabaseHelper>(
  (ref) => DatabaseHelper.instance,
);

/// Notes DAO provider.
final notesDaoProvider = Provider<NotesDao>(
  (ref) => NotesDao(ref.watch(databaseHelperProvider)),
);

/// Holds the list. Every decision it makes comes from `lib/src/rules`; this
/// class applies them and owns the identifiers, nothing else.
class NotesNotifier extends Notifier<List<Note>> {
  late final NotesDao _dao;
  late final String _projectId;

  @override
  List<Note> build() {
    _dao = ref.read(notesDaoProvider);
    _projectId = ref.read(currentProjectProvider);

    // Return empty initially; use loadNotes to load from database
    return const [];
  }

  /// Loads notes from database. Call this after build or when needed.
  Future<void> loadNotes() async {
    try {
      final notes = await _dao.listByProject(
        projectId: _projectId,
        limit: 1000,
      );
      state = notes;
    } catch (_) {
      // If load fails, start with empty list
      state = const [];
    }
  }

  /// Adds a note with [rawContent] to the list.
  ///
  /// Returns the reason it was refused, or `null` when it was added. The
  /// caller decides how to show the reason; the notifier does not know about
  /// widgets or about which language the user reads.
  NoteProblem? add({
    required String rawContent,
    required String projectId,
    String sourceType = defaultSourceType,
    Map<String, dynamic> metadata = const {},
  }) {
    final problem = checkNoteContent(rawContent);
    if (problem != null) return problem;

    final now = DateTime.now();
    final note = Note(
      id: _generateId(),
      projectId: projectId,
      content: normalizeNoteContent(rawContent),
      sourceType: sourceType,
      createdAt: now,
      updatedAt: now,
      metadata: metadata,
    );

    // Optimistic update
    state = [...state, note];

    // Persist to database
    Future.microtask(
      () => _dao.insert(note).catchError((_) {
        // Rollback on error
        state = state.where((n) => n.id != note.id).toList();
      }),
    );

    return null;
  }

  /// Updates the note with this [id].
  NoteProblem? update({required String id, required String newContent}) {
    final problem = checkNoteContent(newContent);
    if (problem != null) return problem;

    final index = state.indexWhere((n) => n.id == id);
    if (index == -1) return null; // Not found, silently ignore

    final updatedNote = state[index].withContent(
      normalizeNoteContent(newContent),
      DateTime.now(),
    );

    // Optimistic update
    state = [
      for (final note in state)
        if (note.id == id) updatedNote else note,
    ];

    // Persist to database
    Future.microtask(
      () => _dao.update(updatedNote).catchError((_) {
        // Rollback on error
        loadNotes();
      }),
    );

    return null;
  }

  /// Deletes the note with this [id].
  void delete(String id) {
    Note? deletedNote;
    for (final note in state) {
      if (note.id == id) {
        deletedNote = note;
        break;
      }
    }

    // Optimistic update
    state = state.where((note) => note.id != id).toList();

    // Persist to database
    Future.microtask(
      () => _dao.delete(id).catchError((_) {
        // Rollback on error
        if (deletedNote != null) {
          state = [...state, deletedNote];
        }
      }),
    );
  }

  /// Toggles a note's metadata 'pinned' flag (example of metadata mutation).
  void togglePin(String id) {
    final index = state.indexWhere((n) => n.id == id);
    if (index == -1) return;

    final note = state[index];
    final newMetadata = Map<String, dynamic>.from(note.metadata);
    newMetadata['pinned'] = !(newMetadata['pinned'] as bool? ?? false);

    final updatedNote = Note(
      id: note.id,
      projectId: note.projectId,
      content: note.content,
      sourceType: note.sourceType,
      createdAt: note.createdAt,
      updatedAt: DateTime.now(),
      metadata: newMetadata,
    );

    // Optimistic update
    state = [
      for (final n in state)
        if (n.id == id) updatedNote else n,
    ];

    // Persist to database
    Future.microtask(
      () => _dao.update(updatedNote).catchError((_) {
        // Rollback on error
        loadNotes();
      }),
    );
  }

  /// Replaces the entire list (used when loading from local DB or sync).
  Future<void> replaceAll(List<Note> notes) async {
    state = notes;
    await _dao.replaceAllForProject(_projectId, notes);
  }

  /// Searches notes by content.
  Future<List<Note>> search(
    String query, {
    String? sourceType,
    int? limit,
  }) async {
    return _dao.search(
      query: query,
      projectId: _projectId,
      sourceType: sourceType,
      limit: limit,
    );
  }

  String _generateId() {
    // Simple ULID-like generation (not cryptographically secure, but unique enough for local use)
    final now = DateTime.now().millisecondsSinceEpoch.toRadixString(36);
    final random = (DateTime.now().microsecondsSinceEpoch % 1000000)
        .toRadixString(36);
    return '$now$random'.padLeft(26, '0');
  }
}
