import 'dart:convert';
import 'dart:io';

import 'package:evomem_mobile/src/clusters/cluster.dart';
import 'package:evomem_mobile/src/clusters/cluster_service.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;

/// A stand-in for the Go server's read side.
///
/// A real server rather than a mocked client: what is under test is what the
/// service does with an answer, including the shapes it should refuse.
class FakeReadSide {
  new(this._server, this.body, this.status) {
    _server.listen((request) async {
      paths.add('${request.uri.path}?${request.uri.query}');
      authorizations.add(request.headers.value('authorization') ?? '');
      request.response.statusCode = status;
      request.response.write(body);
      await request.response.close();
    });
  }

  static Future<FakeReadSide> start({
    required String body,
    int status = 200,
  }) async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    return FakeReadSide(server, body, status);
  }

  final HttpServer _server;
  final String body;
  final int status;
  final List<String> paths = [];
  final List<String> authorizations = [];

  String get url => 'http://127.0.0.1:${_server.port}';
  Future<void> close() => _server.close(force: true);
}

ClusterService serviceFor(FakeReadSide server) => ClusterService(
  serverUrl: server.url,
  apiToken: 'tok',
  client: http.Client(),
);

void main() {
  test('lists what the server grouped', () async {
    final server = await FakeReadSide.start(
      body: json.encode({
        'clusters': [
          {
            'id': '01M4D3H3HNMFM69N4MHNAYBZ1X',
            'project_id': 'evomem',
            'name': 'Deployment',
            'summary': 'what has to be up',
            'size': 2,
          },
        ],
      }),
    );
    addTearDown(server.close);

    final clusters = await serviceFor(server).list(projectId: 'evomem');
    expect(clusters, hasLength(1));
    expect(clusters.single.name, 'Deployment');
    expect(clusters.single.size, 2);
    // The token the read side is behind, and the project it was asked for.
    expect(server.authorizations.single, 'Bearer tok');
    expect(server.paths.single, '/clusters?project=evomem');
  });

  // Another program's output: one malformed entry should not lose the rest.
  test('drops an entry it cannot read and keeps the others', () async {
    final server = await FakeReadSide.start(
      body: json.encode({
        'clusters': [
          {
            'id': '01M4D3H3HNMFM69N4MHNAYBZ1X',
            'project_id': 'e',
            'name': 'Good',
          },
          {'id': '', 'project_id': 'e', 'name': 'No id'},
          {'project_id': 'e', 'name': 'Missing id'},
          'not an object',
        ],
      }),
    );
    addTearDown(server.close);

    final clusters = await serviceFor(server).list(projectId: 'e');
    expect(clusters.map((c) => c.name), ['Good']);
  });

  test('reads one group and the notes in it', () async {
    final server = await FakeReadSide.start(
      body: json.encode({
        'cluster': {
          'id': '01M4D3H3HNMFM69N4MHNAYBZ1X',
          'project_id': 'evomem',
          'name': 'Deployment',
          'size': 1,
        },
        'notes': [
          {
            'id': '01M4D3H3HNMFM69N4MHNAYBZ1Y',
            'project_id': 'evomem',
            'content': 'the tunnel has to be running',
            'source_type': 'mobile',
            'created_at': '2026-10-09T06:00:00Z',
            'updated_at': '2026-10-09T06:00:00Z',
            'metadata': {'tainted': true, 'origin': 'telegram'},
          },
        ],
      }),
    );
    addTearDown(server.close);

    final detail = await serviceFor(server).get('01M4D3H3HNMFM69N4MHNAYBZ1X');
    expect(detail.cluster.name, 'Deployment');
    expect(detail.notes.single.content, 'the tunnel has to be running');
    // The marks travel in the metadata; the screen is what renders them.
    expect(detail.notes.single.metadata['tainted'], isTrue);
    expect(detail.notes.single.sourceType, 'mobile');
  });

  test('says when nothing is configured rather than calling nowhere', () async {
    final service = ClusterService(
      serverUrl: '',
      apiToken: '',
      client: http.Client(),
    );
    expect(service.isConfigured, isFalse);
    await expectLater(
      service.list(projectId: 'e'),
      throwsA(
        isA<ClusterFailure>().having(
          (f) => f.problem,
          'problem',
          ClusterProblem.notConfigured,
        ),
      ),
    );
  });

  test('a server that refuses is unreachable, not empty', () async {
    final server = await FakeReadSide.start(body: 'nope', status: 401);
    addTearDown(server.close);

    await expectLater(
      serviceFor(server).list(projectId: 'e'),
      throwsA(
        isA<ClusterFailure>().having(
          (f) => f.problem,
          'problem',
          ClusterProblem.unreachable,
        ),
      ),
    );
  });

  test('an answer this cannot read is said so', () async {
    final server = await FakeReadSide.start(body: 'not json at all');
    addTearDown(server.close);

    await expectLater(
      serviceFor(server).list(projectId: 'e'),
      throwsA(
        isA<ClusterFailure>().having(
          (f) => f.problem,
          'problem',
          ClusterProblem.unreadable,
        ),
      ),
    );
  });

  test('Cluster.fromJson refuses what it cannot identify', () {
    expect(Cluster.fromJson(null), isNull);
    expect(Cluster.fromJson('text'), isNull);
    expect(Cluster.fromJson(<String, dynamic>{}), isNull);
    expect(Cluster.fromJson({'id': 'x', 'project_id': 'e'}), isNull);
  });
}
