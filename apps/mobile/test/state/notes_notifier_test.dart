import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:evomem_mobile/src/rules/note.dart';
import 'package:evomem_mobile/src/rules/note_rules.dart';
import 'package:evomem_mobile/src/state/notes_notifier.dart';
import '../sqflite_test_setup.dart' as sqflite_setup;

void main() {
  sqflite_setup.setupSqfliteFfi();

  test('starts with an empty list', () {
    final container = ProviderContainer.test();
    expect(container.read(notesProvider), isEmpty);
  });

  test('adds a note with content trimmed', () {
    final container = ProviderContainer.test();
    final problem = container.read(notesProvider.notifier).add(
      rawContent: '  buy milk ',
      projectId: 'default',
    );
    expect(problem, isNull);
    expect(container.read(notesProvider).single.content, 'buy milk');
  });

  test('adds a note with custom source type and metadata', () {
    final container = ProviderContainer.test();
    final problem = container.read(notesProvider.notifier).add(
      rawContent: 'from telegram',
      projectId: 'telegram-proj',
      sourceType: 'telegram',
      metadata: {'telegram_chat_id': '12345'},
    );
    expect(problem, isNull);
    final note = container.read(notesProvider).single;
    expect(note.content, 'from telegram');
    expect(note.projectId, 'telegram-proj');
    expect(note.sourceType, 'telegram');
    expect(note.metadata['telegram_chat_id'], '12345');
  });

  test('refuses a blank content and leaves the list alone', () {
    final container = ProviderContainer.test();
    final problem = container.read(notesProvider.notifier).add(
      rawContent: '   ',
      projectId: 'default',
    );
    expect(problem, NoteProblem.blank);
    expect(container.read(notesProvider), isEmpty);
  });

  test('refuses content past the limit and leaves the list alone', () {
    final container = ProviderContainer.test();
    final long = 'a' * (maxNoteLength + 1);
    final problem = container.read(notesProvider.notifier).add(
      rawContent: long,
      projectId: 'default',
    );
    expect(problem, NoteProblem.tooLong);
    expect(container.read(notesProvider), isEmpty);
  });

  test('gives every note its own id, and never reuses one', () {
    final container = ProviderContainer.test();
    container.read(notesProvider.notifier)
      ..add(rawContent: 'one', projectId: 'default')
      ..add(rawContent: 'two', projectId: 'default')
      ..add(rawContent: 'three', projectId: 'default');
    final ids = container.read(notesProvider).map((n) => n.id).toList();
    expect(ids.length, 3);
    expect(ids.toSet().length, 3); // All unique
  });

  test('update changes content and updatedAt', () {
    final container = ProviderContainer.test();
    container.read(notesProvider.notifier).add(
      rawContent: 'original',
      projectId: 'default',
    );
    final originalNote = container.read(notesProvider).single;
    final originalUpdatedAt = originalNote.updatedAt;

    // Wait a bit to ensure timestamp difference
    final problem = container.read(notesProvider.notifier).update(
      id: originalNote.id,
      newContent: 'updated',
    );
    expect(problem, isNull);

    final updatedNote = container.read(notesProvider).single;
    expect(updatedNote.content, 'updated');
    expect(updatedNote.updatedAt.isAfter(originalUpdatedAt), isTrue);
    expect(updatedNote.id, originalNote.id);
    expect(updatedNote.createdAt, originalNote.createdAt);
  });

  test('update refuses blank content', () {
    final container = ProviderContainer.test();
    container.read(notesProvider.notifier).add(
      rawContent: 'original',
      projectId: 'default',
    );
    final note = container.read(notesProvider).single;

    final problem = container.read(notesProvider.notifier).update(
      id: note.id,
      newContent: '   ',
    );
    expect(problem, NoteProblem.blank);
    expect(container.read(notesProvider).single.content, 'original');
  });

  test('delete removes the note', () {
    final container = ProviderContainer.test();
    container.read(notesProvider.notifier).add(
      rawContent: 'to delete',
      projectId: 'default',
    );
    final note = container.read(notesProvider).single;

    container.read(notesProvider.notifier).delete(note.id);
    expect(container.read(notesProvider), isEmpty);
  });

  test('delete ignores unknown id', () {
    final container = ProviderContainer.test();
    container.read(notesProvider.notifier).add(
      rawContent: 'keep',
      projectId: 'default',
    );

    container.read(notesProvider.notifier).delete('unknown-id');
    expect(container.read(notesProvider).length, 1);
  });

  test('togglePin flips the pinned metadata flag', () {
    final container = ProviderContainer.test();
    container.read(notesProvider.notifier).add(
      rawContent: 'pinnable',
      projectId: 'default',
    );
    final note = container.read(notesProvider).single;
    expect(note.metadata['pinned'], isNull);

    container.read(notesProvider.notifier).togglePin(note.id);
    expect(container.read(notesProvider).single.metadata['pinned'], true);

    container.read(notesProvider.notifier).togglePin(note.id);
    expect(container.read(notesProvider).single.metadata['pinned'], false);
  });

  test('replaceAll replaces the entire list', () {
    final container = ProviderContainer.test();
    container.read(notesProvider.notifier).add(
      rawContent: 'old',
      projectId: 'default',
    );

    final newNotes = [
      Note(
        id: 'new1',
        projectId: 'default',
        content: 'new one',
        sourceType: 'manual',
        createdAt: DateTime(2026, 1, 1),
        updatedAt: DateTime(2026, 1, 1),
      ),
      Note(
        id: 'new2',
        projectId: 'default',
        content: 'new two',
        sourceType: 'manual',
        createdAt: DateTime(2026, 1, 1),
        updatedAt: DateTime(2026, 1, 1),
      ),
    ];

    container.read(notesProvider.notifier).replaceAll(newNotes);
    expect(container.read(notesProvider).length, 2);
    expect(container.read(notesProvider).map((n) => n.content).toList(), ['new one', 'new two']);
  });
}