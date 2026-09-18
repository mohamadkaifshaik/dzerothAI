import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:dzeroth/core/error/failures.dart';
import 'package:dzeroth/core/error/result.dart';
import 'package:dzeroth/features/feed/domain/repositories/feed_repository.dart';
import 'package:dzeroth/features/feed/presentation/bloc/home_feed_bloc.dart';
import 'package:dzeroth/features/post/domain/entities/post.dart';

// ---------------------------------------------------------------------------
// Mock
// ---------------------------------------------------------------------------

class MockFeedRepository extends Mock implements FeedRepository {}

// ---------------------------------------------------------------------------
// Fixture data
// ---------------------------------------------------------------------------

const _author = PostAuthor(
  id: '01900000-0000-7000-8000-000000000001',
  handle: 'alice',
  displayName: 'Alice',
);

Post _makePost(String id) => Post(
  id: id,
  author: _author,
  postType: 'original',
  content: 'Post $id',
  isDeleted: false,
  createdAt: DateTime.utc(2024),
  updatedAt: DateTime.utc(2024),
);

final _post1 = _makePost('post-1');
final _post2 = _makePost('post-2');
final _post3 = _makePost('post-3');

/// A page that has more content after it.
PostPage _pageWithMore(List<Post> items, {String cursor = 'cursor-2'}) =>
    PostPage(items: items, nextCursor: cursor, terminated: false);

/// A page that signals the server-enforced feed boundary.
PostPage _terminatedPage(List<Post> items) =>
    PostPage(items: items, nextCursor: null, terminated: true);

