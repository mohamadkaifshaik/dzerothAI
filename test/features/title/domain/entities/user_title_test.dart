import 'package:flutter_test/flutter_test.dart';

import 'package:dzeroth/features/title/domain/entities/user_title.dart';

void main() {
  // ---------------------------------------------------------------------------
  // TestTitleSummary_Equality
  // ---------------------------------------------------------------------------

  group('TestTitleSummary_Equality', () {
    test('equal when slug and displayName match', () {
      const a = TitleSummary(slug: 'pioneer', displayName: 'Pioneer');
      const b = TitleSummary(slug: 'pioneer', displayName: 'Pioneer');
      expect(a, equals(b));
    });

    test('not equal when slug differs', () {
      const a = TitleSummary(slug: 'pioneer', displayName: 'Pioneer');
      const b = TitleSummary(slug: 'legend', displayName: 'Pioneer');
      expect(a, isNot(equals(b)));
    });

    test('not equal when displayName differs', () {
      const a = TitleSummary(slug: 'pioneer', displayName: 'Pioneer');
      const b = TitleSummary(slug: 'pioneer', displayName: 'Legend');
      expect(a, isNot(equals(b)));
    });
  });

  // ---------------------------------------------------------------------------
  // TestUserTitle_Equality
  // ---------------------------------------------------------------------------

  group('TestUserTitle_Equality', () {
    final dt = DateTime(2026, 1, 1);

    test('equal when all fields match', () {
      final a = UserTitle(
        id: 'id-1',
        slug: 'pioneer',
        displayName: 'Pioneer',
        category: 'founding',
        isRevocable: false,
        status: 'active',
        unlockedAt: dt,
      );
      final b = UserTitle(
        id: 'id-1',
        slug: 'pioneer',
        displayName: 'Pioneer',
        category: 'founding',
        isRevocable: false,
        status: 'active',
        unlockedAt: dt,
      );
      expect(a, equals(b));
    });

    test('not equal when id differs', () {
      final a = UserTitle(
        id: 'id-1',
        slug: 'pioneer',
        displayName: 'Pioneer',
        category: 'founding',
        isRevocable: false,
        status: 'active',
        unlockedAt: dt,
      );
      final b = UserTitle(
        id: 'id-2',
        slug: 'pioneer',
        displayName: 'Pioneer',
        category: 'founding',
        isRevocable: false,
        status: 'active',
        unlockedAt: dt,
      );
      expect(a, isNot(equals(b)));
    });

    test('props contains all fields', () {
      final title = UserTitle(
        id: 'id-1',
        slug: 'pioneer',
        displayName: 'Pioneer',
        category: 'founding',
        isRevocable: true,
        status: 'active',
        unlockedAt: dt,
      );
      expect(title.props, [
        'id-1',
        'pioneer',
        'Pioneer',
        'founding',
        true,
        'active',
        dt,
      ]);
    });
  });
}
