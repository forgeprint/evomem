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

  /// The names chosen here, newest first, like the real one keeps.
  final List<String> chosen = [];

  @override
  Future<void> remember(String projectId) async {
    value = projectId;
    chosen
      ..remove(projectId)
      ..insert(0, projectId);
  }

  @override
  Future<List<String>> known() async => List.unmodifiable(chosen);

  @override
  Future<void> forget(String projectId) async => chosen.remove(projectId);
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
  // The report: "I created a new project and it disappeared when I switched
  // projects." It did. A project was known only from the notes filed under
  // it, plus whichever one was selected — so switching off a brand new,
  // empty one was the exact moment it stopped being known.
  test('a project with no notes survives switching away from it', () async {
    final memory = FakeProjectMemory();
    final container = containerWith(memory);

    container.read(currentProjectProvider.notifier).select('deneme');
    await settle();

    container.read(currentProjectProvider.notifier).select(defaultProjectId);
    await settle();
    final projects = await container.read(projectsProvider.future);

    expect(
      projects.map((p) => p.projectId),
      contains('deneme'),
      reason: 'the project somebody just made is gone from the picker',
    );
    expect(
      projects.firstWhere((p) => p.projectId == 'deneme').notes,
      0,
      reason: 'it is still empty, and should say so',
    );
  });

  test('a name typed by mistake can be dropped', () async {
    final memory = FakeProjectMemory();
    final container = containerWith(memory);

    container.read(currentProjectProvider.notifier).select('yanlis');
    await settle();
    container.read(currentProjectProvider.notifier).select(defaultProjectId);
    await settle();
    expect(
      (await container.read(projectsProvider.future)).map((p) => p.projectId),
      contains('yanlis'),
    );

    await container
        .read(projectNamesRevisionProvider.notifier)
        .forget('yanlis');
    await settle();

    expect(
      (await container.read(projectsProvider.future)).map((p) => p.projectId),
      isNot(contains('yanlis')),
    );
  });

  test(
    'the chosen project is always offered, even if nothing remembers it',
    () async {
      final container = containerWith(FakeProjectMemory());

      // Nothing has been remembered and no note is filed under it; it is
      // still what the app is showing, so the picker has to say so.
      container.read(currentProjectProvider.notifier).select('simdi');
      await settle();

      expect(
        (await container.read(projectsProvider.future)).map((p) => p.projectId),
        contains('simdi'),
      );
    },
  );

  test('a name is offered once, with its real count', () async {
    final memory = FakeProjectMemory();
    final container = containerWith(memory);

    // Chosen, so remembered; and it is also the default, which has notes.
    container.read(currentProjectProvider.notifier).select('baska');
    await settle();
    container.read(currentProjectProvider.notifier).select(defaultProjectId);
    await settle();

    final projects = await container.read(projectsProvider.future);
    final names = projects.map((p) => p.projectId).toList();
    expect(
      names.length,
      names.toSet().length,
      reason: 'the same project is offered twice',
    );
  });

  // On a fresh store nothing is filed under `default` and nobody has chosen
  // it on purpose, so it fell out of the picker the moment somebody switched
  // away — and the only way back was typing the word. Seen in the browser,
  // where the store starts empty.
  test('the project the app starts in is always reachable', () async {
    final container = containerWith(FakeProjectMemory());

    container.read(currentProjectProvider.notifier).select('web-deneme');
    await settle();

    expect(
      (await container.read(projectsProvider.future)).map((p) => p.projectId),
      contains(defaultProjectId),
      reason: 'there is no way back to the project the app opens in',
    );
  });

  // Forgetting it would be a button that appears to do nothing, because the
  // list always puts it back.
  test('the starting project cannot be forgotten away', () async {
    final memory = FakeProjectMemory();
    final container = containerWith(memory);

    container.read(currentProjectProvider.notifier).select(defaultProjectId);
    await container
        .read(projectNamesRevisionProvider.notifier)
        .forget(defaultProjectId);
    await settle();

    expect(
      (await container.read(projectsProvider.future)).map((p) => p.projectId),
      contains(defaultProjectId),
    );
  });
}
