import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:dzeroth/core/error/failures.dart';
import 'package:dzeroth/core/error/result.dart';
import 'package:dzeroth/features/hashtag/domain/repositories/hashtag_repository.dart';
import 'package:dzeroth/features/hashtag/presentation/bloc/hashtag_feed_bloc.dart';
import 'package:dzeroth/features/post/domain/entities/post.dart';

// ---------------------------------------------------------------------------
// Mock
// ---------------------------------------------------------------------------

class MockHashtagRepository extends Mock implements HashtagRepository {}

// ---------------------------------------------------------------------------
// Fixture helpers
// ---------------------------------------------------------------------------

Post _post(String id) => Post(
  id: id,
  author: const PostAuthor(
    id: 'author-1',
    handle: 'alice',
    displayName: 'Alice',
  ),
  postType: 'original',
  content: 'Hello #golang',
  isDeleted: false,
  createdAt: DateTime(2026, 9, 1),
  updatedAt: DateTime(2026, 9, 1),
);

final _post1 = _post('post-1');
final _post2 = _post('post-2');
final _post3 = _post('post-3');

final _pageLoaded = PostPage(
  items: [_post1, _post2],
  nextCursor: 'cursor-abc',
  terminated: false,
);

final _pageSecond = PostPage(
  items: [_post3],
  nextCursor: null,
  terminated: false,
);

final _pageTerminatedFirst = PostPage(
  items: [_post1],
  nextCursor: null,
  terminated: true,
);

const _pageEmpty = PostPage(items: [], nextCursor: null, terminated: true);

// ---------------------------------------------------------------------------
// Helper — build bloc under test
// ---------------------------------------------------------------------------