/// An immediately-terminated page with no items (empty follow list).
const _emptyTerminatedPage = PostPage(
  items: [],
  nextCursor: null,
  terminated: true,
);

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  late MockFeedRepository mockRepo;

  setUp(() {
    mockRepo = MockFeedRepository();
  });

  group('HomeFeedBloc', () {
    // -----------------------------------------------------------------------
    // HomeFeedLoadRequested — non-terminated load
    // -----------------------------------------------------------------------

    blocTest<HomeFeedBloc, HomeFeedState>(
      'HomeFeedBloc_EmitsLoading_ThenLoaded_OnLoadRequested: '
      'emits [HomeFeedLoading, HomeFeedLoaded] on successful non-terminated load',
      build: () {
        when(() => mockRepo.getHomeFeed(cursor: any(named: 'cursor')))
            .thenAnswer((_) async => Success(_pageWithMore([_post1, _post2])));
        return HomeFeedBloc(feedRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const HomeFeedLoadRequested()),
      expect: () => [
        const HomeFeedLoading(),
        HomeFeedLoaded(
          posts: [_post1, _post2],
          nextCursor: 'cursor-2',
          hasMore: true,
        ),
      ],
      verify: (_) {
        verify(() => mockRepo.getHomeFeed(cursor: null)).called(1);
      },
    );

    // -----------------------------------------------------------------------
    // HomeFeedLoadRequested — terminated immediately
    // -----------------------------------------------------------------------

    blocTest<HomeFeedBloc, HomeFeedState>(
      'HomeFeedBloc_EmitsTerminated_WhenTerminatedFlagTrue: '
      'emits [HomeFeedLoading, HomeFeedTerminated] when repository returns '
      'terminated:true with items',
      build: () {
        when(() => mockRepo.getHomeFeed(cursor: any(named: 'cursor')))
            .thenAnswer((_) async => Success(_terminatedPage([_post1])));
        return HomeFeedBloc(feedRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const HomeFeedLoadRequested()),
      expect: () => [
        const HomeFeedLoading(),
        HomeFeedTerminated(posts: [_post1]),
      ],
      verify: (_) {
        verify(() => mockRepo.getHomeFeed(cursor: null)).called(1);
      },
    );

    // -----------------------------------------------------------------------
    // HomeFeedTerminated is a terminal state — next page is ignored
    // -----------------------------------------------------------------------

    blocTest<HomeFeedBloc, HomeFeedState>(
      'HomeFeedBloc_IgnoresNextPage_AfterTerminated: '
      'HomeFeedNextPageRequested after HomeFeedTerminated emits nothing and '
      'does not call repository',
      build: () => HomeFeedBloc(feedRepository: mockRepo),
      seed: () => HomeFeedTerminated(posts: [_post1]),
      act: (bloc) => bloc.add(const HomeFeedNextPageRequested()),
      // Terminal state: no emissions.
      expect: () => <HomeFeedState>[],
      verify: (_) {
        verifyNever(() => mockRepo.getHomeFeed(cursor: any(named: 'cursor')));
      },
    );

    // -----------------------------------------------------------------------
    // Empty follow list → HomeFeedEmpty
    // -----------------------------------------------------------------------

    blocTest<HomeFeedBloc, HomeFeedState>(
      'HomeFeedBloc_EmitsEmpty_WhenNoFollows: '
      'emits [HomeFeedLoading, HomeFeedEmpty] when repository returns '
      'terminated:true with empty items list',
      build: () {
        when(() => mockRepo.getHomeFeed(cursor: any(named: 'cursor')))
            .thenAnswer((_) async => const Success(_emptyTerminatedPage));
        return HomeFeedBloc(feedRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const HomeFeedLoadRequested()),
      expect: () => [const HomeFeedLoading(), const HomeFeedEmpty()],
      verify: (_) {
        verify(() => mockRepo.getHomeFeed(cursor: null)).called(1);
      },
    );

    // -----------------------------------------------------------------------
    // Next page appends posts
    // -----------------------------------------------------------------------

    blocTest<HomeFeedBloc, HomeFeedState>(
      'HomeFeedBloc_LoadsNextPage: '
      'fetches next page and appends posts when hasMore is true',
      build: () {
        when(() => mockRepo.getHomeFeed(cursor: any(named: 'cursor')))
            .thenAnswer(
              (_) async => Success(_pageWithMore([_post3], cursor: 'cursor-3')),
            );
        return HomeFeedBloc(feedRepository: mockRepo);
      },
      seed: () => HomeFeedLoaded(
        posts: [_post1, _post2],
        nextCursor: 'cursor-2',
        hasMore: true,
      ),
      act: (bloc) => bloc.add(const HomeFeedNextPageRequested()),
      expect: () => [
        // In-flight: hasMore locked to false.
        HomeFeedLoaded(
          posts: [_post1, _post2],
          nextCursor: 'cursor-2',
          hasMore: false,
        ),
        // Final: next page merged.
        HomeFeedLoaded(
          posts: [_post1, _post2, _post3],
          nextCursor: 'cursor-3',
          hasMore: true,
        ),
      ],
      verify: (_) {
        verify(() => mockRepo.getHomeFeed(cursor: 'cursor-2')).called(1);
      },
    );

    // -----------------------------------------------------------------------
    // Repository error → HomeFeedError
    // -----------------------------------------------------------------------

    blocTest<HomeFeedBloc, HomeFeedState>(
      'HomeFeedBloc_EmitsError_OnFailure: '
      'emits [HomeFeedLoading, HomeFeedError] when repository returns failure',
      build: () {
        when(() => mockRepo.getHomeFeed(cursor: any(named: 'cursor')))
            .thenAnswer((_) async => const Err(NetworkFailure('unreachable')));
        return HomeFeedBloc(feedRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const HomeFeedLoadRequested()),
      expect: () => [
        const HomeFeedLoading(),
        isA<HomeFeedError>().having(
          (s) => s.failure,
          'failure',
          isA<NetworkFailure>(),
        ),
      ],
    );

    // -----------------------------------------------------------------------
    // Next page terminated → HomeFeedTerminated with merged posts
    // -----------------------------------------------------------------------

    blocTest<HomeFeedBloc, HomeFeedState>(
      'emits HomeFeedTerminated with merged posts when next page returns '
      'terminated:true',
      build: () {
        when(() => mockRepo.getHomeFeed(cursor: any(named: 'cursor')))
            .thenAnswer((_) async => Success(_terminatedPage([_post3])));
        return HomeFeedBloc(feedRepository: mockRepo);
      },
      seed: () => HomeFeedLoaded(
        posts: [_post1, _post2],
        nextCursor: 'cursor-2',
        hasMore: true,
      ),
      act: (bloc) => bloc.add(const HomeFeedNextPageRequested()),
      expect: () => [
        HomeFeedLoaded(
          posts: [_post1, _post2],
          nextCursor: 'cursor-2',
          hasMore: false,
        ),
        HomeFeedTerminated(posts: [_post1, _post2, _post3]),
      ],
    );

    // -----------------------------------------------------------------------
    // Next page error → restore loaded state + emit error
    // -----------------------------------------------------------------------

    blocTest<HomeFeedBloc, HomeFeedState>(
      'restores HomeFeedLoaded and emits HomeFeedError when next-page fetch '
      'fails',
      build: () {
        when(
          () => mockRepo.getHomeFeed(cursor: any(named: 'cursor')),
        ).thenAnswer((_) async => const Err(ServerFailure('internal error')));
        return HomeFeedBloc(feedRepository: mockRepo);
      },
      seed: () => HomeFeedLoaded(
        posts: [_post1, _post2],
        nextCursor: 'cursor-2',
        hasMore: true,
      ),
      act: (bloc) => bloc.add(const HomeFeedNextPageRequested()),
      expect: () => [
        // In-flight.
        HomeFeedLoaded(
          posts: [_post1, _post2],
          nextCursor: 'cursor-2',
          hasMore: false,
        ),
        // Restored so user can retry.
        HomeFeedLoaded(
          posts: [_post1, _post2],
          nextCursor: 'cursor-2',
          hasMore: true,
        ),
        isA<HomeFeedError>().having(
          (s) => s.failure,
          'failure',
          isA<ServerFailure>(),
        ),
      ],
    );

    // -----------------------------------------------------------------------
    // hasMore = false guard (no duplicate requests)
    // -----------------------------------------------------------------------

    blocTest<HomeFeedBloc, HomeFeedState>(
      'does nothing when hasMore is false (duplicate request guard)',
      build: () => HomeFeedBloc(feedRepository: mockRepo),
      seed: () =>
          HomeFeedLoaded(posts: [_post1], nextCursor: null, hasMore: false),
      act: (bloc) => bloc.add(const HomeFeedNextPageRequested()),
      expect: () => <HomeFeedState>[],
      verify: (_) {
        verifyNever(() => mockRepo.getHomeFeed(cursor: any(named: 'cursor')));
      },
    );
  });
}
