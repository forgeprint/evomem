import 'package:sqflite_common_ffi/sqflite_ffi.dart';

/// Points sqflite at the in-process ffi implementation for tests.
///
/// `databaseFactoryFfiNoIsolate`, not `databaseFactoryFfi`: the isolate-backed
/// factory completes its futures in real time, while `testWidgets` runs inside
/// FakeAsync, whose clock `pumpAndSettle` advances instead. A write would then
/// still be in flight when the widget tree was disposed, which surfaces as a
/// pending 10-second timer — sqflite's lock warning — and a flaky test.
void setupSqfliteFfi() {
  sqfliteFfiInit();
  databaseFactory = databaseFactoryFfiNoIsolate;
}
