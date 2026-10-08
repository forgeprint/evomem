import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

/// Import prefixes that would pull the framework into the rules.
const _forbidden = [
  'package:flutter/',
  'package:flutter_riverpod/',
  'package:go_router/',
  'dart:ui',
];

void main() {
  final sources = Directory('lib/src/rules')
      .listSync()
      .whereType<File>()
      .where((file) => file.path.endsWith('.dart'))
      .toList();

  test('there are rule files to check', () {
    // Without this, moving or renaming the folder would make every check
    // below pass by having nothing to check.
    expect(sources.length, greaterThanOrEqualTo(2));
  });

  for (final source in sources) {
    test('${source.path} imports nothing from the framework', () {
      final text = source.readAsStringSync();
      final found = _forbidden
          .where((prefix) => text.contains("import '$prefix"))
          .toList();
      expect(
        found,
        isEmpty,
        reason:
            'The rules are plain Dart so that they can be tested without '
            'rendering anything. Move whatever needs the framework into '
            'lib/src/state or lib/src/ui.',
      );
    });
  }
}
