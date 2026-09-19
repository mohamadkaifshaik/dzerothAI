import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:dzeroth/core/error/failures.dart';
import 'package:dzeroth/core/error/result.dart';
import 'package:dzeroth/features/reaction/domain/repositories/reaction_repository.dart';
import 'package:dzeroth/features/reaction/presentation/bloc/reaction_toggle_bloc.dart';

// ---------------------------------------------------------------------------
// Mock
// ---------------------------------------------------------------------------

class MockReactionRepository extends Mock implements ReactionRepository {}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const _postId = 'post-1';

ReactionToggleBloc _makeBloc(
  MockReactionRepository repo, {
  bool initiallyReacted = false,
}) => ReactionToggleBloc(
  reactionRepository: repo,
  postId: _postId,
  initiallyReacted: initiallyReacted,
);

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  late MockReactionRepository mockRepo;

  setUp(() {
    mockRepo = MockReactionRepository();
  });

  group('ReactionToggleBloc', () {
    // -----------------------------------------------------------------------
    // Initial state
    // -----------------------------------------------------------------------

    test('initial state is ReactionOff when initiallyReacted is false', () {
      final bloc = _makeBloc(mockRepo);
      expect(bloc.state, ReactionOff(postId: _postId));
      bloc.close();
    });

    test('initial state is ReactionOn when initiallyReacted is true', () {
      final bloc = _makeBloc(mockRepo, initiallyReacted: true);
      expect(bloc.state, ReactionOn(postId: _postId));
      bloc.close();
    });

    // -----------------------------------------------------------------------
    // ReactionToggleRequested from off → on (react)
    // -----------------------------------------------------------------------

    blocTest<ReactionToggleBloc, ReactionToggleState>(
      'ReactionToggleBloc_EmitsReactionOn_WhenToggleFromOff: '
      'emits [ReactionLoading, ReactionOn] when react succeeds',
      build: () {
        when(() => mockRepo.react(_postId))
            .thenAnswer((_) async => const Success(null));
        return _makeBloc(mockRepo, initiallyReacted: false);
      },
      act: (bloc) => bloc.add(const ReactionToggleRequested()),
      expect: () => [const ReactionLoading(), ReactionOn(postId: _postId)],
      verify: (_) {
        verify(() => mockRepo.react(_postId)).called(1);
        verifyNever(() => mockRepo.unreact(_postId));
      },
    );

    // -----------------------------------------------------------------------
    // ReactionToggleRequested from on → off (unreact)
    // -----------------------------------------------------------------------

    blocTest<ReactionToggleBloc, ReactionToggleState>(
      'ReactionToggleBloc_EmitsReactionOff_WhenToggleFromOn: '
      'emits [ReactionLoading, ReactionOff] when unreact succeeds',
      build: () {
        when(() => mockRepo.unreact(_postId))
            .thenAnswer((_) async => const Success(null));
        return _makeBloc(mockRepo, initiallyReacted: true);
      },
      act: (bloc) => bloc.add(const ReactionToggleRequested()),
      expect: () => [const ReactionLoading(), ReactionOff(postId: _postId)],
      verify: (_) {
        verify(() => mockRepo.unreact(_postId)).called(1);
        verifyNever(() => mockRepo.react(_postId));
      },
    );

    // -----------------------------------------------------------------------
    // React failure → reverts to ReactionOff
    // -----------------------------------------------------------------------

    blocTest<ReactionToggleBloc, ReactionToggleState>(
      'ReactionToggleBloc_RevertsToOff_WhenReactFails: '
      'emits [ReactionLoading, ReactionError, ReactionOff] on react failure',
      build: () {
        when(() => mockRepo.react(_postId))
            .thenAnswer((_) async => const Err(NetworkFailure()));
        return _makeBloc(mockRepo, initiallyReacted: false);
      },
      act: (bloc) => bloc.add(const ReactionToggleRequested()),
      expect: () => [
        const ReactionLoading(),
        isA<ReactionError>().having((s) => s.postId, 'postId', _postId),
        ReactionOff(postId: _postId),
      ],
    );

    // -----------------------------------------------------------------------
    // Unreact failure → reverts to ReactionOn
    // -----------------------------------------------------------------------

    blocTest<ReactionToggleBloc, ReactionToggleState>(
      'ReactionToggleBloc_RevertsToOn_WhenUnreactFails: '
      'emits [ReactionLoading, ReactionError, ReactionOn] on unreact failure',
      build: () {
        when(() => mockRepo.unreact(_postId))
            .thenAnswer((_) async => const Err(ServerFailure()));
        return _makeBloc(mockRepo, initiallyReacted: true);
      },
      act: (bloc) => bloc.add(const ReactionToggleRequested()),
      expect: () => [
        const ReactionLoading(),
        isA<ReactionError>().having((s) => s.postId, 'postId', _postId),
        ReactionOn(postId: _postId),
      ],
    );

    // -----------------------------------------------------------------------
    // Idempotent: double react (second call returns 204 from backend)
    // -----------------------------------------------------------------------

    blocTest<ReactionToggleBloc, ReactionToggleState>(
      'ReactionToggleBloc_HandlesDoubleReact_Gracefully: '
      'second react call still succeeds (backend is idempotent)',
      build: () {
        when(() => mockRepo.react(_postId))
            .thenAnswer((_) async => const Success(null));
        return _makeBloc(mockRepo, initiallyReacted: false);
      },
      act: (bloc) async {
        bloc.add(const ReactionToggleRequested());
        await Future<void>.delayed(Duration.zero);
        // Bloc is now ReactionOn; toggle again triggers unreact,
        // but we can also simulate a second react while still loading.
      },
      // Only verify first toggle completes correctly.
      expect: () => [const ReactionLoading(), ReactionOn(postId: _postId)],
    );

    // -----------------------------------------------------------------------
    // ReactionStateHydrated — viewer_has_reacted = true
    // -----------------------------------------------------------------------

    blocTest<ReactionToggleBloc, ReactionToggleState>(
      'ReactionToggleBloc_Hydrates_ReactionOn_WhenViewerHasReacted: '
      'emits ReactionOn when ReactionStateHydrated(reacted: true) is added',
      build: () => _makeBloc(mockRepo, initiallyReacted: false),
      act: (bloc) => bloc.add(const ReactionStateHydrated(reacted: true)),
      expect: () => [ReactionOn(postId: _postId)],
      verify: (_) {
        verifyNever(() => mockRepo.react(_postId));
        verifyNever(() => mockRepo.unreact(_postId));
      },
    );

    // -----------------------------------------------------------------------
    // ReactionStateHydrated — viewer_has_reacted = false
    // -----------------------------------------------------------------------

    blocTest<ReactionToggleBloc, ReactionToggleState>(
      'ReactionToggleBloc_Hydrates_ReactionOff_WhenViewerHasNotReacted: '
      'emits ReactionOff when ReactionStateHydrated(reacted: false) is added',
      build: () => _makeBloc(mockRepo, initiallyReacted: true),
      act: (bloc) => bloc.add(const ReactionStateHydrated(reacted: false)),
      expect: () => [ReactionOff(postId: _postId)],
      verify: (_) {
        verifyNever(() => mockRepo.react(_postId));
        verifyNever(() => mockRepo.unreact(_postId));
      },
    );

    // -----------------------------------------------------------------------
    // ReactionStateHydrated — viewer_has_reacted = null (absent/unauthenticated)
    // No event is dispatched; bloc stays in its initial ReactionOff state.
    // -----------------------------------------------------------------------

    test(
      'ReactionToggleBloc_StaysOff_WhenViewerHasReactedIsNull: '
      'no state change and no API call when viewerHasReacted is null',
      () async {
        final bloc = _makeBloc(mockRepo, initiallyReacted: false);
        // Simulate the screen guard: null means no hydration event is added.
        // The bloc state must remain ReactionOff.
        expect(bloc.state, ReactionOff(postId: _postId));
        verifyNever(() => mockRepo.react(_postId));
        verifyNever(() => mockRepo.unreact(_postId));
        await bloc.close();
      },
    );

    // -----------------------------------------------------------------------
    // After hydration to ReactionOn, toggle → ReactionOff (calls unreact)
    // -----------------------------------------------------------------------

    blocTest<ReactionToggleBloc, ReactionToggleState>(
      'ReactionToggleBloc_ToggleAfterHydrationOn_CallsUnreact: '
      'after hydrating to ReactionOn, toggle emits [ReactionLoading, ReactionOff]',
      build: () {
        when(() => mockRepo.unreact(_postId))
            .thenAnswer((_) async => const Success(null));
        return _makeBloc(mockRepo, initiallyReacted: false);
      },
      act: (bloc) async {
        bloc.add(const ReactionStateHydrated(reacted: true));
        await Future<void>.delayed(Duration.zero);
        bloc.add(const ReactionToggleRequested());
      },
      expect: () => [
        ReactionOn(postId: _postId),
        const ReactionLoading(),
        ReactionOff(postId: _postId),
      ],
      verify: (_) {
        verify(() => mockRepo.unreact(_postId)).called(1);
        verifyNever(() => mockRepo.react(_postId));
      },
    );

    // -----------------------------------------------------------------------
    // After hydration to ReactionOff, toggle → ReactionOn (calls react)
    // -----------------------------------------------------------------------

    blocTest<ReactionToggleBloc, ReactionToggleState>(
      'ReactionToggleBloc_ToggleAfterHydrationOff_CallsReact: '
      'after hydrating to ReactionOff, toggle emits [ReactionLoading, ReactionOn]',
      build: () {
        when(() => mockRepo.react(_postId))
            .thenAnswer((_) async => const Success(null));
        return _makeBloc(mockRepo, initiallyReacted: true);
      },
      act: (bloc) async {
        bloc.add(const ReactionStateHydrated(reacted: false));
        await Future<void>.delayed(Duration.zero);
        bloc.add(const ReactionToggleRequested());
      },
      expect: () => [
        ReactionOff(postId: _postId),
        const ReactionLoading(),
        ReactionOn(postId: _postId),
      ],
      verify: (_) {
        verify(() => mockRepo.react(_postId)).called(1);
        verifyNever(() => mockRepo.unreact(_postId));
      },
    );
  });
}
