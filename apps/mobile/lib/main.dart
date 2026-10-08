import 'package:evomem_mobile/src/app.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Starts the app inside the one [ProviderScope] every provider is read from.
void main() {
  runApp(const ProviderScope(child: EvomemApp()));
}
