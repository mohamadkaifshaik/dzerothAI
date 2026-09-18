import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:dzeroth/core/error/failures.dart';
import 'package:dzeroth/core/error/result.dart';
import 'package:dzeroth/features/post/domain/entities/post.dart';
import 'package:dzeroth/features/post/domain/repositories/post_repository.dart';
import 'package:dzeroth/features/post/presentation/bloc/post_feed_bloc.dart';

// ---------------------------------------------------------------------------
// Mock
// ---------------------------------------------------------------------------

class MockPostRepository extends Mock implements PostRepository {}

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

const _subjectId = 'user-abc';

// A page that has more content after it.
PostPage _pageWithMore(List<Post> items, {String cursor = 'cursor-2'}) =>
    PostPage(items: items, nextCursor: cursor, terminated: false);

// A page that signals the server-enforced feed boundary.
PostPage _terminatedPage(List<Post> items) =>
    PostPage(items: items, nextCursor: null, terminated: true);

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  late MockPostRepository mockRepo;

  setUp(() {
    mockRepo = MockPostRepository();
  });

  group('PostFeedBloc', () {
    // -----------------------------------------------------------------------
    // PostFeedLoadRequested
    // -----------------------------------------------------------------------

    group('PostFeedLoadRequested', () {
      blocTest<PostFeedBloc, PostFeedState>(
        'emits [PostFeedLoading, PostFeedLoaded] on successful non-terminated load',
        build: () {
          when(
            () => mockRepo.listPostsByAuthor(
              any(),
              cursor: any(named: 'cursor'),
            ),
          ).thenAnswer(
            (_) async => Success(_pageWithMore([_post1, _post2])),
          );
          return PostFeedBloc(postRepository: mockRepo);
        },
        act: (bloc) => bloc.add(
          const PostFeedLoadRequested(
            subjectId: _subjectId,
            feedType: FeedType.authorPosts,
          ),
        ),
        expect: () => [
          const PostFeedLoading(),
          PostFeedLoaded(
            posts: [_post1, _post2],
            nextCursor: 'cursor-2',
            hasMore: true,
            feedType: FeedType.authorPosts,
            subjectId: _subjectId,
          ),
        ],
        verify: (_) {
          verify(
            () => mockRepo.listPostsByAuthor(_subjectId, cursor: null),
          ).called(1);
        },
      );

      blocTest<PostFeedBloc, PostFeedState>(
        'emits [PostFeedLoading, PostFeedTerminated] when repository returns terminated:true',
        build: () {
          when(
            () => mockRepo.listPostsByAuthor(
              any(),
              cursor: any(named: 'cursor'),
            ),
          ).thenAnswer(
            (_) async => Success(_terminatedPage([_post1])),
          );
          return PostFeedBloc(postRepository: mockRepo);
        },
        act: (bloc) => bloc.add(
          const PostFeedLoadRequested(
            subjectId: _subjectId,
            feedType: FeedType.authorPosts,
          ),
        ),
        expect: () => [
          const PostFeedLoading(),
          PostFeedTerminated(posts: [_post1]),
        ],
        verify: (_) {
          verify(
            () => mockRepo.listPostsByAuthor(_subjectId, cursor: null),
          ).called(1);
        },
      );

      blocTest<PostFeedBloc, PostFeedState>(
        'emits [PostFeedLoading, PostFeedError] when repository fails',
        build: () {
          when(
            () => mockRepo.listPostsByAuthor(
              any(),
              cursor: any(named: 'cursor'),
            ),
          ).thenAnswer(
            (_) async => const Err(NetworkFailure('unreachable')),
          );
          return PostFeedBloc(postRepository: mockRepo);
        },
        act: (bloc) => bloc.add(
          const PostFeedLoadRequested(
            subjectId: _subjectId,
            feedType: FeedType.authorPosts,
          ),
        ),
        expect: () => [
          const PostFeedLoading(),
          isA<PostFeedError>().having(
            (s) => s.failure,
            'failure',
            isA<NetworkFailure>(),
          ),
        ],
      );

      blocTest<PostFeedBloc, PostFeedState>(
        'uses listThreadReplies when feedType is threadReplies',
        build: () {
          when(
            () => mockRepo.listThreadReplies(
              any(),
              cursor: any(named: 'cursor'),
            ),
          ).thenAnswer(
            (_) async => Success(_pageWithMore([_post2])),
          );
          return PostFeedBloc(postRepository: mockRepo);
        },
        act: (bloc) => bloc.add(
          const PostFeedLoadRequested(
            subjectId: 'thread-root-id',
            feedType: FeedType.threadReplies,
          ),
        ),
        expect: () => [
          const PostFeedLoading(),
          PostFeedLoaded(
            posts: [_post2],
            nextCursor: 'cursor-2',
            hasMore: true,
            feedType: FeedType.threadReplies,
            subjectId: 'thread-root-id',
          ),
        ],
        verify: (_) {
          verify(
            () => mockRepo.listThreadReplies('thread-root-id', cursor: null),
          ).called(1);
          verifyNever(
            () => mockRepo.listPostsByAuthor(any(), cursor: any(named: 'cursor')),
          );
        },
      );
    });

    // -----------------------------------------------------------------------
    // PostFeedNextPageRequested
    // -----------------------------------------------------------------------

    group('PostFeedNextPageRequested', () {
      blocTest<PostFeedBloc, PostFeedState>(
        'ignores PostFeedNextPageRequested in PostFeedTerminated state — '
        'repository must not be called again',
        build: () => PostFeedBloc(postRepository: mockRepo),
        // Seed directly into the terminated terminal state.
        seed: () => PostFeedTerminated(posts: [_post1]),
        act: (bloc) => bloc.add(const PostFeedNextPageRequested()),
        // No state changes expected; terminal state is preserved unchanged.
        expect: () => <PostFeedState>[],
        verify: (_) {
          verifyNever(
            () => mockRepo.listPostsByAuthor(any(), cursor: any(named: 'cursor')),
          );
          verifyNever(
            () => mockRepo.listThreadReplies(any(), cursor: any(named: 'cursor')),
          );
        },
      );

      blocTest<PostFeedBloc, PostFeedState>(
        'fetches next page and appends posts when hasMore is true',
        build: () {
          when(
            () => mockRepo.listPostsByAuthor(
              any(),
              cursor: any(named: 'cursor'),
            ),
          ).thenAnswer(
            (_) async => Success(_pageWithMore([_post3], cursor: 'cursor-3')),
          );
          return PostFeedBloc(postRepository: mockRepo);
        },
        seed: () => PostFeedLoaded(
          posts: [_post1, _post2],
          nextCursor: 'cursor-2',
          hasMore: true,
          feedType: FeedType.authorPosts,
          subjectId: _subjectId,
        ),
        act: (bloc) => bloc.add(const PostFeedNextPageRequested()),
        expect: () => [
          // In-flight state: hasMore set to false to block duplicate requests.
          PostFeedLoaded(
            posts: [_post1, _post2],
            nextCursor: 'cursor-2',
            hasMore: false,
            feedType: FeedType.authorPosts,
            subjectId: _subjectId,
          ),
          // Final state: new page merged.
          PostFeedLoaded(
            posts: [_post1, _post2, _post3],
            nextCursor: 'cursor-3',
            hasMore: true,
            feedType: FeedType.authorPosts,
            subjectId: _subjectId,
          ),
        ],
        verify: (_) {
          verify(
            () => mockRepo.listPostsByAuthor(
              _subjectId,
              cursor: 'cursor-2',
            ),
          ).called(1);
        },
      );

      blocTest<PostFeedBloc, PostFeedState>(
        'emits PostFeedTerminated (with merged posts) when next page is the last',
        build: () {
          when(
            () => mockRepo.listPostsByAuthor(
              any(),
              cursor: any(named: 'cursor'),
            ),
          ).thenAnswer(
            (_) async => Success(_terminatedPage([_post3])),
          );
          return PostFeedBloc(postRepository: mockRepo);
        },
        seed: () => PostFeedLoaded(
          posts: [_post1, _post2],
          nextCursor: 'cursor-2',
          hasMore: true,
          feedType: FeedType.authorPosts,
          subjectId: _subjectId,
        ),
        act: (bloc) => bloc.add(const PostFeedNextPageRequested()),
        expect: () => [
          PostFeedLoaded(
            posts: [_post1, _post2],
            nextCursor: 'cursor-2',
            hasMore: false,
            feedType: FeedType.authorPosts,
            subjectId: _subjectId,
          ),
          PostFeedTerminated(posts: [_post1, _post2, _post3]),
        ],
      );

      blocTest<PostFeedBloc, PostFeedState>(
        'does nothing when hasMore is false (next-page guard)',
        build: () => PostFeedBloc(postRepository: mockRepo),
        seed: () => PostFeedLoaded(
          posts: [_post1],
          nextCursor: null,
          hasMore: false,
          feedType: FeedType.authorPosts,
          subjectId: _subjectId,
        ),
        act: (bloc) => bloc.add(const PostFeedNextPageRequested()),
        expect: () => <PostFeedState>[],
        verify: (_) {
          verifyNever(
            () => mockRepo.listPostsByAuthor(any(), cursor: any(named: 'cursor')),
          );
        },
      );

      blocTest<PostFeedBloc, PostFeedState>(
        'restores loaded state and emits PostFeedError when next-page fetch fails',
        build: () {
          when(
            () => mockRepo.listPostsByAuthor(
              any(),
              cursor: any(named: 'cursor'),
            ),
          ).thenAnswer(
            (_) async => const Err(ServerFailure('internal error')),
          );
          return PostFeedBloc(postRepository: mockRepo);
        },
        seed: () => PostFeedLoaded(
          posts: [_post1, _post2],
          nextCursor: 'cursor-2',
          hasMore: true,
          feedType: FeedType.authorPosts,
          subjectId: _subjectId,
        ),
        act: (bloc) => bloc.add(const PostFeedNextPageRequested()),
        expect: () => [
          // In-flight.
          PostFeedLoaded(
            posts: [_post1, _post2],
            nextCursor: 'cursor-2',
            hasMore: false,
            feedType: FeedType.authorPosts,
            subjectId: _subjectId,
          ),
          // Restore so user can retry.
          PostFeedLoaded(
            posts: [_post1, _post2],
            nextCursor: 'cursor-2',
            hasMore: true,
            feedType: FeedType.authorPosts,
            subjectId: _subjectId,
          ),
          isA<PostFeedError>().having(
            (s) => s.failure,
            'failure',
            isA<ServerFailure>(),
          ),
        ],
      );
    });
  });
}
