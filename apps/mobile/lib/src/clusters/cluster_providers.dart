import 'package:evomem_mobile/src/clusters/cluster.dart';
import 'package:evomem_mobile/src/clusters/cluster_service.dart';
import 'package:evomem_mobile/src/state/notes_notifier.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:http/http.dart' as http;

/// Where the server is and what opens it, as entered in settings.
///
/// Read through a provider so a test can supply them without a keychain.
final serverSettingsProvider = FutureProvider<({String url, String token})>((
  ref,
) async {
  const storage = FlutterSecureStorage();
  final url = await storage.read(key: 'evomem_server_url') ?? '';
  final token = await storage.read(key: 'evomem_api_token') ?? '';
  return (url: url.trim(), token: token.trim());
});

/// The client the cluster service sends with.
final httpClientProvider = Provider<http.Client>((ref) {
  final client = http.Client();
  ref.onDispose(client.close);
  return client;
});

/// Reads the groupings the server made.
final clusterServiceProvider = FutureProvider<ClusterService>((ref) async {
  final settings = await ref.watch(serverSettingsProvider.future);
  return ClusterService(
    serverUrl: settings.url,
    apiToken: settings.token,
    client: ref.watch(httpClientProvider),
  );
});

/// The groupings in the current project.
///
/// A future rather than a stored list: clusters live on the server and
/// nowhere else, so this is a question asked each time rather than a copy
/// kept in step (ADR-0024).
final clustersProvider = FutureProvider<List<Cluster>>((ref) async {
  final service = await ref.watch(clusterServiceProvider.future);
  return await service.list(projectId: ref.watch(currentProjectProvider));
});
