import 'dart:convert';

import 'package:evomem_mobile/src/clusters/cluster_service.dart'
    show ClusterFailure, ClusterProblem;
import 'package:evomem_mobile/src/sources/source_connection.dart';
import 'package:http/http.dart' as http;

/// Manages the sources the server pulls from.
///
/// The panel's half of ADR-0026. Reuses [ClusterFailure] rather than
/// inventing a second vocabulary for the same three problems: nothing
/// configured, nothing reachable, nothing readable.
class SourceService {
  /// Talks to [serverUrl] with [apiToken].
  const new({
    required this.serverUrl,
    required this.apiToken,
    required this.client,
    this.timeout = const Duration(seconds: 20),
    this.pullTimeout = const Duration(minutes: 2),
  });

  /// Where the server is.
  final String serverUrl;

  /// The bearer token every route is behind.
  final String apiToken;

  /// The client to send with.
  final http.Client client;

  /// How long an ordinary request may take.
  final Duration timeout;

  /// How long a pull may take.
  ///
  /// Longer than the rest: the server runs it synchronously and a source with
  /// a lot of issues holds the request open (ADR-0026).
  final Duration pullTimeout;

  /// Whether there is anything to ask.
  bool get isConfigured => serverUrl.isNotEmpty && apiToken.isNotEmpty;

  /// What is connected.
  Future<List<SourceConnection>> list() async {
    final body = await _send('GET', '/connections', timeout: timeout);
    final raw = body['connections'];
    if (raw is! List) {
      throw const ClusterFailure(
        ClusterProblem.unreadable,
        'no connections array',
      );
    }
    return raw
        .map(SourceConnection.fromJson)
        .whereType<SourceConnection>()
        .toList();
  }

  /// Connects a source. The token is sent once and never comes back.
  Future<void> connect({
    required String sourceType,
    required String projectId,
    required String baseUrl,
    required String secret,
    String account = '',
    String query = '',
  }) async {
    await _send(
      'POST',
      '/connections',
      timeout: timeout,
      body: {
        'source_type': sourceType,
        'project_id': projectId,
        'base_url': baseUrl,
        'account': account,
        'query': query,
        'secret': secret,
      },
    );
  }

  /// Forgets a source, and its token with it.
  Future<void> forget(String id) async {
    await _send(
      'DELETE',
      '/connections/${Uri.encodeComponent(id)}',
      timeout: timeout,
    );
  }

  /// Pulls from every connected source now.
  Future<PullOutcome> pull() async {
    final body = await _send('POST', '/connections/pull', timeout: pullTimeout);
    return PullOutcome.fromJson(body);
  }

  /// Asks the connected model to group what nothing has grouped yet.
  ///
  /// Pressing this sends the text of those notes to whatever server the
  /// stored key points at (ADR-0027). With no model connected the server
  /// answers that nothing was sent anywhere.
  Future<OrganizeOutcome> organize({String projectId = ''}) async {
    final query = projectId.isEmpty
        ? ''
        : '?project=${Uri.encodeQueryComponent(projectId)}';
    final body = await _send('POST', '/organize$query', timeout: pullTimeout);
    return OrganizeOutcome.fromJson(body);
  }

  Future<Map<String, dynamic>> _send(
    String method,
    String path, {
    required Duration timeout,
    Map<String, Object?>? body,
  }) async {
    if (!isConfigured) {
      throw const ClusterFailure(ClusterProblem.notConfigured);
    }

    final request = http.Request(method, Uri.parse('$serverUrl$path'))
      ..headers['Authorization'] = 'Bearer $apiToken';
    if (body != null) {
      request.headers['Content-Type'] = 'application/json';
      request.body = json.encode(body);
    }

    final http.Response response;
    try {
      final streamed = await client.send(request).timeout(timeout);
      response = await http.Response.fromStream(streamed);
    } on Exception catch (e) {
      throw ClusterFailure(ClusterProblem.unreachable, e.toString());
    }

    if (response.statusCode < 200 || response.statusCode >= 300) {
      // The server's own sentence, which says things the client cannot know:
      // that EVOMEM_SECRET_KEY is missing, or which field was empty.
      throw ClusterFailure(
        ClusterProblem.unreachable,
        response.body.trim().isEmpty
            ? 'the server answered ${response.statusCode}'
            : response.body.trim(),
      );
    }

    if (response.body.trim().isEmpty) return const {};
    try {
      final decoded = json.decode(response.body);
      return decoded is Map<String, dynamic> ? decoded : const {};
    } on FormatException catch (e) {
      throw ClusterFailure(ClusterProblem.unreadable, e.message);
    }
  }
}
