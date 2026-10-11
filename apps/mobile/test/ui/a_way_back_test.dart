import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

/// Screens reached from the notes list, and where back has to go.
///
/// They are all navigated to with `context.go`, which replaces the screen
/// rather than stacking on it, so Flutter adds no back button of its own. In
/// a browser nobody noticed — the browser's back button did the job. The
/// first time the app ran on a phone, the Log screen had no way out at all.
const _wayBack = <String, String>{
  'log_screen.dart': 'NotesListRoute',
  'clusters_screen.dart': 'NotesListRoute',
  'sources_screen.dart': 'NotesListRoute',
  'devices_screen.dart': 'SourcesRoute',
  'cluster_detail_screen.dart': 'ClustersRoute',
  'settings_screen.dart': 'NotesListRoute',
  'sync_status_screen.dart': 'NotesListRoute',
};

void main() {
  group('every screen has a way back', () {
    for (final entry in _wayBack.entries) {
      test(entry.key, () {
        final source = File('lib/src/ui/${entry.key}').readAsStringSync();

        // Either the shared widget or the hand-written IconButton the
        // older screens use; what matters is that the app bar has a
        // leading control and that it goes somewhere.
        final appBarLeading = RegExp(r'appBar: AppBar\([\s\S]{0,400}?leading: ')
            .hasMatch(source);
        expect(
          appBarLeading,
          isTrue,
          reason: '${entry.key} is a room with no door on a phone',
        );
        expect(source, contains(entry.value));
      });
    }
  });
}
