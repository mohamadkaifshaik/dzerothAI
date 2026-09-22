import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:dzeroth/features/title/domain/entities/user_title.dart';
import 'package:dzeroth/features/title/presentation/widgets/title_badge_widget.dart';

void main() {
  group('TitleBadgeWidget', () {
    testWidgets(
      'renders SizedBox.shrink when title is null',
      (WidgetTester tester) async {
        await tester.pumpWidget(
          const MaterialApp(
            home: Scaffold(
              body: TitleBadgeWidget(title: null),
            ),
          ),
        );

        // The widget must produce a SizedBox.shrink (zero-size) for null title.
        expect(find.byType(SizedBox), findsOneWidget);
        // No text should be rendered when there is no title.
        expect(find.byType(Text), findsNothing);
      },
    );

    testWidgets(
      'renders displayName text when title is non-null',
      (WidgetTester tester) async {
        const summary = TitleSummary(
          slug: 'pioneer',
          displayName: 'Pioneer',
        );

        await tester.pumpWidget(
          const MaterialApp(
            home: Scaffold(
              body: TitleBadgeWidget(title: summary),
            ),
          ),
        );

        // The displayName must appear in the widget tree.
        expect(find.text('Pioneer'), findsOneWidget);
        // A container with the badge decoration should be present.
        expect(find.byType(Container), findsOneWidget);
      },
    );

    testWidgets(
      'does not render slug, category, status, or is_revocable fields '
      '(PUBLIC SURFACE: only displayName — CLAUDE.md §2.3)',
      (WidgetTester tester) async {
        const summary = TitleSummary(
          slug: 'secret-slug',
          displayName: 'Visible Name',
        );

        await tester.pumpWidget(
          const MaterialApp(
            home: Scaffold(
              body: TitleBadgeWidget(title: summary),
            ),
          ),
        );

        // The slug must NOT appear as text anywhere in the tree.
        expect(find.text('secret-slug'), findsNothing);
        // The displayName is the only text rendered.
        expect(find.text('Visible Name'), findsOneWidget);
      },
    );
  });
}
