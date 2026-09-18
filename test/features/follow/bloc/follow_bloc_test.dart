import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:dzeroth/core/error/failures.dart';
import 'package:dzeroth/core/error/result.dart';
import 'package:dzeroth/features/follow/domain/entities/follow_page.dart';
import 'package:dzeroth/features/follow/domain/repositories/follow_repository.dart';
import 'package:dzeroth/features/follow/presentation/bloc/follow_bloc.dart';

// ---------------------------------------------------------------------------
// Mock
// ---------------------------------------------------------------------------

class MockFollowRepository extends Mock implements FollowRepository {}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const _targetUserId = 'user-target-1';

FollowBloc _makeBloc(MockFollowRepository repo) =>
    FollowBloc(followRepository: repo, targetUserId: _targetUserId);

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  late MockFollowRepository mockRepo;

  setUp(() {
    mockRepo = MockFollowRepository();
  });

  group('FollowBloc', () {
    // -----------------------------------------------------------------------
    // FollowRequested → success
    // -----------------------------------------------------------------------

    blocTest<FollowBloc, FollowState>(
      'FollowBloc_EmitsSuccess_OnFollowRequested: '
      'emits [FollowLoading, FollowSuccess(isFollowing: true)] on success',
      build: () {
        when(() => mockRepo.follow(_targetUserId))
            .thenAnswer((_) async => const Success(null));
        return _makeBloc(mockRepo);
      },
      act: (bloc) => bloc.add(const FollowRequested()),
      expect: () => [
        const FollowLoading(),
        const FollowSuccess(isFollowing: true),
      ],
      verify: (_) {
        verify(() => mockRepo.follow(_targetUserId)).called(1);
      },
    );

    // -----------------------------------------------------------------------
    // UnfollowRequested → success
    // -----------------------------------------------------------------------

    blocTest<FollowBloc, FollowState>(
      'FollowBloc_EmitsSuccess_OnUnfollowRequested: '
      'emits [FollowLoading, FollowSuccess(isFollowing: false)] on success',
      build: () {
        when(() => mockRepo.unfollow(_targetUserId))
            .thenAnswer((_) async => const Success(null));
        return _makeBloc(mockRepo);
      },
      act: (bloc) => bloc.add(const UnfollowRequested()),
      expect: () => [
        const FollowLoading(),
        const FollowSuccess(isFollowing: false),
      ],
      verify: (_) {
        verify(() => mockRepo.unfollow(_targetUserId)).called(1);
      },
    );

    // -----------------------------------------------------------------------
    // Repository failure → FollowError
    // -----------------------------------------------------------------------

    blocTest<FollowBloc, FollowState>(
      'FollowBloc_EmitsError_OnFailure: '
      'emits [FollowLoading, FollowError] when repository returns a failure',
      build: () {
        when(() => mockRepo.follow(_targetUserId))
            .thenAnswer((_) async => const Err(NetworkFailure()));
        return _makeBloc(mockRepo);
      },
      act: (bloc) => bloc.add(const FollowRequested()),
      expect: () => [
        const FollowLoading(),
        isA<FollowError>().having(
          (s) => s.failure,
          'failure',
          isA<NetworkFailure>(),
        ),
      ],
    );

    // -----------------------------------------------------------------------
    // UnfollowRequested failure
    // -----------------------------------------------------------------------

    blocTest<FollowBloc, FollowState>(
      'emits [FollowLoading, FollowError] when unfollow returns a failure',
      build: () {
        when(() => mockRepo.unfollow(_targetUserId))
            .thenAnswer((_) async => const Err(ServerFailure()));
        return _makeBloc(mockRepo);
      },
      act: (bloc) => bloc.add(const UnfollowRequested()),
      expect: () => [
        const FollowLoading(),
        isA<FollowError>().having(
          (s) => s.failure,
          'failure',
          isA<ServerFailure>(),
        ),
      ],
    );

    // -----------------------------------------------------------------------
    // FollowStatusCheckRequested → success (following)
    // -----------------------------------------------------------------------

    blocTest<FollowBloc, FollowState>(
      'emits [FollowLoading, FollowSuccess(isFollowing: true)] on status check '
      'when already following',
      build: () {
        when(() => mockRepo.isFollowing(_targetUserId))
            .thenAnswer((_) async => const Success(true));
        return _makeBloc(mockRepo);
      },
      act: (bloc) => bloc.add(const FollowStatusCheckRequested()),
      expect: () => [
        const FollowLoading(),
        const FollowSuccess(isFollowing: true),
      ],
    );

    // -----------------------------------------------------------------------
    // FollowStatusCheckRequested → success (not following)
    // -----------------------------------------------------------------------

    blocTest<FollowBloc, FollowState>(
      'emits [FollowLoading, FollowSuccess(isFollowing: false)] on status check '
      'when not following',
      build: () {
        when(() => mockRepo.isFollowing(_targetUserId))
            .thenAnswer((_) async => const Success(false));
        return _makeBloc(mockRepo);
      },
      act: (bloc) => bloc.add(const FollowStatusCheckRequested()),
      expect: () => [
        const FollowLoading(),
        const FollowSuccess(isFollowing: false),
      ],
    );
  });
}
