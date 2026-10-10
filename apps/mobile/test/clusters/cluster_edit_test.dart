import 'dart:convert';
import 'dart:io';

import 'package:evomem_mobile/src/clusters/cluster_service.dart';
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

ClusterService serviceFor(FakeServer server) => ClusterService(
  serverUrl: server.url,
  apiToken: 'tok',
  client: http.Client(),
);

/// What the server answers an edit with: the grouping as it now stands.
final String _oneGroup = json.encode({
  'cluster': {
    'id': 'c1',
    'project_id': 'evomem',
    'name': 'after',
    'summary': 'new',
    'created_at': '2026-10-10T00:00:00Z',
    'updated_at': '2026-10-10T00:00:00Z',
    'size': 1,
  },
  'notes': [
    {
      'id': 'n1',
      'project_id': 'evomem',
      'content': 'a note',
      'source_type': 'manual',
      'created_at': '2026-10-10T00:00:00Z',
      'updated_at': '2026-10-10T00:00:00Z',
    },
  ],
});

void main() {
  test('editing sends only what was given', () async {
    final server = await FakeServer.start(replies: [Reply(200, _oneGroup)]);
    addTearDown(server.close);

    await serviceFor(server).edit('c1', name: 'after');

    expect(server.methods, ['PUT /clusters/c1']);
    // Null means leave it: a rename that also sent an empty summary would
    // wipe one nobody asked to clear.
    expect(json.decode(server.bodies.single), {'name': 'after'});
  });

  test('an empty summary is a deliberate clear, not an omission', () async {
    final server = await FakeServer.start(replies: [Reply(200, _oneGroup)]);
    addTearDown(server.close);

    await serviceFor(server).edit('c1', summary: '');
    expect(json.decode(server.bodies.single), {'summary': ''});
  });

  // The server answers an edit with what the grouping now is, so the screen
  // is told rather than having to ask again.
  test('editing gives back the grouping as it now stands', () async {
    final server = await FakeServer.start(replies: [Reply(200, _oneGroup)]);
    addTearDown(server.close);

    final detail = await serviceFor(server).edit('c1', name: 'after');
    expect(detail.cluster.name, 'after');
    expect(detail.notes.single.content, 'a note');
  });

  test('removing a note names it', () async {
    final server = await FakeServer.start(replies: [Reply(200, _oneGroup)]);
    addTearDown(server.close);

    await serviceFor(server).edit('c1', remove: ['n1']);
    expect(json.decode(server.bodies.single), {
      'remove': ['n1'],
    });
  });

  test('an edit with nothing in it never reaches the network', () async {
    final server = await FakeServer.start();
    addTearDown(server.close);

    await expectLater(
      serviceFor(server).edit('c1'),
      throwsA(isA<ClusterFailure>()),
    );
    expect(server.methods, isEmpty);
  });

  test('deleting sends a delete for that one', () async {
    final server = await FakeServer.start(replies: [const Reply(204, '')]);
    addTearDown(server.close);

    await serviceFor(server).delete('c1');
    expect(server.methods, ['DELETE /clusters/c1']);
  });

  // The server's own sentence says things the client cannot know, like
  // which field it refused.
  test('a refusal carries what the server said', () async {
    final server = await FakeServer.start(
      replies: [const Reply(400, 'a grouping needs a name')],
    );
    addTearDown(server.close);

    await expectLater(
      serviceFor(server).edit('c1', name: ' '),
      throwsA(
        isA<ClusterFailure>().having(
          (f) => f.detail,
          'detail',
          contains('needs a name'),
        ),
      ),
    );
  });
}
