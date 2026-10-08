import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:evomem_mobile/src/app.dart';

/// Pumps the whole app, router and localisations included.
///
/// The tests drive the real screens rather than a widget wrapped in a
/// `MaterialApp` of their own, because the wiring — the router, the provider
/// scope, the delegates — is most of what can break.
Future<void> pumpApp(WidgetTester tester, {String? initialLocation}) async {
  await tester.pumpWidget(
    ProviderScope(child: EvomemApp(initialLocation: initialLocation)),
  );
  await tester.pumpAndSettle();
}

/// Types [content] into the add note field and presses the button that adds it.
Future<void> addNote(WidgetTester tester, String content) async {
  // Find the add note TextField by its labelText
  await tester.enterText(
    find.widgetWithText(TextField, 'Note content'),
    content,
  );
  await tester.tap(find.widgetWithText(ElevatedButton, 'Add Note'));
  await tester.pumpAndSettle();
}
