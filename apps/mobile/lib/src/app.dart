import 'package:evomem_mobile/l10n/app_localizations.dart';
import 'package:evomem_mobile/src/routing/router.dart';
import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

/// The application: theme, localisations and the router.
///
/// Stateful only to hold the router, which must outlive a rebuild; nothing
/// else here keeps state.
class EvomemApp extends StatefulWidget {
  /// Creates the application widget.
  const new({super.key, this.initialLocation});

  /// Where the app opens, when a test wants somewhere other than the list.
  final String? initialLocation;

  @override
  State<EvomemApp> createState() => _EvomemAppState();
}

class _EvomemAppState extends State<EvomemApp> {
  late final GoRouter _router = widget.initialLocation == null
      ? buildRouter()
      : buildRouter(initialLocation: widget.initialLocation!);

  @override
  Widget build(BuildContext context) {
    return MaterialApp.router(
      onGenerateTitle: (context) => AppLocalizations.of(context).appTitle,
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      theme: buildTheme(),
      routerConfig: _router,
    );
  }
}

/// The theme.
///
/// The one thing here that is not decoration: a button is 48 logical pixels
/// tall. Material 3 ships 40, which is below the 48 by 48 target Android's
/// accessibility guidance asks for, and `androidTapTargetGuideline` in the
/// widget tests fails the build when a control is smaller.
ThemeData buildTheme() {
  return ThemeData(
    colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xFF3F51B5)),
    useMaterial3: true,
    elevatedButtonTheme: ElevatedButtonThemeData(
      style: ElevatedButton.styleFrom(
        minimumSize: const Size(64, 48),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      ),
    ),
    textButtonTheme: TextButtonThemeData(
      style: TextButton.styleFrom(
        minimumSize: const Size(64, 48),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        minimumSize: const Size(64, 48),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      ),
    ),
    inputDecorationTheme: InputDecorationTheme(
      border: OutlineInputBorder(borderRadius: BorderRadius.circular(8)),
      filled: true,
      fillColor: Colors.grey.shade50,
    ),
    cardTheme: CardThemeData(
      elevation: 1,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
    ),
    appBarTheme: const AppBarTheme(
      centerTitle: false,
      elevation: 0,
      scrolledUnderElevation: 1,
    ),
  );
}
