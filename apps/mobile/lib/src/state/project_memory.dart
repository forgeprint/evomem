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

  /// Remembers [projectId] as the one being looked at.
  Future<void> remember(String projectId);
}

/// Keeps the choice in the store the server settings already use.
class StoredProjectMemory implements ProjectMemory {
  /// Creates a memory over [storage].
  const new([this.storage = const FlutterSecureStorage()]);

  /// Where the value is kept.
  final FlutterSecureStorage storage;

  /// The key the value is kept under.
  static const key = 'evomem_current_project';

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
