import 'package:evomem_mobile/src/state/notes_notifier.dart';
import 'package:evomem_mobile/src/state/project_memory.dart';
import 'package:evomem_mobile/src/storage/database.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import '../sqflite_test_setup.dart' as sqflite_setup;

/// A memory that lives in this test and reaches no keychain.
class FakeProjectMemory implements ProjectMemory {
  new([this.value = '']);

  String value;

  @override
  Future<String> remembered() async => value;

  @override
  Future<void> remember(String projectId) async => value = projectId;
}

ProviderContainer containerWith(FakeProjectMemory memory) {
  final container = ProviderContainer(
    overrides: [projectMemoryProvider.overrideWithValue(memory)],
  );
  addTearDown(container.dispose);
  // Providers dispose themselves when nothing is listening, and a notifier
  // that was thrown away and rebuilt looks exactly like one that never
  // reloaded. In the app a widget is watching; here this stands in for it.
  container.listen(notesProvider, (_, _) {});
  return container;
}

/// Lets the eagerly-started reads and the optimistic writes land.
Future<void> settle() => Future<void>.delayed(const Duration(milliseconds: 60));

void main() {
  sqflite_setup.setupSqfliteFfi();

  setUp(() async {
    final db = await DatabaseHelper.instance.database;
    await db.delete('notes');
  });

  test('the list follows the project that was chosen', () async {
    final container = containerWith(FakeProjectMemory());
    container.read(notesProvider.notifier)
      ..add(rawContent: 'work thing', projectId: 'work')
      ..add(rawContent: 'home thing', projectId: 'home');
    await settle();

    // Both were written while 'default' was selected, so neither belongs to
    // the list on screen.
    expect(container.read(notesProvider), isEmpty);

    container.read(currentProjectProvider.notifier).select('work');
    await settle();
    expect(container.read(notesProvider).map((n) => n.content).toList(), [
      'work thing',
    ], reason: 'switching the project did not reload the list');

    container.read(currentProjectProvider.notifier).select('home');
    await settle();
    expect(container.read(notesProvider).map((n) => n.content).toList(), [
      'home thing',
    ]);
  });

  // The notifier's build() runs again on the same instance when the project
  // changes. A `late final` field would throw the second time, and the
  // failure would look like a storage bug rather than a lifecycle one.
  test('switching twice does not throw', () async {
    final container = containerWith(FakeProjectMemory());
    final project = container.read(currentProjectProvider.notifier)
      ..select('one');
    await settle();
    project.select('two');
    await settle();

    expect(container.read(currentProjectProvider), 'two');
  });

  test('the choice is remembered and comes back next launch', () async {
    final memory = FakeProjectMemory();
    final first = containerWith(memory);
    first.read(currentProjectProvider.notifier).select('work');
    await settle();
    expect(memory.value, 'work');

    // A second container is a second launch.
    final second = containerWith(memory);
    expect(second.read(currentProjectProvider), defaultProjectId);
    await settle();
    expect(second.read(currentProjectProvider), 'work');
  });

  // A remembered value arrives asynchronously. If somebody switches before
  // it lands, the restore must not undo their choice.
  test('a remembered value does not undo a deliberate switch', () async {
    final container = containerWith(FakeProjectMemory('work'));
    container.read(currentProjectProvider.notifier).select('home');
    await settle();

    expect(container.read(currentProjectProvider), 'home');
  });

  test(
    'the picker lists the projects the store holds, and the current one',
    () async {
      final container = containerWith(FakeProjectMemory());
      container.read(notesProvider.notifier)
        ..add(rawContent: 'a', projectId: 'work')
        ..add(rawContent: 'b', projectId: 'work')
        ..add(rawContent: 'c', projectId: 'home');
      await settle();

      final listed = await container.read(projectsProvider.future);
      final byProject = {for (final p in listed) p.projectId: p.notes};

      expect(byProject['work'], 2);
      expect(byProject['home'], 1);
      // Nothing is filed under it, so the store does not know it exists — but
      // it is what the app is showing, so the picker has to say so.
      expect(
        byProject[defaultProjectId],
        0,
        reason: 'the selected project vanished from its own picker',
      );
    },
  );
}
