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
              shareInitiatedAt: any(named: 'shareInitiatedAt'),
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
              shareInitiatedAt: any(named: 'shareInitiatedAt'),
            ),
          );
        },
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'emits PostComposeError(ValidationFailure) when content exceeds 500 code points',
        build: () => PostComposeBloc(postRepository: mockRepo),
        act: (bloc) => bloc.add(
          PostComposeSubmitted(postType: 'original', content: _runeString(501)),
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
              shareInitiatedAt: any(named: 'shareInitiatedAt'),
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
              shareInitiatedAt: any(named: 'shareInitiatedAt'),
            ),
          ).thenAnswer((_) async => Success(_createdPost));
          return PostComposeBloc(postRepository: mockRepo);
        },
        act: (bloc) => bloc.add(
          PostComposeSubmitted(postType: 'original', content: _runeString(500)),
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
              shareInitiatedAt: any(named: 'shareInitiatedAt'),
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
              shareInitiatedAt: any(named: 'shareInitiatedAt'),
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
              shareInitiatedAt: any(named: 'shareInitiatedAt'),
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
              shareInitiatedAt: any(named: 'shareInitiatedAt'),
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

      // Quote posts with valid content now start the countdown rather than
      // submitting directly (CLAUDE.md §2.2).
      blocTest<PostComposeBloc, PostComposeState>(
        'emits PostComposeCountdown(5) — not PostComposeSubmitting — for quote '
        'post with exactly 5 distinct words (countdown gate, CLAUDE.md §2.2)',
        build: () => PostComposeBloc(postRepository: mockRepo),
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(
            postType: 'quote',
            // Exactly 5 distinct words — must pass the word-count threshold
            // and enter the countdown.
            content: 'alpha beta gamma delta epsilon',
            quotedPostId: 'post-abc',
          ),
        ),
        expect: () => [
          const PostComposeCountdown(secondsRemaining: 5, totalSeconds: 5),
        ],
        verify: (_) {
          // Repository must not be called until countdown completes.
          verifyNever(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
              shareInitiatedAt: any(named: 'shareInitiatedAt'),
            ),
          );
        },
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'emits PostComposeCountdown(5) for quote post with more than 5 distinct words',
        build: () => PostComposeBloc(postRepository: mockRepo),
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(
            postType: 'quote',
            content: 'This is a great quoted post indeed',
            quotedPostId: 'post-abc',
          ),
        ),
        expect: () => [
          const PostComposeCountdown(secondsRemaining: 5, totalSeconds: 5),
        ],
        verify: (_) {
          verifyNever(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
              shareInitiatedAt: any(named: 'shareInitiatedAt'),
            ),
          );
        },
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'normalizes punctuation when counting distinct words for quote posts',
        build: () => PostComposeBloc(postRepository: mockRepo),
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(
            postType: 'quote',
            // Punctuation is stripped; "alpha", "beta", "gamma", "delta",
            // "epsilon" are 5 distinct normalized tokens — enters countdown.
            content: 'alpha, beta! gamma. delta; epsilon?',
            quotedPostId: 'post-abc',
          ),
        ),
        expect: () => [
          const PostComposeCountdown(secondsRemaining: 5, totalSeconds: 5),
        ],
      );
    });

    // -----------------------------------------------------------------------
    // Repost — no content validation, but countdown required (CLAUDE.md §2.2)
    // -----------------------------------------------------------------------

    group('repost type', () {
      blocTest<PostComposeBloc, PostComposeState>(
        'skips content validation for repost type and starts countdown',
        build: () => PostComposeBloc(postRepository: mockRepo),
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(
            postType: 'repost',
            content: null,
            quotedPostId: 'post-xyz',
          ),
        ),
        expect: () => [
          const PostComposeCountdown(secondsRemaining: 5, totalSeconds: 5),
        ],
        verify: (_) {
          // Repository must not be called until countdown completes.
          verifyNever(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
              shareInitiatedAt: any(named: 'shareInitiatedAt'),
            ),
          );
        },
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
              shareInitiatedAt: any(named: 'shareInitiatedAt'),
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

    // -----------------------------------------------------------------------
    // Countdown state machine (CLAUDE.md §2.2)
    // -----------------------------------------------------------------------
    //
    // Timer ticks are simulated by dispatching PostComposeCountdownTicked
    // events manually, because the real Timer.periodic uses wall-clock time.
    // The bloc_test framework does not have fake_async built in and
    // package:fake_async is not a project dependency, so we verify the state
    // machine directly.

    group('countdown state machine (CLAUDE.md §2.2)', () {
      blocTest<PostComposeBloc, PostComposeState>(
        'PostComposeCountdown emitted for quote post — countdown starts at 5',
        build: () => PostComposeBloc(postRepository: mockRepo),
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(
            postType: 'quote',
            content: 'alpha beta gamma delta epsilon',
            quotedPostId: 'post-abc',
          ),
        ),
        expect: () => [
          const PostComposeCountdown(secondsRemaining: 5, totalSeconds: 5),
        ],
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'PostComposeCountdown emitted for repost — countdown starts at 5',
        build: () => PostComposeBloc(postRepository: mockRepo),
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(
            postType: 'repost',
            quotedPostId: 'post-xyz',
          ),
        ),
        expect: () => [
          const PostComposeCountdown(secondsRemaining: 5, totalSeconds: 5),
        ],
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'no countdown for original post — submits directly',
        build: () {
          when(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
              shareInitiatedAt: any(named: 'shareInitiatedAt'),
            ),
          ).thenAnswer((_) async => Success(_createdPost));
          return PostComposeBloc(postRepository: mockRepo);
        },
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(
            postType: 'original',
            content: 'Hello world this is fine',
          ),
        ),
        expect: () => [
          const PostComposeSubmitting(),
          PostComposeSuccess(post: _createdPost),
        ],
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'no countdown for reply post — submits directly',
        build: () {
          when(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
              shareInitiatedAt: any(named: 'shareInitiatedAt'),
            ),
          ).thenAnswer((_) async => Success(_createdPost));
          return PostComposeBloc(postRepository: mockRepo);
        },
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(
            postType: 'reply',
            content: 'Great point',
            parentId: 'post-parent',
          ),
        ),
        expect: () => [
          const PostComposeSubmitting(),
          PostComposeSuccess(post: _createdPost),
        ],
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'countdown ticks decrement secondsRemaining: 5 → 4 → 3',
        build: () => PostComposeBloc(postRepository: mockRepo),
        act: (bloc) async {
          bloc.add(
            const PostComposeSubmitted(
              postType: 'quote',
              content: 'alpha beta gamma delta epsilon',
              quotedPostId: 'post-abc',
            ),
          );
          // Simulate two manual ticks (stopping before 0 so no submission
          // is triggered and no repository mock is needed).
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(4));
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(3));
        },
        expect: () => [
          const PostComposeCountdown(secondsRemaining: 5, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 4, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 3, totalSeconds: 5),
        ],
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'cancelling during countdown returns to PostComposeInitial',
        build: () => PostComposeBloc(postRepository: mockRepo),
        act: (bloc) async {
          bloc.add(
            const PostComposeSubmitted(
              postType: 'quote',
              content: 'alpha beta gamma delta epsilon',
              quotedPostId: 'post-abc',
            ),
          );
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownCancelled());
        },
        expect: () => [
          const PostComposeCountdown(secondsRemaining: 5, totalSeconds: 5),
          const PostComposeInitial(),
        ],
        verify: (_) {
          verifyNever(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
              shareInitiatedAt: any(named: 'shareInitiatedAt'),
            ),
          );
        },
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'submits after countdown reaches 0 — emits PostComposeSubmitting then '
        'PostComposeSuccess',
        build: () {
          when(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
              shareInitiatedAt: any(named: 'shareInitiatedAt'),
            ),
          ).thenAnswer((_) async => Success(_createdPost));
          return PostComposeBloc(postRepository: mockRepo);
        },
        act: (bloc) async {
          bloc.add(
            const PostComposeSubmitted(
              postType: 'quote',
              content: 'alpha beta gamma delta epsilon',
              quotedPostId: 'post-abc',
            ),
          );
          await Future<void>.delayed(Duration.zero);
          // Drive the countdown to zero manually.
          bloc.add(const PostComposeCountdownTicked(4));
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(3));
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(2));
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(1));
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(0));
          await Future<void>.delayed(Duration.zero);
        },
        expect: () => [
          const PostComposeCountdown(secondsRemaining: 5, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 4, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 3, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 2, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 1, totalSeconds: 5),
          // secondsRemaining == 0 → no new countdown state, proceeds to submit
          const PostComposeSubmitting(),
          PostComposeSuccess(post: _createdPost),
        ],
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'repost submits after countdown reaches 0',
        build: () {
          when(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
              shareInitiatedAt: any(named: 'shareInitiatedAt'),
            ),
          ).thenAnswer((_) async => Success(_createdPost));
          return PostComposeBloc(postRepository: mockRepo);
        },
        act: (bloc) async {
          bloc.add(
            const PostComposeSubmitted(
              postType: 'repost',
              quotedPostId: 'post-xyz',
            ),
          );
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(4));
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(3));
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(2));
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(1));
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(0));
          await Future<void>.delayed(Duration.zero);
        },
        expect: () => [
          const PostComposeCountdown(secondsRemaining: 5, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 4, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 3, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 2, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 1, totalSeconds: 5),
          const PostComposeSubmitting(),
          PostComposeSuccess(post: _createdPost),
        ],
      );
    });

    // -----------------------------------------------------------------------
    // share_initiated_at — included for repost/quote, absent for others
    // (CLAUDE.md §2.2 — backend enforcement of 5-second delay)
    // -----------------------------------------------------------------------

    group('share_initiated_at field (CLAUDE.md §2.2)', () {
      late DateTime? capturedShareInitiatedAt;

      setUp(() {
        capturedShareInitiatedAt = null;
      });

      blocTest<PostComposeBloc, PostComposeState>(
        'repost submission includes non-null shareInitiatedAt after countdown',
        build: () {
          when(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
              shareInitiatedAt: any(named: 'shareInitiatedAt'),
            ),
          ).thenAnswer((invocation) async {
            capturedShareInitiatedAt =
                invocation.namedArguments[#shareInitiatedAt] as DateTime?;
            return Success(_createdPost);
          });
          return PostComposeBloc(postRepository: mockRepo);
        },
        act: (bloc) async {
          bloc.add(
            const PostComposeSubmitted(
              postType: 'repost',
              quotedPostId: 'post-xyz',
            ),
          );
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(4));
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(3));
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(2));
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(1));
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(0));
          await Future<void>.delayed(Duration.zero);
        },
        expect: () => [
          const PostComposeCountdown(secondsRemaining: 5, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 4, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 3, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 2, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 1, totalSeconds: 5),
          const PostComposeSubmitting(),
          PostComposeSuccess(post: _createdPost),
        ],
        verify: (_) {
          expect(
            capturedShareInitiatedAt,
            isNotNull,
            reason:
                'shareInitiatedAt must be non-null for repost submissions',
          );
        },
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'quote submission includes non-null shareInitiatedAt after countdown',
        build: () {
          when(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
              shareInitiatedAt: any(named: 'shareInitiatedAt'),
            ),
          ).thenAnswer((invocation) async {
            capturedShareInitiatedAt =
                invocation.namedArguments[#shareInitiatedAt] as DateTime?;
            return Success(_createdPost);
          });
          return PostComposeBloc(postRepository: mockRepo);
        },
        act: (bloc) async {
          bloc.add(
            const PostComposeSubmitted(
              postType: 'quote',
              content: 'alpha beta gamma delta epsilon',
              quotedPostId: 'post-abc',
            ),
          );
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(4));
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(3));
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(2));
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(1));
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(0));
          await Future<void>.delayed(Duration.zero);
        },
        expect: () => [
          const PostComposeCountdown(secondsRemaining: 5, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 4, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 3, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 2, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 1, totalSeconds: 5),
          const PostComposeSubmitting(),
          PostComposeSuccess(post: _createdPost),
        ],
        verify: (_) {
          expect(
            capturedShareInitiatedAt,
            isNotNull,
            reason:
                'shareInitiatedAt must be non-null for quote submissions',
          );
        },
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'original post does NOT include shareInitiatedAt',
        build: () {
          when(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
              shareInitiatedAt: any(named: 'shareInitiatedAt'),
            ),
          ).thenAnswer((invocation) async {
            capturedShareInitiatedAt =
                invocation.namedArguments[#shareInitiatedAt] as DateTime?;
            return Success(_createdPost);
          });
          return PostComposeBloc(postRepository: mockRepo);
        },
        act: (bloc) => bloc.add(
          const PostComposeSubmitted(
            postType: 'original',
            content: 'Hello world this is fine',
          ),
        ),
        expect: () => [
          const PostComposeSubmitting(),
          PostComposeSuccess(post: _createdPost),
        ],
        verify: (_) {
          expect(
            capturedShareInitiatedAt,
            isNull,
            reason:
                'shareInitiatedAt must be null for original post submissions',
          );
        },
      );

      blocTest<PostComposeBloc, PostComposeState>(
        'timing validation error from backend surfaces as PostComposeError',
        build: () {
          when(
            () => mockRepo.createPost(
              postType: any(named: 'postType'),
              content: any(named: 'content'),
              parentId: any(named: 'parentId'),
              quotedPostId: any(named: 'quotedPostId'),
              shareInitiatedAt: any(named: 'shareInitiatedAt'),
            ),
          ).thenAnswer(
            (_) async => const Err(
              ValidationFailure(
                message:
                    'share action must be initiated at least 5 seconds before submission',
              ),
            ),
          );
          return PostComposeBloc(postRepository: mockRepo);
        },
        act: (bloc) async {
          bloc.add(
            const PostComposeSubmitted(
              postType: 'repost',
              quotedPostId: 'post-xyz',
            ),
          );
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(4));
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(3));
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(2));
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(1));
          await Future<void>.delayed(Duration.zero);
          bloc.add(const PostComposeCountdownTicked(0));
          await Future<void>.delayed(Duration.zero);
        },
        expect: () => [
          const PostComposeCountdown(secondsRemaining: 5, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 4, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 3, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 2, totalSeconds: 5),
          const PostComposeCountdown(secondsRemaining: 1, totalSeconds: 5),
          const PostComposeSubmitting(),
          isA<PostComposeError>().having(
            (s) => s.failure,
            'failure',
            isA<ValidationFailure>(),
          ),
        ],
      );
    });
  });
}
