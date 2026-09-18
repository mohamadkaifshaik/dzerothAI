import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:dzeroth/core/error/failures.dart';
import 'package:dzeroth/core/error/result.dart';
import 'package:dzeroth/features/post/domain/entities/post.dart';
import 'package:dzeroth/features/post/domain/repositories/post_repository.dart';
import 'package:dzeroth/features/post/presentation/bloc/post_compose_bloc.dart';

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

final _createdPost = Post(
  id: 'post-new',
  author: _author,
  postType: 'original',
  content: 'Hello world',
  isDeleted: false,
  createdAt: DateTime.utc(2024),
  updatedAt: DateTime.utc(2024),
);

/// Generates a string of exactly [n] Unicode runes.
String _runeString(int n) => 'a' * n;

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  late MockPostRepository mockRepo;

  setUp(() {
    mockRepo = MockPostRepository();
  });

  group('PostComposeBloc', () {
    // -----------------------------------------------------------------------
    // Validation — original posts
    // -----------------------------------------------------------------------

    group('original post validation', () {
      blocTest<PostComposeBloc, PostComposeState>(
        'emits PostComposeError(ValidationFailure) when content is empty',
        build: () => PostComposeBloc(postRepository: mockRepo),
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(postType: 'original', content: ''),
        ),
        expect: () => [
          isA<PostComposeError>().having(
            (s) => s.failure,
            'failure',
            isA<ValidationFailure>(),
          ),
        ],
        verify: (_) {
          verifyNever(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
            ),
          );
        },
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'emits PostComposeError(ValidationFailure) when content is null '
        '(treated as empty for non-repost types)',
        build: () => PostComposeBloc(postRepository: mockRepo),
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(postType: 'original', content: null),
        ),
        expect: () => [
          isA<PostComposeError>().having(
            (s) => s.failure,
            'failure',
            isA<ValidationFailure>(),
          ),
        ],
        verify: (_) {
          verifyNever(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
            ),
          );
        },
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'emits PostComposeError(ValidationFailure) when content exceeds 500 code points',
        build: () => PostComposeBloc(postRepository: mockRepo),
        act: (bloc) => bloc.add(
          PostComposeSubmitted(
            postType: 'original',
            content: _runeString(501),
          ),
        ),
        expect: () => [
          isA<PostComposeError>().having(
            (s) => s.failure,
            'failure',
            isA<ValidationFailure>(),
          ),
        ],
        verify: (_) {
          verifyNever(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
            ),
          );
        },
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'accepts exactly 500 code points without emitting a validation error',
        build: () {
          when(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
            ),
          ).thenAnswer((_) async => Success(_createdPost));
          return PostComposeBloc(postRepository: mockRepo);
        },
        act: (bloc) => bloc.add(
          PostComposeSubmitted(
            postType: 'original',
            content: _runeString(500),
          ),
        ),
        expect: () => [
          const PostComposeSubmitting(),
          PostComposeSuccess(post: _createdPost),
        ],
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'emits [PostComposeSubmitting, PostComposeSuccess] with valid content',
        build: () {
          when(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
            ),
          ).thenAnswer((_) async => Success(_createdPost));
          return PostComposeBloc(postRepository: mockRepo);
        },
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(
            postType: 'original',
            content: 'Hello world',
          ),
        ),
        expect: () => [
          const PostComposeSubmitting(),
          PostComposeSuccess(post: _createdPost),
        ],
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'emits PostComposeError when repository returns a failure',
        build: () {
          when(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
            ),
          ).thenAnswer(
            (_) async => const Err(ServerFailure('post creation failed')),
          );
          return PostComposeBloc(postRepository: mockRepo);
        },
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(
            postType: 'original',
            content: 'Hello world',
          ),
        ),
        expect: () => [
          const PostComposeSubmitting(),
          isA<PostComposeError>().having(
            (s) => s.failure,
            'failure',
            isA<ServerFailure>(),
          ),
        ],
      );
    });

    // -----------------------------------------------------------------------
    // Validation — quote posts  (CLAUDE.md §2.2 absolute rule)
    // -----------------------------------------------------------------------

    group('quote post five-distinct-word rule (CLAUDE.md §2.2)', () {
      blocTest<PostComposeBloc, PostComposeState>(
        'emits PostComposeError(ValidationFailure) for quote post with 0 words',
        build: () => PostComposeBloc(postRepository: mockRepo),
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(
            postType: 'quote',
            content: '   ',
            quotedPostId: 'post-abc',
          ),
        ),
        expect: () => [
          isA<PostComposeError>().having(
            (s) => s.failure,
            'failure',
            isA<ValidationFailure>(),
          ),
        ],
        verify: (_) {
          verifyNever(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
            ),
          );
        },
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'emits PostComposeError(ValidationFailure) for quote post with 4 distinct words',
        build: () => PostComposeBloc(postRepository: mockRepo),
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(
            postType: 'quote',
            // "one two three four" — 4 distinct words, below threshold.
            content: 'one two three four',
            quotedPostId: 'post-abc',
          ),
        ),
        expect: () => [
          isA<PostComposeError>().having(
            (s) => s.failure,
            'failure',
            isA<ValidationFailure>(),
          ),
        ],
        verify: (_) {
          verifyNever(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
            ),
          );
        },
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'emits PostComposeError(ValidationFailure) for quote post with '
        'repeated words that reduce distinct count below 5',
        build: () => PostComposeBloc(postRepository: mockRepo),
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(
            postType: 'quote',
            // "hello hello hello hello world" — only 2 distinct words.
            content: 'hello hello hello hello world',
            quotedPostId: 'post-abc',
          ),
        ),
        expect: () => [
          isA<PostComposeError>().having(
            (s) => s.failure,
            'failure',
            isA<ValidationFailure>(),
          ),
        ],
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'accepts quote post with exactly 5 distinct words',
        build: () {
          when(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
            ),
          ).thenAnswer((_) async => Success(_createdPost));
          return PostComposeBloc(postRepository: mockRepo);
        },
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(
            postType: 'quote',
            // Exactly 5 distinct words — must pass the threshold.
            content: 'alpha beta gamma delta epsilon',
            quotedPostId: 'post-abc',
          ),
        ),
        expect: () => [
          const PostComposeSubmitting(),
          PostComposeSuccess(post: _createdPost),
        ],
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'accepts quote post with more than 5 distinct words',
        build: () {
          when(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
            ),
          ).thenAnswer((_) async => Success(_createdPost));
          return PostComposeBloc(postRepository: mockRepo);
        },
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(
            postType: 'quote',
            content: 'This is a great quoted post indeed',
            quotedPostId: 'post-abc',
          ),
        ),
        expect: () => [
          const PostComposeSubmitting(),
          PostComposeSuccess(post: _createdPost),
        ],
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'normalizes punctuation when counting distinct words for quote posts',
        build: () {
          when(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
            ),
          ).thenAnswer((_) async => Success(_createdPost));
          return PostComposeBloc(postRepository: mockRepo);
        },
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(
            postType: 'quote',
            // Punctuation is stripped; "alpha", "beta", "gamma", "delta",
            // "epsilon" are 5 distinct normalized tokens.
            content: 'alpha, beta! gamma. delta; epsilon?',
            quotedPostId: 'post-abc',
          ),
        ),
        expect: () => [
          const PostComposeSubmitting(),
          PostComposeSuccess(post: _createdPost),
        ],
      );
    });

    // -----------------------------------------------------------------------
    // Repost — no content validation
    // -----------------------------------------------------------------------

    group('repost type', () {
      blocTest<PostComposeBloc, PostComposeState>(
        'skips content validation for repost type and calls repository',
        build: () {
          when(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
            ),
          ).thenAnswer((_) async => Success(_createdPost));
          return PostComposeBloc(postRepository: mockRepo);
        },
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(
            postType: 'repost',
            content: null,
            quotedPostId: 'post-xyz',
          ),
        ),
        expect: () => [
          const PostComposeSubmitting(),
          PostComposeSuccess(post: _createdPost),
        ],
      );
    });

    // -----------------------------------------------------------------------
    // Reply post — inherits content validation, no word-count rule
    // -----------------------------------------------------------------------

    group('reply post', () {
      blocTest<PostComposeBloc, PostComposeState>(
        'emits PostComposeError for empty reply content',
        build: () => PostComposeBloc(postRepository: mockRepo),
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(
            postType: 'reply',
            content: '',
            parentId: 'post-parent',
          ),
        ),
        expect: () => [
          isA<PostComposeError>().having(
            (s) => s.failure,
            'failure',
            isA<ValidationFailure>(),
          ),
        ],
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'accepts reply with fewer than 5 words (word-count rule is quote-only)',
        build: () {
          when(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
            ),
          ).thenAnswer((_) async => Success(_createdPost));
          return PostComposeBloc(postRepository: mockRepo);
        },
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(
            postType: 'reply',
            content: 'ok',
            parentId: 'post-parent',
          ),
        ),
        expect: () => [
          const PostComposeSubmitting(),
          PostComposeSuccess(post: _createdPost),
        ],
      );
    });
  });
}