HashtagFeedBloc _bloc(MockHashtagRepository repo) =>
    HashtagFeedBloc(hashtagRepository: repo, tag: 'golang');

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  group('HashtagFeedBloc', () {
    late MockHashtagRepository repo;

    setUp(() {
      repo = MockHashtagRepository();
    });

    // 1. Initial load → loaded
    blocTest<HashtagFeedBloc, HashtagFeedState>(
      'emits [loading, loaded] when first page succeeds',
      build: () => _bloc(repo),
      setUp: () {
        when(() => repo.getHashtagFeed(tag: 'golang', cursor: null))
            .thenAnswer((_) async => Success(_pageLoaded));
      },
      act: (bloc) => bloc.add(const HashtagFeedLoadRequested()),
      expect: () => [
        const HashtagFeedLoading(),
        HashtagFeedLoaded(
          posts: _pageLoaded.items,
          nextCursor: 'cursor-abc',
          hasMore: true,
        ),
      ],
    );

    // 2. Initial load → immediately terminated
    blocTest<HashtagFeedBloc, HashtagFeedState>(
      'emits [loading, terminated] when first page has terminated=true',
      build: () => _bloc(repo),
      setUp: () {
        when(() => repo.getHashtagFeed(tag: 'golang', cursor: null))
            .thenAnswer((_) async => Success(_pageTerminatedFirst));
      },
      act: (bloc) => bloc.add(const HashtagFeedLoadRequested()),
      expect: () => [
        const HashtagFeedLoading(),
        HashtagFeedTerminated(posts: _pageTerminatedFirst.items),
      ],
    );

    // 3. Initial load → empty
    blocTest<HashtagFeedBloc, HashtagFeedState>(
      'emits [loading, empty] when first page has no items',
      build: () => _bloc(repo),
      setUp: () {
        when(() => repo.getHashtagFeed(tag: 'golang', cursor: null))
            .thenAnswer((_) async => const Success(_pageEmpty));
      },
      act: (bloc) => bloc.add(const HashtagFeedLoadRequested()),
      expect: () => [const HashtagFeedLoading(), const HashtagFeedEmpty()],
    );

    // 4. Initial load → error
    blocTest<HashtagFeedBloc, HashtagFeedState>(
      'emits [loading, error] when first page fails',
      build: () => _bloc(repo),
      setUp: () {
        when(() => repo.getHashtagFeed(tag: 'golang', cursor: null))
            .thenAnswer((_) async => const Err(ServerFailure()));
      },
      act: (bloc) => bloc.add(const HashtagFeedLoadRequested()),
      expect: () => [
        const HashtagFeedLoading(),
        const HashtagFeedError(failure: ServerFailure()),
      ],
    );

    // 5. Pagination — load more succeeds
    blocTest<HashtagFeedBloc, HashtagFeedState>(
      'emits merged posts when next page succeeds',
      build: () => _bloc(repo),
      seed: () => HashtagFeedLoaded(
        posts: _pageLoaded.items,
        nextCursor: 'cursor-abc',
        hasMore: true,
      ),
      setUp: () {
        when(() => repo.getHashtagFeed(tag: 'golang', cursor: 'cursor-abc'))
            .thenAnswer((_) async => Success(_pageSecond));
      },
      act: (bloc) => bloc.add(const HashtagFeedNextPageRequested()),
      expect: () => [
        // Intermediate: hasMore=false while loading
        HashtagFeedLoaded(
          posts: _pageLoaded.items,
          nextCursor: 'cursor-abc',
          hasMore: false,
        ),
        // Final: merged with second page; nextCursor=null → hasMore=false
        HashtagFeedLoaded(
          posts: [..._pageLoaded.items, ..._pageSecond.items],
          nextCursor: null,
          hasMore: false,
        ),
      ],
    );

    // 6. Pagination — next page terminates
    blocTest<HashtagFeedBloc, HashtagFeedState>(
      'emits terminated when next page has terminated=true',
      build: () => _bloc(repo),
      seed: () => HashtagFeedLoaded(
        posts: _pageLoaded.items,
        nextCursor: 'cursor-abc',
        hasMore: true,
      ),
      setUp: () {
        when(() => repo.getHashtagFeed(tag: 'golang', cursor: 'cursor-abc'))
            .thenAnswer(
              (_) async => Success(
                PostPage(items: [_post3], nextCursor: null, terminated: true),
              ),
            );
      },
      act: (bloc) => bloc.add(const HashtagFeedNextPageRequested()),
      expect: () => [
        HashtagFeedLoaded(
          posts: _pageLoaded.items,
          nextCursor: 'cursor-abc',
          hasMore: false,
        ),
        HashtagFeedTerminated(posts: [..._pageLoaded.items, _post3]),
      ],
    );

    // 7. Terminated state — next page request ignored
    blocTest<HashtagFeedBloc, HashtagFeedState>(
      'ignores NextPageRequested when already terminated',
      build: () => _bloc(repo),
      seed: () => HashtagFeedTerminated(posts: [_post1]),
      act: (bloc) => bloc.add(const HashtagFeedNextPageRequested()),
      expect: () => <HashtagFeedState>[],
      verify: (_) => verifyNever(
        () => repo.getHashtagFeed(
          tag: any(named: 'tag'),
          cursor: any(named: 'cursor'),
        ),
      ),
    );

    // 8. Pagination error — restores previous state
    blocTest<HashtagFeedBloc, HashtagFeedState>(
      'restores loaded state and emits error when next page fails',
      build: () => _bloc(repo),
      seed: () => HashtagFeedLoaded(
        posts: _pageLoaded.items,
        nextCursor: 'cursor-abc',
        hasMore: true,
      ),
      setUp: () {
        when(() => repo.getHashtagFeed(tag: 'golang', cursor: 'cursor-abc'))
            .thenAnswer((_) async => const Err(NetworkFailure()));
      },
      act: (bloc) => bloc.add(const HashtagFeedNextPageRequested()),
      expect: () => [
        // Intermediate: hasMore=false while loading
        HashtagFeedLoaded(
          posts: _pageLoaded.items,
          nextCursor: 'cursor-abc',
          hasMore: false,
        ),
        // Restored: hasMore=true again
        HashtagFeedLoaded(
          posts: _pageLoaded.items,
          nextCursor: 'cursor-abc',
          hasMore: true,
        ),
        const HashtagFeedError(failure: NetworkFailure()),
      ],
    );
  });
}
