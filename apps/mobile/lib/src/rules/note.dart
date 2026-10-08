/// One note in the memory store.
///
/// Plain Dart on purpose: nothing in `lib/src/rules` may import Flutter, so
/// every rule here is testable without pumping a widget.
/// `test/rules/rules_are_flutter_free_test.dart` is what keeps it that way.
class Note {
  /// Creates a note. [id] is unique within one project and never reused.
  const new({
    required this.id,
    required this.projectId,
    required this.content,
    required this.sourceType,
    required this.createdAt,
    required this.updatedAt,
    this.metadata = const {},
    this.remoteId = '',
  });

  /// Identifies the note in a link such as `/notes/abc123`.
  final String id;

  /// The project this note belongs to.
  final String projectId;

  /// What the note says.
  final String content;

  /// What produced this note (manual, telegram, jira, shortcut, audio, mcp).
  final String sourceType;

  /// When the note was created.
  final DateTime createdAt;

  /// When the note was last updated.
  final DateTime updatedAt;

  /// Extensible metadata for source-specific attributes.
  final Map<String, dynamic> metadata;

  /// What the server called this note when it accepted it, or empty when it
  /// has not been accepted.
  ///
  /// The phone mints [id] and the server mints its own, so one note has two
  /// identifiers and only this says what the other one is. Two things need
  /// it: a recording is uploaded against the server's id, and a note that
  /// already has one is not posted a second time. See ADR-0018.
  final String remoteId;

  /// Whether the server has accepted this note.
  bool get isPushed => remoteId.isNotEmpty;

  /// The same note with [content] and [updatedAt] changed.
  Note withContent(String newContent, DateTime newUpdatedAt) => Note(
    id: id,
    projectId: projectId,
    content: newContent,
    sourceType: sourceType,
    createdAt: createdAt,
    updatedAt: newUpdatedAt,
    metadata: metadata,
    remoteId: remoteId,
  );

  /// The same note with the identifier the server gave it.
  Note withRemoteId(String newRemoteId) => Note(
    id: id,
    projectId: projectId,
    content: content,
    sourceType: sourceType,
    createdAt: createdAt,
    updatedAt: updatedAt,
    metadata: metadata,
    remoteId: newRemoteId,
  );
}
