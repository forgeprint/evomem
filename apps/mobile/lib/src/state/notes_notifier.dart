import 'dart:async';

import 'package:evomem_mobile/src/rules/note.dart';
import 'package:evomem_mobile/src/rules/note_rules.dart';
import 'package:evomem_mobile/src/state/project_memory.dart';
import 'package:evomem_mobile/src/storage/database.dart';
import 'package:evomem_mobile/src/storage/notes_dao.dart';
import 'package:evomem_mobile/src/storage/notes_store.dart';
import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// The notes list for a project, and the only thing allowed to change it.
final notesProvider = NotifierProvider<NotesNotifier, List<Note>>(
  NotesNotifier.new,
);

/// What a note written in this app is filed under.
///
/// The phone is a source in its own right, the way Jira and Telegram are: a
/// note that came off a phone says so, and the server can be asked for just
/// those. The browser is not a source — it is where the memory is worked on —
/// so a note typed there is filed as the hand-written note it is.
///
/// Decided here rather than in `lib/src/rules`, which may not import Flutter
/// and so cannot ask which platform this is.
final currentSourceProvider = Provider<String>(
  (ref) => kIsWeb ? defaultSourceType : 'mobile',
);

/// What a project is called when nobody has chosen one.
const defaultProjectId = 'default';

/// Which project the app is looking at.
///
/// Everything project-scoped reads this: the note list, the clusters screen,
/// a note being written and a recording being filed. Changing it rebuilds
/// [notesProvider], which is what makes the switch take effect.
///
/// The choice is remembered, because a project somebody picked and lost on
/// reload is a project they have to pick again every time.
final currentProjectProvider = NotifierProvider<CurrentProject, String>(
  CurrentProject.new,
);

/// Holds the chosen project and remembers it.
class CurrentProject extends Notifier<String> {
  @override
  String build() {
    unawaited(_restore());
    return defaultProjectId;
  }

  Future<void> _restore() async {
    final remembered = await ref.read(rememberedProjectProvider.future);
    // Dropped when the read lands after somebody has already chosen: a
    // remembered value must not undo a deliberate switch.
    if (!ref.mounted || state != defaultProjectId) return;
    if (remembered.isNotEmpty) state = remembered;
  }

  /// Looks at [projectId] from now on, and remembers it.
  void select(String projectId) {
    final trimmed = projectId.trim();
    if (trimmed.isEmpty || trimmed == state) return;
    state = trimmed;
    unawaited(ref.read(projectMemoryProvider).remember(trimmed));
  }
}

/// Database helper provider.
final databaseHelperProvider = Provider<DatabaseHelper>(
  (ref) => DatabaseHelper.instance,
);

/// Notes DAO provider.
/// Where notes are kept, as the port rather than the implementation: nothing
/// above this line knows that today's answer is sqflite (ADR-0022).
final notesDaoProvider = Provider<NotesStore>(
  (ref) => NotesDao(ref.watch(databaseHelperProvider)),
);

/// The projects to offer: the ones with notes, plus the names chosen on this
/// device, plus whichever is current.
///
/// A project nobody has written to yet does not exist as far as the store is
/// concerned. That used to be the whole answer, and it meant a project
/// somebody had just created vanished the moment they switched away — which
/// is exactly what somebody does after creating one: go and look at the old
/// project first. Only the current one was kept, so switching off it lost it.
///
/// So a name somebody chose is remembered on the device as well
/// ([ProjectMemory.known]). It is still not a row in any table and it is not
/// sent anywhere: a project is brought into existence by a note being filed
/// under it, and this is the picker remembering what was typed.
final projectsProvider = FutureProvider<List<ProjectCount>>((ref) async {
  final current = ref.watch(currentProjectProvider);
  // Rebuilt when the notes change, so a project that has just received its
  // first note appears without a reload.
  ref
    ..watch(notesProvider)
    // And when a name is remembered or forgotten.
    ..watch(projectNamesRevisionProvider);

  final withNotes = await ref.watch(notesDaoProvider).projects();
  final counted = {for (final p in withNotes) p.projectId: p};

  final names = <String>[
    ...withNotes.map((p) => p.projectId),
    ...await ref.watch(projectMemoryProvider).known(),
    current,
  ];

  final seen = <String>{};
  return [
    for (final name in names)
      if (seen.add(name)) counted[name] ?? ProjectCount(name, 0),
  ];
});

/// Bumped when the remembered names change, so the picker reads them again.
///
/// [ProjectMemory] is a port over a platform store, not something that can be
/// watched. This is the smallest thing that makes a write visible.
final projectNamesRevisionProvider = NotifierProvider<ProjectNames, int>(
  ProjectNames.new,
);

/// Holds the revision and the two ways to change the remembered names.
class ProjectNames extends Notifier<int> {
  @override
  int build() => 0;

