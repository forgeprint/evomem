import 'package:evomem_mobile/src/clusters/cluster_service.dart';
import 'package:evomem_mobile/src/ui/clusters_screen.dart';
import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('an unconfigured screen says where to set it', () {
    final text = explainClusterProblem(ClusterProblem.notConfigured);
    expect(text, contains('Settings'));
  });

  // The half-hour this sentence saves: flutter_secure_storage on the web is
  // localStorage, which belongs to the origin. Settings saved at localhost
  // are invisible at 127.0.0.1, and the app looks as though it was never
  // configured.
  test('in a browser it warns about the address bar', () {
    final text = explainClusterProblem(ClusterProblem.notConfigured);
    if (kIsWeb) {
      expect(text, contains('address bar'));
    } else {
      expect(text, isNot(contains('address bar')));
    }
  });

  test('every problem explains itself', () {
    for (final problem in ClusterProblem.values) {
      expect(explainClusterProblem(problem), isNotEmpty);
    }
  });
}
