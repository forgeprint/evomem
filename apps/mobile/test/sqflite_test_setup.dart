import 'package:sqflite_common_ffi/sqflite_ffi.dart';
import 'package:flutter_test/flutter_test.dart';

void setupSqfliteFfi() {
  sqfliteFfiInit();
  databaseFactory = databaseFactoryFfi;
}

// This file should be imported by test files that need sqflite
// Usage: import 'package:evomem_mobile/test/sqflite_test_setup.dart' as setup; setup.setupSqfliteFfi();