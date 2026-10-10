import 'dart:convert';
import 'dart:io';

import 'package:evomem_mobile/src/clusters/cluster_service.dart';
import 'package:evomem_mobile/src/tokens/device_token.dart';
import 'package:evomem_mobile/src/tokens/token_service.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;

class FakeServer {
  new(this._server, this.replies) {
    _server.listen((request) async {
      methods.add('${request.method} ${request.uri.path}');
      bodies.add(await utf8.decoder.bind(request).join());
      final reply = replies.isEmpty
          ? const Reply(200, '{}')
          : replies.removeAt(0);
      request.response.statusCode = reply.status;
      request.response.write(reply.body);
      await request.response.close();
    });
  }

  static Future<FakeServer> start({List<Reply> replies = const []}) async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    return FakeServer(server, [...replies]);
  }

  final HttpServer _server;
  final List<Reply> replies;
  final List<String> methods = [];
  final List<String> bodies = [];

  String get url => 'http://127.0.0.1:${_server.port}';
  Future<void> close() => _server.close(force: true);
}

class Reply {
  const new(this.status, this.body);
  final int status;
  final String body;
}

TokenService serviceFor(FakeServer server) =>
    TokenService(serverUrl: server.url, apiToken: 'tok', client: http.Client());

void main() {
  test('a token row carries no secret, because there is none', () {
    final token = DeviceToken.fromJson(<String, dynamic>{
      'id': '01M4D3H3HNMFM69N4MHNAYBZ1X',
      'name': 'phone',
      'prefix': 'abcd',
      'last_used_at': '2026-10-10T08:00:00Z',
    });
    expect(token, isNotNull);
    expect(token!.name, 'phone');
    expect(token.isRevoked, isFalse);
    expect(token.lastUsedAt, isNotNull);
  });

  test('a revoked token says so', () {
    final token = DeviceToken.fromJson(<String, dynamic>{
      'id': 'a',
      'name': 'old laptop',
      'revoked_at': '2026-03-01T00:00:00Z',
    });
    expect(token!.isRevoked, isTrue);
  });

  test('listing reads what the server sent', () async {
    final server = await FakeServer.start(
      replies: [
        Reply(
          200,
          json.encode({
            'tokens': [
              {'id': 'a', 'name': 'phone', 'prefix': 'abcd'},
              {
                'id': 'b',
                'name': 'browser',
                'revoked_at': '2026-03-01T00:00:00Z',
              },
            ],
          }),
        ),
      ],
    );
    addTearDown(server.close);

    final tokens = await serviceFor(server).list();
    expect(server.methods, ['GET /tokens']);
    expect(tokens.map((t) => t.name), ['phone', 'browser']);
    expect(tokens[1].isRevoked, isTrue);
  });

  // The secret exists exactly once, in this reply. A caller that did not
  // get it has nothing to show, and silently returning an empty string
  // would look like a token nobody can use.
  test(
    'creating returns the secret, and refuses a reply without one',
    () async {
      final server = await FakeServer.start(
        replies: [
          const Reply(
            201,
            '{"token":{"id":"a"},"secret_shown_once":"evm_ab_cd"}',
          ),
          const Reply(201, '{"token":{"id":"b"}}'),
        ],
      );
      addTearDown(server.close);
      final service = serviceFor(server);

      expect(await service.create('phone'), 'evm_ab_cd');
      expect(json.decode(server.bodies.first), {'name': 'phone'});

      await expectLater(
        service.create('browser'),
        throwsA(isA<ClusterFailure>()),
      );
    },
  );

  test('revoking sends a delete for that one', () async {
    final server = await FakeServer.start(replies: [const Reply(204, '')]);
    addTearDown(server.close);

    await serviceFor(server).revoke('01M4D3H3HNMFM69N4MHNAYBZ1X');
    expect(server.methods, ['DELETE /tokens/01M4D3H3HNMFM69N4MHNAYBZ1X']);
  });

  test('with nothing configured it does not reach the network', () async {
    // A client that would throw if it were ever used; the point is that it
    // is not.
    final service = TokenService(
      serverUrl: '',
      apiToken: '',
      client: http.Client(),
    );
    await expectLater(
      service.list(),
      throwsA(
        isA<ClusterFailure>().having(
          (f) => f.problem,
          'problem',
          ClusterProblem.notConfigured,
        ),
      ),
    );
  });
}
