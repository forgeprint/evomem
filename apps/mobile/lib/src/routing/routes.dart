import 'package:go_router/go_router.dart';

/// The list of notes for a project.
///
/// Routes are objects rather than strings so that a location is spelled once.
/// A `context.go('/nots/1')` typo compiles; `NoteDetailRoute('abc').location`
/// cannot be misspelled, and renaming a path is one edit in this file.
class NotesListRoute {
  /// Creates the route to the list.
  const new({this.projectId = 'default'});

  /// The project ID this route shows.
  final String projectId;

  /// The pattern `GoRouter` matches on.
  static const String path = '/';

  /// Where to send `context.go`.
  String get location => path;
}

/// One note, addressed by its id.
class NoteDetailRoute {
  /// Creates the route to the note with this [id].
  const new(this.id);

  /// The pattern `GoRouter` matches on, with its one path parameter.
  static const String path = '/notes/:id';

  /// The note this route names.
  final String id;

  /// Where to send `context.go`.
  String get location => '/notes/$id';
}

/// Settings screen route.
class SettingsRoute {
  /// Creates the route to settings.
  const new();

  /// The pattern `GoRouter` matches on.
  static const String path = '/settings';

  /// Where to send `context.go`.
  String get location => path;
}

/// Sync status screen route.
class SyncStatusRoute {
  /// Creates the route to sync status.
  const new();

  /// The pattern `GoRouter` matches on.
  static const String path = '/sync';

  /// Where to send `context.go`.
  String get location => path;
}

/// Reads a [NoteDetailRoute] out of [state], or `null` when the link is not
/// one this app can serve.
///
/// Every route parameter is input from outside the app: a deep link, a
/// browser address bar, another app. `/notes/abc` arrives here, and the
/// caller has to be able to say so rather than throw.
NoteDetailRoute? parseNoteDetailRoute(GoRouterState state) {
  final raw = state.pathParameters['id'];
  if (raw == null || raw.isEmpty) return null;
  return NoteDetailRoute(raw);
}
