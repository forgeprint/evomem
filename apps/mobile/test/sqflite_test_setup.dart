import 'dart:io';

import 'package:evomem_mobile/src/storage/database.dart';
import 'package:path/path.dart' as p;
import 'package:sqflite_common_ffi/sqflite_ffi.dart';

/// Points sqflite at the in-process ffi implementation, and the store at a
/// file of this test process's own.
///
/// Two separate reasons, both about tests sharing something they should not:
///
/// `databaseFactoryFfiNoIsolate`, not `databaseFactoryFfi`: the isolate-backed
/// factory completes its futures in real time, while `testWidgets` runs inside
/// FakeAsync, whose clock `pumpAndSettle` advances instead. A write would then
/// still be in flight when the widget tree was disposed, which surfaces as a
/// pending 10-second timer — sqflite's lock warning — and a flaky test.
///
/// The database path, because `flutter test` runs test files in parallel and
/// `DatabaseHelper` picks one name under one temporary directory. Every file
/// opening it was opening the same file, so one test's rows were another
/// test's starting state and a test needing a particular schema version could
/// not have one.
void setupSqfliteFfi() {
  sqfliteFfiInit();
  databaseFactory = databaseFactoryFfiNoIsolate;

  final dir = Directory.systemTemp.createTempSync('evomem-test-store');
  DatabaseHelper.databasePathOverride = p.join(dir.path, 'evomem.db');
  DatabaseHelper.instance.forgetConnection();
}
