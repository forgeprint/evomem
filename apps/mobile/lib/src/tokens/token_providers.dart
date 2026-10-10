import 'package:evomem_mobile/src/clusters/cluster_providers.dart';
import 'package:evomem_mobile/src/clusters/cluster_service.dart';
import 'package:evomem_mobile/src/log/log.dart';
import 'package:evomem_mobile/src/tokens/device_token.dart';
import 'package:evomem_mobile/src/tokens/token_service.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Manages which devices can reach the server.
final tokenServiceProvider = FutureProvider<TokenService>((ref) async {
  final settings = await ref.watch(serverSettingsProvider.future);
  return TokenService(
    serverUrl: settings.url,
    apiToken: settings.token,
    client: ref.watch(httpClientProvider),
  );
});

/// What can get in, asked each time: tokens live on the server.
final deviceTokensProvider = FutureProvider<List<DeviceToken>>((ref) async {
  final service = await ref.watch(tokenServiceProvider.future);
  final log = ref.read(logbookProvider.notifier);

  if (!service.isConfigured) {
    log.warn('tokens', 'no server configured; nothing was asked');
    throw const ClusterFailure(ClusterProblem.notConfigured);
  }

  try {
    log.info('tokens', 'asking ${service.serverUrl}/tokens');
    final tokens = await service.list();
    log.info('tokens', '${tokens.length} token(s)');
    return tokens;
  } on Object catch (e) {
    log.error('tokens', 'could not list the tokens', e);
    rethrow;
  }
});
