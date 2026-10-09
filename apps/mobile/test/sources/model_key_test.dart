import 'dart:convert';

import 'package:evomem_mobile/src/clusters/cluster_service.dart';
import 'package:evomem_mobile/src/sources/source_connection.dart';
import 'package:flutter_test/flutter_test.dart';

import 'source_service_test.dart' show Answer, FakePanel, serviceFor;

void main() {
  group('the model key (ADR-0027)', () {
    test('a model row is told apart from a source', () {
      final model = SourceConnection.fromJson(<String, dynamic>{
        'id': 'a',
        'source_type': 'model',
        'project_id': 'default',
        'base_url': 'https://api.openai.com',
        'query': 'gpt-4o-mini',
      });
      final jira = SourceConnection.fromJson(<String, dynamic>{
        'id': 'b',
        'source_type': 'jira',
        'project_id': 'default',
        'base_url': 'https://you.atlassian.net',
        'query': 'project = EVO',
      });

      expect(model!.isModel, isTrue);
      expect(model.modelName, 'gpt-4o-mini');
      expect(jira!.isModel, isFalse);
      // The field means "what we ask this source for". For a source that is
      // a search, so reading it as a model name would be wrong.
      expect(jira.modelName, 'project = EVO');
    });

    test(
      'connecting a model posts the key once and nothing reads it back',
      () async {
        final panel = await FakePanel.start(
          answers: [const Answer(201, '{"connection":{"id":"a"}}')],
        );
        addTearDown(panel.close);

        await serviceFor(panel).connect(
          sourceType: modelSourceType,
          projectId: 'default',
          baseUrl: 'https://api.openai.com',
          query: 'gpt-4o-mini',
          secret: 'the-model-key',
        );

        expect(panel.methods, ['POST /connections']);
        final sent = json.decode(panel.bodies.single) as Map<String, dynamic>;
        expect(sent['source_type'], 'model');
        expect(sent['query'], 'gpt-4o-mini');
        expect(sent['secret'], 'the-model-key');
      },
    );

    test('organize reports what the run did', () async {
      final panel = await FakePanel.start(
        answers: [
          const Answer(
            200,
            '{"offered":7,"created":2,"grouped":5,"invented":1,"dropped":1}',
          ),
        ],
      );
      addTearDown(panel.close);

      final outcome = await serviceFor(panel).organize(projectId: 'evomem');

      expect(panel.methods, ['POST /organize']);
      expect(outcome.offered, 7);
      expect(outcome.created, 2);
      expect(outcome.grouped, 5);
      expect(outcome.invented, 1);
      expect(outcome.dropped, 1);
    });

    test('no model connected is the server saying nothing was sent', () async {
      final panel = await FakePanel.start(
        answers: [
          const Answer(
            503,
            'no model is connected, so nothing was sent anywhere',
          ),
        ],
      );
      addTearDown(panel.close);

      await expectLater(
        serviceFor(panel).organize(),
        throwsA(
          isA<ClusterFailure>().having(
            (f) => f.detail,
            'detail',
            contains('nothing was sent anywhere'),
          ),
        ),
      );
    });
  });
}
