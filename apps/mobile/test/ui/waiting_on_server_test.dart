import 'package:evomem_mobile/src/clusters/cluster_providers.dart';
import 'package:evomem_mobile/src/ui/waiting_on_server.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

Future<void> pumpWith(WidgetTester tester, String url) async {
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        serverSettingsProvider.overrideWith(
          (ref) async => (url: url, token: 'tok'),
        ),
      ],
      child: const MaterialApp(home: Scaffold(body: WaitingOnServer())),
    ),
  );
  await tester.pump();
}

void main() {
  // A bare spinner looks the same whether the server is slow, has moved, or
  // was never configured. The stored address is what tells them apart.
  testWidgets('it names the address it is waiting on', (tester) async {
    await pumpWith(tester, 'http://192.168.1.50:8787');
    expect(find.textContaining('192.168.1.50:8787'), findsOneWidget);
  });

  testWidgets('with no address it still says what it is doing', (tester) async {
    await pumpWith(tester, '');
    expect(find.textContaining('Asking the server'), findsOneWidget);
    expect(find.byType(CircularProgressIndicator), findsOneWidget);
  });
}
