import 'package:evomem_mobile/src/audio/recording_store.dart';
import 'package:evomem_mobile/src/audio/recording_store_io.dart';
import 'package:evomem_mobile/src/clusters/cluster_providers.dart';
import 'package:evomem_mobile/src/sync/sync_service.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import '../sqflite_test_setup.dart' as sqflite_setup;

/// Long enough for the restore the notifier starts in build().
Future<void> settle() => Future<void>.delayed(const Duration(milliseconds: 80));

/// Builds the notifier and lets the restore it starts in build() finish.
Future<SyncServiceState> built(ProviderContainer container) async {
  container.read(syncServiceProvider);
  await settle();
  final state = container.read(syncServiceProvider);
  return state;
}

ProviderContainer containerWith({required String url, required String token}) {
  final container = ProviderContainer(
    overrides: [
      serverSettingsProvider.overrideWith(
        (ref) async => (url: url, token: token),
      ),
      recordingStoreProvider.overrideWithValue(FileRecordingStore()),
    ],
  );
  addTearDown(container.dispose);
  return container;
}

void main() {
  sqflite_setup.setupSqfliteFfi();

  // initialize() used to be called from exactly one place: the settings
  // screen's save handler. So a reload with perfectly good settings left
  // the sync service unconfigured, and the status screen told somebody who
  // had just entered them to go and enter them. The other screens read the
  // stored settings directly, which is why they worked and this did not.
  test('it configures itself from settings already stored', () async {
    final container = containerWith(url: 'http://localhost:8787', token: 'tok');

    final state = await built(container);
    expect(
      state.isInitialized,
      isTrue,
      reason: 'a reload leaves sync unconfigured until somebody saves again',
    );
    expect(state.config?.serverUrl, 'http://localhost:8787');
  });

  test('with nothing stored it stays unconfigured', () async {
    final container = containerWith(url: '', token: '');

    expect((await built(container)).isInitialized, isFalse);
  });

  test('half a setting is not a setting', () async {
    final container = containerWith(url: 'http://localhost:8787', token: '');

    expect((await built(container)).isInitialized, isFalse);
  });
}
