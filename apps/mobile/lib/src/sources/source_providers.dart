import 'package:evomem_mobile/src/clusters/cluster_providers.dart';
import 'package:evomem_mobile/src/clusters/cluster_service.dart';
import 'package:evomem_mobile/src/log/log.dart';
import 'package:evomem_mobile/src/sources/source_connection.dart';
import 'package:evomem_mobile/src/sources/source_service.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Manages the sources the server pulls from.
final sourceServiceProvider = FutureProvider<SourceService>((ref) async {
  final settings = await ref.watch(serverSettingsProvider.future);
  return SourceService(
    serverUrl: settings.url,
    apiToken: settings.token,
    client: ref.watch(httpClientProvider),
  );
});

/// What is connected, asked each time rather than kept in step: connections
/// live on the server and nowhere else.
///
/// Both ends of the request are written down. A screen that sits on a
/// spinner says nothing about which address it is waiting on, and a stored
/// setting that points at a server which has moved looks exactly like one
/// that is slow.
final sourceConnectionsProvider = FutureProvider<List<SourceConnection>>((
  ref,
) async {
  final service = await ref.watch(sourceServiceProvider.future);
  final log = ref.read(logbookProvider.notifier);
  if (!service.isConfigured) {
    // Said outright rather than as a failed request. "asking /connections"
    // with nothing in front of the slash is what the log printed the first
    // time, and it reads as a bug in the app rather than an empty setting.
    log.warn('panel', 'no server configured; nothing was asked');
    throw const ClusterFailure(ClusterProblem.notConfigured);
  }

  try {
    log.info('panel', 'asking ${service.serverUrl}/connections');
    final connections = await service.list();
    log.info('panel', '${connections.length} connection(s)');
    return connections;
  } on Object catch (e) {
    log.error('panel', 'could not list the connections', e);
    rethrow;
  }
});
