// Minimal example app: imports the smackerel_assistant package so that
// `flutter build apk` exercises the shared core through a real Flutter
// application, satisfying spec 073 Scope 1d cross-platform parity proof.

import 'package:flutter/material.dart';
import 'package:smackerel_assistant/smackerel_assistant.dart';

void main() {
  runApp(const SmackerelAssistantExampleApp());
}

class SmackerelAssistantExampleApp extends StatelessWidget {
  const SmackerelAssistantExampleApp({super.key});

  @override
  Widget build(BuildContext context) {
    // Touch one exported symbol so tree-shaking cannot drop the import.
    final state = TurnState(transportMessageId: 'example-0');
    return MaterialApp(
      title: 'Smackerel Assistant Example',
      home: Scaffold(
        appBar: AppBar(title: const Text('Smackerel Assistant Example')),
        body: Center(child: Text('TurnState phase: ${state.phase}')),
      ),
    );
  }
}
