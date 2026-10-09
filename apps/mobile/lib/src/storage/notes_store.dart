import 'package:evomem_mobile/src/rules/note.dart';
import 'package:evomem_mobile/src/storage/notes_dao.dart' show Deletion;

/// Everything the app asks of a place to keep notes.
///
/// A port, not a layer for its own sake. `NotesDao` is the one implementation
/// today and it speaks SQL through sqflite; what this buys is that the next
/// one does not have to. A store backed by the Go server, or one that keeps
/// notes somewhere an organising model can reach, implements this and nothing
/// above it changes.
///
/// It is deliberately the set of calls the app already makes and not a
/// complete SQL surface: a method nobody calls is a method the next
/// implementation has to write for nobody.
abstract interface class NotesStore {
  /// Stores a new note.
  Future<void> insert(Note note);

  /// Replaces a note's content, metadata and `updatedAt`.
  Future<void> update(Note note);

  /// Deletes a note, and remembers to tell the mirror when the note had been
  /// accepted by it.
  Future<void> delete(String id);

  /// Reads one note, or null when there is none.
  Future<Note?> getById(String id);

  /// The projects this store holds notes for, with how many each has.
  ///
  /// Asked of the store rather than kept in a setting, because the answer is
  /// a fact about the notes: a project exists exactly as long as a note is
  /// filed under it, and one that was emptied should stop being offered.
  Future<List<ProjectCount>> projects();

  /// A project's notes, newest first.
  Future<List<Note>> listByProject({
    required String projectId,
    String? sourceType,
    int? limit,
    int? offset,
  });

  /// Notes matching [query] within a project.
  Future<List<Note>> search({
    required String query,
    required String projectId,
    String? sourceType,
    int? limit,
  });

  /// Replaces everything a project holds, for a restore.
  Future<void> replaceAllForProject(String projectId, List<Note> notes);

  /// Notes after the sync cursor, oldest first, for a push.
  Future<List<Note>> getNotesAfterCursor(
    String cursorUpdatedAt,
    String cursorId,
    int limit,
  );

  /// Records what the server called a note it accepted, and what the note
  /// said at that moment.
  Future<void> setRemoteId(
    String localId,
    String remoteId,
    DateTime remoteUpdatedAt,
  );

  /// Records that the mirror now holds what this note says.
  Future<void> setRemoteUpdatedAt(String localId, DateTime at);

  /// The deletions waiting to be told to the mirror, oldest first.
  Future<List<Deletion>> pendingDeletions(int limit);

  /// Forgets a deletion the mirror has taken.
  Future<void> forgetDeletion(String localId);
}

/// A project and how many notes are filed under it.
class ProjectCount {
  /// Creates a count.
  const new(this.projectId, this.notes);

  /// What the project is called.
  final String projectId;

  /// How many notes it holds.
  final int notes;
}
