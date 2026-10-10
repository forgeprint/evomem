import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

/// Remembers which project the app was last looking at.
///
/// Its own port so a test can supply one without a keychain: the real
/// implementation reaches a platform channel, and a widget test that has to
/// stand one up is a test nobody writes.
abstract interface class ProjectMemory {
  /// The project last chosen, or empty when none has been.
  Future<String> remembered();

  /// Remembers [projectId] as the one being looked at, and as one that
  /// exists.
  Future<void> remember(String projectId);

  /// The projects somebody has chosen on this device, in the order they
  /// were last chosen.
  ///
  /// A project is otherwise known only from the notes filed under it, so one
  /// that has just been created has nothing to be known by: it vanished from
  /// the picker the moment anybody switched away — which is exactly when
  /// somebody creates one and then goes to look at something else first.
  ///
  /// Kept on the device, not sent anywhere. A project is still not a row in
  /// any table; this is the picker remembering what you typed.
  Future<List<String>> known();

  /// Forgets [projectId], for a name that was typed and never used.
  ///
  /// It only takes the name off this list. A project with notes under it is
  /// known from the notes and comes straight back.
  Future<void> forget(String projectId);
}

/// Keeps the choice in the store the server settings already use.
class StoredProjectMemory implements ProjectMemory {
  /// Creates a memory over [storage].
  const new([this.storage = const FlutterSecureStorage()]);

  /// Where the value is kept.
  final FlutterSecureStorage storage;

  /// The key the value is kept under.
  static const key = 'evomem_current_project';

  /// Where the chosen names are kept, newline separated.
  ///
  /// One string rather than a key each: the store is a flat map with no way
  /// to list what is in it, so a set has to be one value.
  static const knownKey = 'evomem_known_projects';

  /// How many names are kept.
  ///
  /// A bound, because nothing else removes them: somebody who types a name a
  /// day for a year should not find a picker with 365 rows in it. The oldest
  /// goes first, and a project with notes under it is unaffected — it is
  /// known from the notes.
  static const maxKnown = 20;

  // Both of these swallow anything, not just Exception. The ways this store
  // is unavailable are not all exceptions: a locked keychain and a browser
  // with site data blocked throw, and with no Flutter binding — any plain
  // Dart test that happens to build the provider — the platform channel
  // throws an Error. Remembering a project is a convenience; none of those
  // is worth taking the app down for, and every one of them means the same
  // thing: there is no remembered choice.

  @override
  Future<String> remembered() async {
    try {
      return (await storage.read(key: key) ?? '').trim();
    } on Object {
      return '';
    }
  }

  @override
  Future<void> remember(String projectId) async {
    try {
      await storage.write(key: key, value: projectId);
    } on Object {
      // The project is already selected; this only fails to outlive a
      // reload.
    }
    await _write([projectId, ...await known()]);
  }

  @override
  Future<List<String>> known() async {
    try {
      final raw = await storage.read(key: knownKey) ?? '';
      return raw
          .split('\n')
          .map((name) => name.trim())
          .where((name) => name.isNotEmpty)
          .toList();
    } on Object {
      return const [];
    }
  }

  @override
  Future<void> forget(String projectId) async {
    final name = projectId.trim();
    await _write((await known()).where((k) => k != name).toList());
  }

  /// Writes the list back, newest first, without repeats and bounded.
  Future<void> _write(List<String> names) async {
    final unique = <String>[];
    for (final name in names) {
      final trimmed = name.trim();
      if (trimmed.isEmpty || unique.contains(trimmed)) continue;
      unique.add(trimmed);
      if (unique.length == maxKnown) break;
    }
    try {
      await storage.write(key: knownKey, value: unique.join('\n'));
    } on Object {
      // Same as above: a convenience that did not outlive a reload.
    }
  }
}

/// Where the chosen project is remembered.
final projectMemoryProvider = Provider<ProjectMemory>(
  (ref) => const StoredProjectMemory(),
);

/// What was remembered, or empty when nothing was.
final rememberedProjectProvider = FutureProvider<String>(
  (ref) => ref.watch(projectMemoryProvider).remembered(),
);
