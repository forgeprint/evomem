import 'package:flutter_test/flutter_test.dart';
import 'package:evomem_mobile/src/rules/note.dart';
import 'package:evomem_mobile/src/rules/note_rules.dart';

void main() {
  group('checkNoteContent', () {
    test('refuses nothing typed', () {
      expect(checkNoteContent(''), NoteProblem.blank);
    });

    test('refuses whitespace only', () {
      expect(checkNoteContent('   \t\n '), NoteProblem.blank);
    });

    test('accepts content of exactly the limit', () {
      expect(checkNoteContent('a' * maxNoteLength), isNull);
    });

    test('refuses one character past the limit', () {
      expect(checkNoteContent('a' * (maxNoteLength + 1)), NoteProblem.tooLong);
    });

    test('measures the trimmed content, not what was typed', () {
      expect(checkNoteContent('  ${'a' * maxNoteLength}  '), isNull);
    });
  });

  group('normalizeNoteContent', () {
    test('drops the whitespace around the content', () {
      expect(normalizeNoteContent('  buy milk \n'), 'buy milk');
    });

    test('leaves the inside of the content alone', () {
      expect(normalizeNoteContent('buy  milk'), 'buy  milk');
    });
  });

  group('isValidSourceType', () {
    test('accepts known source types', () {
      for (final type in knownSourceTypes) {
        expect(
          isValidSourceType(type),
          isTrue,
          reason: '$type should be valid',
        );
      }
    });

    test('rejects unknown source types', () {
      expect(isValidSourceType('unknown'), isFalse);
      expect(isValidSourceType(''), isFalse);
    });
  });

  group('Note', () {
    test(
      'withContent creates a new note with updated content and timestamp',
      () {
        final original = Note(
          id: '1',
          projectId: 'proj',
          content: 'original',
          sourceType: 'manual',
          createdAt: DateTime(2026, 1, 1),
          updatedAt: DateTime(2026, 1, 1),
        );
        final updated = original.withContent(
          'new content',
          DateTime(2026, 1, 2),
        );

        expect(updated.id, original.id);
        expect(updated.projectId, original.projectId);
        expect(updated.content, 'new content');
        expect(updated.sourceType, original.sourceType);
        expect(updated.createdAt, original.createdAt);
        expect(updated.updatedAt, DateTime(2026, 1, 2));
        expect(updated.metadata, original.metadata);
      },
    );
  });
}
