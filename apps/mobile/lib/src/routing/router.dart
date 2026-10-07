import 'package:go_router/go_router.dart';
import 'package:evomem_mobile/src/routing/routes.dart';
import 'package:evomem_mobile/src/ui/missing_screen.dart';
import 'package:evomem_mobile/src/ui/notes_list_screen.dart';
import 'package:evomem_mobile/src/ui/note_detail_screen.dart';
import 'package:evomem_mobile/src/ui/settings_screen.dart';
import 'package:evomem_mobile/src/ui/sync_status_screen.dart';

/// Builds the router.
///
/// A new one per call: a `GoRouter` owns navigation history, so a test that
/// reused one would start where the previous test finished. [initialLocation]
/// is how a test opens a deep link without tapping its way there.
GoRouter buildRouter({String initialLocation = NotesListRoute.path}) {
  return GoRouter(
    initialLocation: initialLocation,
    // Anything the routes below do not match — a stale link, a typo, a
    // location from another version of the app — lands here rather than on
    // the framework's red error page.
    errorBuilder: (context, state) => const MissingScreen(),
    routes: [
      GoRoute(
        path: NotesListRoute.path,
        builder: (context, state) => const NotesListScreen(),
      ),
      GoRoute(
        path: NoteDetailRoute.path,
        builder: (context, state) {
          final route = parseNoteDetailRoute(state);
          if (route == null) return const MissingScreen();
          return NoteDetailScreen(noteId: route.id);
        },
      ),
      GoRoute(
        path: SettingsRoute.path,
        builder: (context, state) => const SettingsScreen(),
      ),
      GoRoute(
        path: SyncStatusRoute.path,
        builder: (context, state) => const SyncStatusScreen(),
      ),
    ],
  );
}