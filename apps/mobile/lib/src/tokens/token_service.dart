import 'dart:convert';

import 'package:evomem_mobile/src/clusters/cluster_service.dart'
    show ClusterFailure, ClusterProblem;
import 'package:evomem_mobile/src/http/utf8_body.dart';
import 'package:evomem_mobile/src/tokens/device_token.dart';
import 'package:http/http.dart' as http;

/// Manages which devices can reach the server (ADR-0030).
class TokenService {
  /// Talks to [serverUrl] with [apiToken].
  const new({
    required this.serverUrl,
    required this.apiToken,
    required this.client,
    this.timeout = const Duration(seconds: 20),
  });

  /// Where the server is.
  final String serverUrl;

  /// The bearer token these routes are behind — which may itself be a
  /// device token: a credential that could not make another one would need
  /// a second way in.
  final String apiToken;

  /// The client to send with.
  final http.Client client;

  /// How long a request may take.
  final Duration timeout;

  /// Whether there is anything to ask.
  bool get isConfigured => serverUrl.isNotEmpty && apiToken.isNotEmpty;

  /// What can get in.
  Future<List<DeviceToken>> list() async {
    final body = await _send('GET', '/tokens');
    final raw = body['tokens'];
    if (raw is! List) {
      throw const ClusterFailure(ClusterProblem.unreadable, 'no tokens array');
    }
    return raw.map(DeviceToken.fromJson).whereType<DeviceToken>().toList();
  }

  /// Mints one, returning the secret.
  ///
  /// The caller has to show it immediately: this is the only time it
  /// exists, and nothing can read it back.
  Future<String> create(String name) async {
    final body = await _send('POST', '/tokens', body: {'name': name});
    final secret = body['secret_shown_once'];
    if (secret is! String || secret.isEmpty) {
      throw const ClusterFailure(
        ClusterProblem.unreadable,
        'the server made a token but did not hand it over',
      );
    }
    return secret;
  }

  /// Turns one off.
  Future<void> revoke(String id) async {
    await _send('DELETE', '/tokens/${Uri.encodeComponent(id)}');
  }

  Future<Map<String, dynamic>> _send(
    String method,
    String path, {
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
      throw ClusterFailure(
        ClusterProblem.unreachable,
        utf8Body(response).trim().isEmpty
            ? 'the server answered ${response.statusCode}'
            : utf8Body(response).trim(),
      );
    }

    if (utf8Body(response).trim().isEmpty) return const {};
    try {
      final decoded = json.decode(utf8Body(response));
      return decoded is Map<String, dynamic> ? decoded : const {};
    } on FormatException catch (e) {
      throw ClusterFailure(ClusterProblem.unreadable, e.message);
    }
  }
}
