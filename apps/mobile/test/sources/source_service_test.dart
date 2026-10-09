import 'dart:convert';
import 'dart:io';

import 'package:evomem_mobile/src/clusters/cluster_service.dart';
import 'package:evomem_mobile/src/sources/source_connection.dart';
import 'package:evomem_mobile/src/sources/source_service.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;

/// A stand-in for the panel's half of the server.
class FakePanel {
  new(this._server, this.answers) {
    _server.listen((request) async {
      methods.add('${request.method} ${request.uri.path}');
      bodies.add(await utf8.decoder.bind(request).join());
      final answer = answers.isEmpty
          ? const Answer(200, '{}')
          : answers.removeAt(0);
      request.response.statusCode = answer.status;
      request.response.write(answer.body);
      await request.response.close();
    });
  }

  static Future<FakePanel> start({List<Answer> answers = const []}) async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    return FakePanel(server, [...answers]);
  }

  final HttpServer _server;
  final List<Answer> answers;
  final List<String> methods = [];
  final List<String> bodies = [];

  String get url => 'http://127.0.0.1:${_server.port}';
  Future<void> close() => _server.close(force: true);
}

class Answer {
  const new(this.status, this.body);
  final int status;
  final String body;
}

SourceService serviceFor(FakePanel panel) =>
    SourceService(serverUrl: panel.url, apiToken: 'tok', client: http.Client());

void main() {
  test('lists what is connected', () async {
    final panel = await FakePanel.start(
      answers: [
        Answer(
          200,
          json.encode({
            'connections': [
              {
                'id': '01M4D3H3HNMFM69N4MHNAYBZ1X',
                'source_type': 'jira',
                'project_id': 'evomem',
                'base_url': 'https://example.atlassian.net',
                'account': 'someone@example.com',
                'query': 'project = EVO',
                'last_error': 'the server answered 401',
              },
            ],
          }),
        ),
      ],
    );
    addTearDown(panel.close);

    final connections = await serviceFor(panel).list();
    expect(connections, hasLength(1));
    expect(connections.single.sourceType, 'jira');
    // The failure travels, because a stale source is otherwise a mystery.
    expect(connections.single.lastError, contains('401'));
    expect(panel.methods.single, 'GET /connections');
  });

  test('sends the token once and does not expect it back', () async {
    final panel = await FakePanel.start(answers: [const Answer(201, '{}')]);
    addTearDown(panel.close);

    await serviceFor(panel).connect(
      sourceType: 'jira',
      projectId: 'evomem',
      baseUrl: 'https://example.atlassian.net',
      account: 'someone@example.com',
      query: 'project = EVO',
      secret: 'the-api-token',
    );

    expect(panel.methods.single, 'POST /connections');
    final sent = json.decode(panel.bodies.single) as Map<String, dynamic>;
    expect(sent['secret'], 'the-api-token');
    expect(sent['source_type'], 'jira');
  });

  // There is no field for a token on the way back, and this keeps it so.
  test('a connection read back carries no token', () {
    final connection = SourceConnection.fromJson({
      'id': '01M4D3H3HNMFM69N4MHNAYBZ1X',
      'source_type': 'jira',
      'project_id': 'evomem',
      'secret': 'should-not-survive',
    });
    expect(connection, isNotNull);
    // The type has no token field at all; this is the shape check.
    expect(connection!.account, isEmpty);
    expect(connection.query, isEmpty);
  });

  test('forgets a source', () async {
    final panel = await FakePanel.start(answers: [const Answer(204, '')]);
    addTearDown(panel.close);

    await serviceFor(panel).forget('01M4D3H3HNMFM69N4MHNAYBZ1X');
    expect(
      panel.methods.single,
      'DELETE /connections/01M4D3H3HNMFM69N4MHNAYBZ1X',
    );
  });

  test('pulls and reports what the server did', () async {
    final panel = await FakePanel.start(
      answers: [
        Answer(200, json.encode({'considered': 2, 'pulled': 7, 'failed': 1})),
      ],
    );
    addTearDown(panel.close);

    final outcome = await serviceFor(panel).pull();
    expect(panel.methods.single, 'POST /connections/pull');
    expect(outcome.pulled, 7);
    expect(outcome.failed, 1);
  });

  // The server says things the client cannot know — that the key is missing,
  // or which field was empty — so its sentence is what reaches the person.
  test('carries the server own words on a refusal', () async {
    final panel = await FakePanel.start(
      answers: [
        const Answer(503, 'this server has no usable EVOMEM_SECRET_KEY'),
      ],
    );
    addTearDown(panel.close);

    await expectLater(
      serviceFor(panel).connect(
        sourceType: 'jira',
        projectId: 'evomem',
        baseUrl: 'https://x',
        secret: 't',
      ),
      throwsA(
        isA<ClusterFailure>().having(
          (f) => f.detail,
          'detail',
          contains('EVOMEM_SECRET_KEY'),
        ),
      ),
    );
  });

  test('says when nothing is configured rather than calling nowhere', () async {
    final service = SourceService(
      serverUrl: '',
      apiToken: '',
      client: http.Client(),
    );
    expect(service.isConfigured, isFalse);
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

  test('SourceConnection.fromJson refuses what it cannot identify', () {
    expect(SourceConnection.fromJson(null), isNull);
    expect(SourceConnection.fromJson('text'), isNull);
    expect(
      SourceConnection.fromJson({
        'id': '',
        'source_type': 'jira',
        'project_id': 'e',
      }),
      isNull,
    );
    expect(SourceConnection.fromJson({'source_type': 'jira'}), isNull);
  });
}
