import 'package:flutter_test/flutter_test.dart';

import 'package:dzeroth/features/notification/domain/entities/notification.dart';

void main() {
  group('NotificationEvent.fromString — title-system mappings', () {
    test('title_unlocked maps to NotificationEvent.titleUnlocked', () {
      expect(
        NotificationEvent.fromString('title_unlocked'),
        NotificationEvent.titleUnlocked,
      );
    });

    test('title_grace_period maps to NotificationEvent.titleGracePeriod', () {
      expect(
        NotificationEvent.fromString('title_grace_period'),
        NotificationEvent.titleGracePeriod,
      );
    });
  });

  group('NotificationEvent.fromString — existing mappings unchanged', () {
    test('follow maps to NotificationEvent.follow', () {
      expect(
        NotificationEvent.fromString('follow'),
        NotificationEvent.follow,
      );
    });

    test('mention maps to NotificationEvent.mention', () {
      expect(
        NotificationEvent.fromString('mention'),
        NotificationEvent.mention,
      );
    });

    test('reply maps to NotificationEvent.reply', () {
      expect(
        NotificationEvent.fromString('reply'),
        NotificationEvent.reply,
      );
    });

    test('reaction maps to NotificationEvent.reaction', () {
      expect(
        NotificationEvent.fromString('reaction'),
        NotificationEvent.reaction,
      );
    });
  });

  group('NotificationEvent.fromString — unknown value fallback', () {
    test('unknown value falls back to NotificationEvent.reaction', () {
      expect(
        NotificationEvent.fromString('some_future_event'),
        NotificationEvent.reaction,
      );
    });
  });
}
