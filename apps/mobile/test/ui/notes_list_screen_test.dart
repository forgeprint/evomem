import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import '../app_harness.dart';
import '../sqflite_test_setup.dart' as sqflite_setup;

void main() {
  sqflite_setup.setupSqfliteFfi();

  testWidgets('adds a typed note and shows it in the list', (tester) async {
    await pumpApp(tester);
    expect(find.text('No notes yet. Tap + to add one.'), findsOneWidget);

    await addNote(tester, 'buy milk');

    expect(find.text('buy milk'), findsOneWidget);
  });

  testWidgets('stores the content trimmed', (tester) async {
    await pumpApp(tester);
    await addNote(tester, '   buy milk   ');
    expect(find.text('buy milk'), findsOneWidget);
  });

  testWidgets('refuses a blank content and says why', (tester) async {
    await pumpApp(tester);
    await addNote(tester, '   ');

    expect(find.text('Note cannot be empty.'), findsOneWidget);
    expect(find.text('No notes yet. Tap + to add one.'), findsOneWidget);
  });

  testWidgets('refuses content past the limit and says the limit', (tester) async {
    await pumpApp(tester);
    await addNote(tester, 'a' * 10001);

    expect(find.text('Note is too long (max 10000 characters).'), findsOneWidget);
    expect(find.text('No notes yet. Tap + to add one.'), findsOneWidget);
  });

  testWidgets('tapping delete removes the note', (tester) async {
    await pumpApp(tester);
    await addNote(tester, 'to delete');
    expect(find.text('to delete'), findsOneWidget);

    // Find the specific dismissible by its key (note id)
    final dismissibleFinder = find.ancestor(
      of: find.text('to delete'),
      matching: find.byType(Dismissible),
    );
    await tester.drag(dismissibleFinder, const Offset(-400, 0));
    await tester.pumpAndSettle();

    // Confirm deletion in dialog - tap "Delete" button
    await tester.tap(find.text('Delete'));
    await tester.pumpAndSettle();

    expect(find.text('to delete'), findsNothing);
  });

  testWidgets('meets the accessibility guidelines a test can check', (tester) async {
    final handle = tester.ensureSemantics();
    await pumpApp(tester);
    await addNote(tester, 'buy milk');

    // Every tappable node carries a label a screen reader can read out, and
    // every one is at least 48 by 48 (Android) and 44 by 44 (iOS).
    await expectLater(tester, meetsGuideline(labeledTapTargetGuideline));
    await expectLater(tester, meetsGuideline(androidTapTargetGuideline));
    await expectLater(tester, meetsGuideline(iOSTapTargetGuideline));
    await expectLater(tester, meetsGuideline(textContrastGuideline));

    handle.dispose();
  });

  testWidgets('search filters the list', (tester) async {
    await pumpApp(tester);
    await addNote(tester, 'buy milk');
    await addNote(tester, 'walk dog');
    await addNote(tester, 'write code');

    // Search for "milk" - use the search field (first TextField with hintText)
    await tester.enterText(
      find.widgetWithText(TextField, 'Search notes...'),
      'milk',
    );
    await tester.pumpAndSettle();

    expect(find.text('buy milk'), findsOneWidget);
    expect(find.text('walk dog'), findsNothing);
    expect(find.text('write code'), findsNothing);

    // Clear search
    await tester.tap(find.byIcon(Icons.clear));
    await tester.pumpAndSettle();

    expect(find.text('buy milk'), findsOneWidget);
    expect(find.text('walk dog'), findsOneWidget);
    expect(find.text('write code'), findsOneWidget);
  });

  testWidgets('pin icon toggles pinned state', (tester) async {
    await pumpApp(tester);
    await addNote(tester, 'pinnable note');

    // Tap pin icon (outlined)
    await tester.tap(find.byTooltip('Pin'));
    await tester.pumpAndSettle();

    // Should show filled pin icon
    expect(find.byTooltip('Unpin'), findsOneWidget);

    // Tap again to unpin
    await tester.tap(find.byTooltip('Unpin'));
    await tester.pumpAndSettle();

    // Should show outlined pin icon
    expect(find.byTooltip('Pin'), findsOneWidget);
  });
}