import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:evomem_mobile/l10n/app_localizations.dart';
import 'package:evomem_mobile/src/routing/routes.dart';

/// Where a link the app cannot serve ends up.
///
/// A deep link is input, so this is the app's error path, not a placeholder:
/// it says what happened and offers the one way back.
class MissingScreen extends StatelessWidget {
  /// Creates the screen.
  const new({super.key});

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return Scaffold(
      appBar: AppBar(title: Text(l10n.appTitle)),
      body: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(l10n.pageMissing),
            const SizedBox(height: 12),
            ElevatedButton(
              onPressed: () => context.go(const NotesListRoute().location),
              child: Text(l10n.backToList),
            ),
          ],
        ),
      ),
    );
  }
}
