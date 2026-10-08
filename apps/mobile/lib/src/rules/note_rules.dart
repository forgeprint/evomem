/// The longest note content the app stores.
const int maxNoteLength = 10000;

/// Why a typed note was refused.
enum NoteProblem {
  /// Nothing was typed, or only whitespace was.
  blank,

  /// The trimmed content is longer than [maxNoteLength].
  tooLong,
}

/// What is wrong with [raw] as note content, or `null` when it can be stored.
NoteProblem? checkNoteContent(String raw) {
  final trimmed = raw.trim();
  if (trimmed.isEmpty) return NoteProblem.blank;
  if (trimmed.length > maxNoteLength) return NoteProblem.tooLong;
  return null;
}

/// The form of [raw] that is stored: the surrounding whitespace removed.
String normalizeNoteContent(String raw) => raw.trim();

/// Known source types matching the Go backend.
const List<String> knownSourceTypes = [
  'manual',
  'telegram',
  'jira',
  'shortcut',
  'audio',
  'mcp',
];

/// Validates a source type.
bool isValidSourceType(String sourceType) =>
    knownSourceTypes.contains(sourceType);

/// Default source type for user-created notes.
const String defaultSourceType = 'manual';
