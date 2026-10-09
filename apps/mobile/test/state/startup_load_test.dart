import 'package:evomem_mobile/src/state/notes_notifier.dart';
import 'package:evomem_mobile/src/storage/database.dart';
import 'package:evomem_mobile/src/storage/notes_dao.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import '../sqflite_test_setup.dart' as sqflite_setup;

void main() {
  sqflite_setup.setupSqfliteFfi();

  setUp(() async {
    final db = await DatabaseHelper.instance.database;
    await db.delete('notes');
  });

  // What a person sees after closing the app and opening it again. The
  // widget tests never caught this because each one adds its notes inside
  // the session it then asserts on.
  test('notes written in one session are there in the next', () async {
    final first = ProviderContainer();
    first
        .read(notesProvider.notifier)
        .add(rawContent: 'buy milk', projectId: 'default');
    // Let the optimistic write reach the database.
    await Future<void>.delayed(const Duration(milliseconds: 50));
    expect(first.read(notesProvider).single.content, 'buy milk');
    first.dispose();

    // A second container is a second launch: nothing is carried over in
    // memory, so whatever shows up came from the store.
    final second = ProviderContainer();
    addTearDown(second.dispose);
    second.read(notesProvider);
    await Future<void>.delayed(const Duration(milliseconds: 50));

    expect(second.read(notesProvider).map((n) => n.content).toList(), [
      'buy milk',
    ], reason: 'the list is empty on launch; the store is never read');
  });

  test('the store really has the note', () async {
    final container = ProviderContainer();
    addTearDown(container.dispose);
    container
        .read(notesProvider.notifier)
        .add(rawContent: 'buy milk', projectId: 'default');
    await Future<void>.delayed(const Duration(milliseconds: 50));

    // Separating the two failures: written but not read back, or never
    // written at all.
    final dao = NotesDao(DatabaseHelper.instance);
    final stored = await dao.listByProject(projectId: 'default');
    expect(stored.single.content, 'buy milk');
  });
}