  /// Takes [projectId] off the device's list of names.
  ///
  /// Only the name. A project with notes under it is known from the notes
  /// and comes straight back, which is what makes this safe to offer.
  Future<void> forget(String projectId) async {
    await ref.read(projectMemoryProvider).forget(projectId);
    state++;
  }
}

/// Holds the list. Every decision it makes comes from `lib/src/rules`; this
/// class applies them and owns the identifiers, nothing else.
class NotesNotifier extends Notifier<List<Note>> {
  // Neither of these is `late final`: build() runs again on this same
  // instance whenever the chosen project changes, and a `late final` would
  // throw the second time.
  late NotesStore _dao;
  String _projectId = defaultProjectId;

  /// Counts the changes made in memory.
  ///
  /// A read started before a change must not land after it: the startup read
  /// and the optimistic add race, and without this the note somebody typed a
  /// moment after launch is wiped by a query that ran before they typed it.
  int _revision = 0;

  @override
  List<Note> build() {
    _dao = ref.read(notesDaoProvider);
    // Watched, not read: changing the project has to rebuild this list, or
    // the app shows one project's notes under another's name.
    _projectId = ref.watch(currentProjectProvider);

    // The store is read here and nowhere else at startup. Without this the
    // list is empty on every launch while the rows sit in the database, and
    // no widget test catches it: each one adds its notes inside the session
    // it then asserts on.
    //
    // build() cannot wait, so the first frame is the empty list and the
    // notes arrive when the read returns.
    unawaited(loadNotes());
    return const [];
  }

  /// Loads notes from database. Call this after build or when needed.
  Future<void> loadNotes() async {
    final startedAt = _revision;
    try {
      final notes = await _dao.listByProject(
        projectId: _projectId,
        limit: 1000,
      );
      // Dropped when the notifier is gone or something changed while this
      // was in flight: a read that lands late is either addressed to nobody
      // or already out of date.
      if (!ref.mounted || _revision != startedAt) return;
      state = notes;
    } on Exception {
      if (!ref.mounted || _revision != startedAt) return;
      // A read that fails leaves the screen empty rather than stale.
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

    _revision++;
    // Shown straight away, but only when it belongs to the project on
    // screen. Before the project could be switched these were always the
    // same, so the list showed whatever was added; now a note filed
    // elsewhere would appear under a name it does not belong to.
    if (note.projectId == _projectId) {
      state = [...state, note];
    }

    // Persist to database
    unawaited(
      Future.microtask(() async {
        try {
          await _dao.insert(note);
        } on Exception {
          // The note never reached the database, so take it back out of the
          // list the screen is showing — unless nobody is showing it any more.
          if (!ref.mounted) return;
          state = state.where((n) => n.id != note.id).toList();
        }
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

    _revision++;
    // Optimistic update
    state = [
      for (final note in state)
        if (note.id == id) updatedNote else note,
    ];

    // Persist to database
    unawaited(
      Future.microtask(() async {
        try {
          await _dao.update(updatedNote);
        } on Exception {
          // The database still holds the old content; show that instead of
          // the edit that did not land.
          if (!ref.mounted) return;
          await loadNotes();
        }
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

    _revision++;
    // Optimistic update
    state = state.where((note) => note.id != id).toList();

    // Persist to database
    unawaited(
      Future.microtask(() async {
        try {
          await _dao.delete(id);
        } on Exception {
          // The row is still there, so put the note back in the list.
          if (!ref.mounted) return;
          if (deletedNote != null) {
            state = [...state, deletedNote];
          }
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

    _revision++;
    // Optimistic update
    state = [
      for (final n in state)
        if (n.id == id) updatedNote else n,
    ];

    // Persist to database
    unawaited(
      Future.microtask(() async {
        try {
          await _dao.update(updatedNote);
        } on Exception {
          // The pin did not land; show what the database holds.
          if (!ref.mounted) return;
          await loadNotes();
        }
      }),
    );
  }

  /// Replaces the entire list (used when loading from local DB or sync).
  Future<void> replaceAll(List<Note> notes) async {
    _revision++;
    state = notes;
    await _dao.replaceAllForProject(_projectId, notes);
  }

  /// Searches notes by content.
  Future<List<Note>> search(
    String query, {
    String? sourceType,
    int? limit,
  }) async {
    return await _dao.search(
      query: query,
      projectId: _projectId,
      sourceType: sourceType,
      limit: limit,
    );
  }

  String _generateId() {
    // ULID-shaped, not a ULID: good enough to be unique on one device, and
    // not cryptographically random.
    final now = DateTime.now().millisecondsSinceEpoch.toRadixString(36);
    final random = (DateTime.now().microsecondsSinceEpoch % 1000000)
        .toRadixString(36);
    return '$now$random'.padLeft(26, '0');
  }
}
