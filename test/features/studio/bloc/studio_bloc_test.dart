import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:dzeroth/core/error/failures.dart';
import 'package:dzeroth/core/error/result.dart';
import 'package:dzeroth/features/studio/domain/entities/analytics_page.dart';
import 'package:dzeroth/features/studio/domain/entities/post_analytics.dart';
import 'package:dzeroth/features/studio/domain/repositories/studio_repository.dart';
import 'package:dzeroth/features/studio/presentation/bloc/studio_bloc.dart';

// ---------------------------------------------------------------------------
// Mock
// ---------------------------------------------------------------------------

class MockStudioRepository extends Mock implements StudioRepository {}

// ---------------------------------------------------------------------------
// Fixture data
// ---------------------------------------------------------------------------

PostAnalytics _fixture(String id) => PostAnalytics(
  postId: id,
  content: 'Test post content $id',
  postType: 'original',
  createdAt: DateTime(2026, 9, 18),
  reactionCount: 1,
  bookmarkCount: 2,
  replyCount: 3,
  quoteCount: 4,
);

final _item1 = _fixture('post-1');
final _item2 = _fixture('post-2');
final _item3 = _fixture('post-3');

// A single-page response, not terminated.
final _pageLoaded = AnalyticsPage(
  items: [_item1, _item2],
  nextCursor: 'cursor-abc',
  terminated: false,
);

// A second page that completes the list without termination.
final _pageSecond = AnalyticsPage(
  items: [_item3],
  nextCursor: null,
  terminated: false,
);

// A first page that is immediately terminated.
final _pageTerminated = AnalyticsPage(
  items: [_item1],
  nextCursor: null,
  terminated: true,
);

// An empty terminated page (no posts at all).
const _pageEmptyTerminated = AnalyticsPage(
  items: [],
  nextCursor: null,
  terminated: true,
);

