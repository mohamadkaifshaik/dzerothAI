import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:dzeroth/core/error/failures.dart';
import 'package:dzeroth/core/error/result.dart';
import 'package:dzeroth/features/notification/domain/entities/notification.dart';
import 'package:dzeroth/features/notification/domain/repositories/notification_repository.dart';
import 'package:dzeroth/features/notification/presentation/bloc/notification_list_bloc.dart';

// ---------------------------------------------------------------------------
// Mock
// ---------------------------------------------------------------------------

class MockNotificationRepository extends Mock
    implements NotificationRepository {}

// ---------------------------------------------------------------------------
// Fixture data
// ---------------------------------------------------------------------------

const _actor = ActorSummary(
  id: '01900000-0000-7000-8000-000000000001',
  handle: 'alice',
  displayName: 'Alice',
);

Notification _makeNotification(String id, {bool isRead = false}) =>
    Notification(
      id: id,
      event: NotificationEvent.mention,
      actor: _actor,
      postId: 'post-$id',
      isRead: isRead,
      createdAt: DateTime.utc(2026, 9, 18),
    );

NotificationPage _pageWithMore(
  List<Notification> items, {
  String cursor = 'cursor-2',
}) => NotificationPage(items: items, nextCursor: cursor, terminated: false);

NotificationPage _terminatedPage(List<Notification> items) =>
    NotificationPage(items: items, nextCursor: null, terminated: true);

NotificationPage _emptyPage() =>
    const NotificationPage(items: [], nextCursor: null, terminated: false);

