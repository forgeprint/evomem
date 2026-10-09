import 'package:evomem_mobile/src/clusters/cluster_service.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  // The default Object.toString is the class name, and a release build has
  // no class names — so the log read `Instance of 'minified:fn'` the first
  // time somebody needed it. A failure has to be able to say what it was.
  test('a failure says what went wrong, not its own type', () {
    const failure = ClusterFailure(ClusterProblem.notConfigured);
    expect(failure.toString(), contains('no server address'));
    expect(failure.toString(), isNot(contains('Instance of')));
  });

  test('the detail comes with it', () {
    const failure = ClusterFailure(
      ClusterProblem.unreachable,
      'Connection refused',
    );
    expect(failure.toString(), contains('could not be reached'));
    expect(failure.toString(), contains('Connection refused'));
  });

  test('every problem has words', () {
    for (final problem in ClusterProblem.values) {
      final text = ClusterFailure(problem).toString();
      expect(text, isNotEmpty);
      expect(text, isNot(contains('Instance of')));
    }
  });
}
