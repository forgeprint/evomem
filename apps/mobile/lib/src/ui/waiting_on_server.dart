import 'package:evomem_mobile/src/clusters/cluster_providers.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// A spinner that says what it is waiting on.
///
/// A bare spinner is the same picture whether the server is slow, has moved,
/// or was never configured — and the stored address is exactly what tells
/// those apart. Showing it turns "it is stuck" into "it is asking the wrong
/// machine".
class WaitingOnServer extends ConsumerWidget {
  /// Creates the indicator.
  const new({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final settings = ref.watch(serverSettingsProvider);
    final url = settings.asData?.value.url ?? '';

    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const CircularProgressIndicator(),
            const SizedBox(height: 16),
            Text(
              url.isEmpty ? 'Asking the server…' : 'Asking $url…',
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.bodySmall,
            ),
          ],
        ),
      ),
    );
  }
}