// An empty non-terminated page.
const _pageEmpty = AnalyticsPage(
  items: [],
  nextCursor: null,
  terminated: false,
);

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  late MockStudioRepository mockRepo;

  setUp(() {
    mockRepo = MockStudioRepository();
  });

  // TestStudioBloc_FetchLoaded
  group('TestStudioBloc_FetchLoaded', () {
    blocTest<StudioBloc, StudioState>(
      'emits [StudioLoading, StudioLoaded] when first page succeeds with items',
      build: () {
        when(() => mockRepo.getAnalytics(cursor: null))
            .thenAnswer((_) async => Success(_pageLoaded));
        return StudioBloc(studioRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const StudioFetchRequested()),
      expect: () => [
        const StudioLoading(),
        StudioLoaded(
          items: [_item1, _item2],
          nextCursor: 'cursor-abc',
          hasMore: true,
        ),
      ],
    );
  });

  // TestStudioBloc_FetchEmpty
  group('TestStudioBloc_FetchEmpty', () {
    blocTest<StudioBloc, StudioState>(
      'emits [StudioLoading, StudioEmpty] when first page is empty and terminated',
      build: () {
        when(() => mockRepo.getAnalytics(cursor: null))
            .thenAnswer((_) async => const Success(_pageEmptyTerminated));
        return StudioBloc(studioRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const StudioFetchRequested()),
      expect: () => [const StudioLoading(), const StudioEmpty()],
    );

    blocTest<StudioBloc, StudioState>(
      'emits [StudioLoading, StudioLoaded(hasMore:false)] when empty page not terminated',
      build: () {
        when(() => mockRepo.getAnalytics(cursor: null))
            .thenAnswer((_) async => const Success(_pageEmpty));
        return StudioBloc(studioRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const StudioFetchRequested()),
      expect: () => [
        const StudioLoading(),
        const StudioLoaded(items: [], nextCursor: null, hasMore: false),
      ],
    );
  });

  // TestStudioBloc_FetchError
  group('TestStudioBloc_FetchError', () {
    blocTest<StudioBloc, StudioState>(
      'emits [StudioLoading, StudioError] when repository returns Err',
      build: () {
        when(
          () => mockRepo.getAnalytics(cursor: null),
        ).thenAnswer((_) async => const Err(ServerFailure('Server exploded.')));
        return StudioBloc(studioRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const StudioFetchRequested()),
      expect: () => [
        const StudioLoading(),
        isA<StudioError>().having(
          (s) => s.failure,
          'failure',
          isA<ServerFailure>(),
        ),
      ],
    );

    blocTest<StudioBloc, StudioState>(
      'emits [StudioLoading, StudioError(NetworkFailure)] on network error',
      build: () {
        when(() => mockRepo.getAnalytics(cursor: null))
            .thenAnswer((_) async => const Err(NetworkFailure()));
        return StudioBloc(studioRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const StudioFetchRequested()),
      expect: () => [
        const StudioLoading(),
        isA<StudioError>().having(
          (s) => s.failure,
          'failure',
          isA<NetworkFailure>(),
        ),
      ],
    );
  });

  // TestStudioBloc_TerminatedState_DropsFurtherRequests
  group('TestStudioBloc_TerminatedState_DropsFurtherRequests', () {
    blocTest<StudioBloc, StudioState>(
      'silently drops StudioNextPageRequested when in StudioTerminated state',
      build: () {
        when(() => mockRepo.getAnalytics(cursor: null))
            .thenAnswer((_) async => Success(_pageTerminated));
        return StudioBloc(studioRepository: mockRepo);
      },
      act: (bloc) async {
        bloc.add(const StudioFetchRequested());
        // Allow the fetch to complete.
        await Future<void>.delayed(Duration.zero);
        // This next-page request must be silently dropped.
        bloc.add(const StudioNextPageRequested());
        await Future<void>.delayed(Duration.zero);
      },
      expect: () => [
        const StudioLoading(),
        StudioTerminated(items: [_item1]),
        // No additional states — the next-page event was dropped.
      ],
      verify: (_) {
        // Repository called exactly once (for the initial fetch only).
        // The silently-dropped StudioNextPageRequested must not trigger a second call.
        verify(() => mockRepo.getAnalytics(cursor: null)).called(1);
      },
    );
  });

  // TestStudioBloc_NextPage_AppendsItems
  group('TestStudioBloc_NextPage_AppendsItems', () {
    blocTest<StudioBloc, StudioState>(
      'appends second page items to existing list',
      build: () {
        when(() => mockRepo.getAnalytics(cursor: null))
            .thenAnswer((_) async => Success(_pageLoaded));
        when(() => mockRepo.getAnalytics(cursor: 'cursor-abc'))
            .thenAnswer((_) async => Success(_pageSecond));
        return StudioBloc(studioRepository: mockRepo);
      },
      act: (bloc) async {
        bloc.add(const StudioFetchRequested());
        await Future<void>.delayed(Duration.zero);
        bloc.add(const StudioNextPageRequested());
      },
      expect: () => [
        const StudioLoading(),
        // First page loaded.
        StudioLoaded(
          items: [_item1, _item2],
          nextCursor: 'cursor-abc',
          hasMore: true,
        ),
        // hasMore locked to false while in-flight.
        StudioLoaded(
          items: [_item1, _item2],
          nextCursor: 'cursor-abc',
          hasMore: false,
        ),
        // Second page appended; nextCursor is null so hasMore = false.
        StudioLoaded(
          items: [_item1, _item2, _item3],
          nextCursor: null,
          hasMore: false,
        ),
      ],
    );

    blocTest<StudioBloc, StudioState>(
      'transitions to StudioTerminated when second page has terminated=true',
      build: () {
        final secondTerminated = AnalyticsPage(
          items: [_item3],
          nextCursor: null,
          terminated: true,
        );
        when(() => mockRepo.getAnalytics(cursor: null))
            .thenAnswer((_) async => Success(_pageLoaded));
        when(() => mockRepo.getAnalytics(cursor: 'cursor-abc'))
            .thenAnswer((_) async => Success(secondTerminated));
        return StudioBloc(studioRepository: mockRepo);
      },
      act: (bloc) async {
        bloc.add(const StudioFetchRequested());
        await Future<void>.delayed(Duration.zero);
        bloc.add(const StudioNextPageRequested());
      },
      expect: () => [
        const StudioLoading(),
        StudioLoaded(
          items: [_item1, _item2],
          nextCursor: 'cursor-abc',
          hasMore: true,
        ),
        StudioLoaded(
          items: [_item1, _item2],
          nextCursor: 'cursor-abc',
          hasMore: false,
        ),
        StudioTerminated(items: [_item1, _item2, _item3]),
      ],
    );

    blocTest<StudioBloc, StudioState>(
      'ignores StudioNextPageRequested when hasMore is false',
      build: () {
        when(() => mockRepo.getAnalytics(cursor: null))
            .thenAnswer((_) async => const Success(_pageEmpty));
        return StudioBloc(studioRepository: mockRepo);
      },
      act: (bloc) async {
        bloc.add(const StudioFetchRequested());
        await Future<void>.delayed(Duration.zero);
        // hasMore is false — this should be ignored.
        bloc.add(const StudioNextPageRequested());
        await Future<void>.delayed(Duration.zero);
      },
      expect: () => [
        const StudioLoading(),
        const StudioLoaded(items: [], nextCursor: null, hasMore: false),
        // No further states — request dropped.
      ],
    );
  });
}