final _n1 = _makeNotification('n-1');
final _n2 = _makeNotification('n-2');
final _n3 = _makeNotification('n-3');

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  late MockNotificationRepository mockRepo;

  setUp(() {
    mockRepo = MockNotificationRepository();
  });

  group('NotificationListBloc', () {
    // -----------------------------------------------------------------------
    // Initial fetch → non-terminated
    // -----------------------------------------------------------------------

    blocTest<NotificationListBloc, NotificationListState>(
      'NotificationListBloc_EmitsLoaded_OnSuccessfulFetch: '
      'emits [Loading, Loaded] when repository returns non-terminated page',
      build: () {
        when(() => mockRepo.getNotifications(cursor: any(named: 'cursor')))
            .thenAnswer((_) async => Success(_pageWithMore([_n1, _n2])));
        return NotificationListBloc(notificationRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const NotificationListFetchRequested()),
      expect: () => [
        const NotificationListLoading(),
        NotificationListLoaded(
          notifications: [_n1, _n2],
          nextCursor: 'cursor-2',
          hasMore: true,
        ),
      ],
      verify: (_) {
        verify(() => mockRepo.getNotifications(cursor: null)).called(1);
      },
    );

    // -----------------------------------------------------------------------
    // Initial fetch → terminated immediately
    // -----------------------------------------------------------------------

    blocTest<NotificationListBloc, NotificationListState>(
      'NotificationListBloc_EmitsTerminated_WhenTerminatedFlagTrue: '
      'emits [Loading, Terminated] when repository returns terminated:true',
      build: () {
        when(() => mockRepo.getNotifications(cursor: any(named: 'cursor')))
            .thenAnswer((_) async => Success(_terminatedPage([_n1, _n2])));
        return NotificationListBloc(notificationRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const NotificationListFetchRequested()),
      expect: () => [
        const NotificationListLoading(),
        NotificationListTerminated(notifications: [_n1, _n2]),
      ],
    );

    // -----------------------------------------------------------------------
    // Initial fetch → empty
    // -----------------------------------------------------------------------

    blocTest<NotificationListBloc, NotificationListState>(
      'NotificationListBloc_EmitsEmpty_WhenNoItems: '
      'emits [Loading, Empty] when repository returns empty non-terminated page',
      build: () {
        when(() => mockRepo.getNotifications(cursor: any(named: 'cursor')))
            .thenAnswer((_) async => Success(_emptyPage()));
        return NotificationListBloc(notificationRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const NotificationListFetchRequested()),
      expect: () => [
        const NotificationListLoading(),
        const NotificationListEmpty(),
      ],
    );

    // -----------------------------------------------------------------------
    // Repository failure → NotificationListError
    // -----------------------------------------------------------------------

    blocTest<NotificationListBloc, NotificationListState>(
      'NotificationListBloc_EmitsError_OnFailure: '
      'emits [Loading, Error] when repository returns a failure',
      build: () {
        when(() => mockRepo.getNotifications(cursor: any(named: 'cursor')))
            .thenAnswer((_) async => const Err(NetworkFailure('unreachable')));
        return NotificationListBloc(notificationRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const NotificationListFetchRequested()),
      expect: () => [
        const NotificationListLoading(),
        isA<NotificationListError>().having(
          (s) => s.failure,
          'failure',
          isA<NetworkFailure>(),
        ),
      ],
    );

    // -----------------------------------------------------------------------
    // Terminated is terminal — next page ignored
    // -----------------------------------------------------------------------

    blocTest<NotificationListBloc, NotificationListState>(
      'NotificationListBloc_IgnoresNextPage_AfterTerminated: '
      'NextPageRequested after Terminated emits nothing and does not call repo',
      build: () => NotificationListBloc(notificationRepository: mockRepo),
      seed: () => NotificationListTerminated(notifications: [_n1]),
      act: (bloc) => bloc.add(const NotificationListNextPageRequested()),
      expect: () => <NotificationListState>[],
      verify: (_) {
        verifyNever(
          () => mockRepo.getNotifications(cursor: any(named: 'cursor')),
        );
      },
    );

    // -----------------------------------------------------------------------
    // Load next page → appends notifications
    // -----------------------------------------------------------------------

    blocTest<NotificationListBloc, NotificationListState>(
      'NotificationListBloc_LoadsNextPage: '
      'fetches next page and appends notifications when hasMore is true',
      build: () {
        when(() => mockRepo.getNotifications(cursor: any(named: 'cursor')))
            .thenAnswer(
              (_) async => Success(_pageWithMore([_n3], cursor: 'cursor-3')),
            );
        return NotificationListBloc(notificationRepository: mockRepo);
      },
      seed: () => NotificationListLoaded(
        notifications: [_n1, _n2],
        nextCursor: 'cursor-2',
        hasMore: true,
      ),
      act: (bloc) => bloc.add(const NotificationListNextPageRequested()),
      expect: () => [
        // In-flight: hasMore locked to false.
        NotificationListLoaded(
          notifications: [_n1, _n2],
          nextCursor: 'cursor-2',
          hasMore: false,
        ),
        // Final: next page merged.
        NotificationListLoaded(
          notifications: [_n1, _n2, _n3],
          nextCursor: 'cursor-3',
          hasMore: true,
        ),
      ],
      verify: (_) {
        verify(() => mockRepo.getNotifications(cursor: 'cursor-2')).called(1);
      },
    );

    // -----------------------------------------------------------------------
    // Next page terminated → merged + terminated state
    // -----------------------------------------------------------------------

    blocTest<NotificationListBloc, NotificationListState>(
      'NotificationListBloc_EmitsTerminated_WhenNextPageTerminates: '
      'emits Terminated with merged notifications when next page returns '
      'terminated:true',
      build: () {
        when(() => mockRepo.getNotifications(cursor: any(named: 'cursor')))
            .thenAnswer((_) async => Success(_terminatedPage([_n3])));
        return NotificationListBloc(notificationRepository: mockRepo);
      },
      seed: () => NotificationListLoaded(
        notifications: [_n1, _n2],
        nextCursor: 'cursor-2',
        hasMore: true,
      ),
      act: (bloc) => bloc.add(const NotificationListNextPageRequested()),
      expect: () => [
        NotificationListLoaded(
          notifications: [_n1, _n2],
          nextCursor: 'cursor-2',
          hasMore: false,
        ),
        NotificationListTerminated(notifications: [_n1, _n2, _n3]),
      ],
    );

    // -----------------------------------------------------------------------
    // hasMore = false guard (duplicate next-page blocked)
    // -----------------------------------------------------------------------

    blocTest<NotificationListBloc, NotificationListState>(
      'does nothing when hasMore is false (duplicate request guard)',
      build: () => NotificationListBloc(notificationRepository: mockRepo),
      seed: () => NotificationListLoaded(
        notifications: [_n1],
        nextCursor: null,
        hasMore: false,
      ),
      act: (bloc) => bloc.add(const NotificationListNextPageRequested()),
      expect: () => <NotificationListState>[],
      verify: (_) {
        verifyNever(
          () => mockRepo.getNotifications(cursor: any(named: 'cursor')),
        );
      },
    );

    // -----------------------------------------------------------------------
    // Mark all read — loaded state
    // -----------------------------------------------------------------------

    blocTest<NotificationListBloc, NotificationListState>(
      'NotificationListBloc_MarkAllRead_Loaded: '
      're-emits Loaded with all isRead=true after successful markAllRead',
      build: () {
        when(() => mockRepo.markAllRead())
            .thenAnswer((_) async => const Success(null));
        return NotificationListBloc(notificationRepository: mockRepo);
      },
      seed: () => NotificationListLoaded(
        notifications: [_n1, _n2],
        nextCursor: 'cursor-2',
        hasMore: true,
      ),
      act: (bloc) => bloc.add(const NotificationListMarkAllReadRequested()),
      expect: () => [
        NotificationListLoaded(
          notifications: [_n1.markRead(), _n2.markRead()],
          nextCursor: 'cursor-2',
          hasMore: true,
        ),
      ],
      verify: (_) {
        verify(() => mockRepo.markAllRead()).called(1);
      },
    );

    // -----------------------------------------------------------------------
    // Mark all read — terminated state
    // -----------------------------------------------------------------------

    blocTest<NotificationListBloc, NotificationListState>(
      'NotificationListBloc_MarkAllRead_Terminated: '
      're-emits Terminated with all isRead=true after successful markAllRead',
      build: () {
        when(() => mockRepo.markAllRead())
            .thenAnswer((_) async => const Success(null));
        return NotificationListBloc(notificationRepository: mockRepo);
      },
      seed: () => NotificationListTerminated(notifications: [_n1, _n2]),
      act: (bloc) => bloc.add(const NotificationListMarkAllReadRequested()),
      expect: () => [
        NotificationListTerminated(
          notifications: [_n1.markRead(), _n2.markRead()],
        ),
      ],
      verify: (_) {
        verify(() => mockRepo.markAllRead()).called(1);
      },
    );

    // -----------------------------------------------------------------------
    // Mark all read — failure
    // -----------------------------------------------------------------------

    blocTest<NotificationListBloc, NotificationListState>(
      'NotificationListBloc_MarkAllRead_EmitsError_OnFailure: '
      'emits Error when markAllRead returns a failure',
      build: () {
        when(() => mockRepo.markAllRead())
            .thenAnswer((_) async => const Err(ServerFailure('server error')));
        return NotificationListBloc(notificationRepository: mockRepo);
      },
      seed: () => NotificationListLoaded(
        notifications: [_n1],
        nextCursor: null,
        hasMore: false,
      ),
      act: (bloc) => bloc.add(const NotificationListMarkAllReadRequested()),
      expect: () => [
        isA<NotificationListError>().having(
          (s) => s.failure,
          'failure',
          isA<ServerFailure>(),
        ),
      ],
    );

    // -----------------------------------------------------------------------
    // Mark all read — ignored when not in loaded/terminated state
    // -----------------------------------------------------------------------

    blocTest<NotificationListBloc, NotificationListState>(
      'NotificationListBloc_MarkAllRead_IgnoredWhenLoading: '
      'MarkAllReadRequested is ignored when state is NotificationListLoading',
      build: () => NotificationListBloc(notificationRepository: mockRepo),
      seed: () => const NotificationListLoading(),
      act: (bloc) => bloc.add(const NotificationListMarkAllReadRequested()),
      expect: () => <NotificationListState>[],
      verify: (_) {
        verifyNever(() => mockRepo.markAllRead());
      },
    );
  });
}
