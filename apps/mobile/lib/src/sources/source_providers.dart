import 'package:evomem_mobile/src/clusters/cluster_providers.dart';
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
final sourceConnectionsProvider = FutureProvider<List<SourceConnection>>((
  ref,
) async {
  final service = await ref.watch(sourceServiceProvider.future);
  return await service.list();
});
