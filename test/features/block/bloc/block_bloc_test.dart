import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:dzeroth/core/error/failures.dart';
import 'package:dzeroth/core/error/result.dart';
import 'package:dzeroth/features/block/domain/repositories/block_repository.dart';
import 'package:dzeroth/features/block/presentation/bloc/block_bloc.dart';

// ---------------------------------------------------------------------------
// Mock
// ---------------------------------------------------------------------------

class MockBlockRepository extends Mock implements BlockRepository {}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const _targetUserId = 'user-target-1';

BlockBloc _makeBloc(MockBlockRepository repo) =>
    BlockBloc(blockRepository: repo, targetUserId: _targetUserId);

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  late MockBlockRepository mockRepo;

  setUp(() {
    mockRepo = MockBlockRepository();
  });

  group('BlockBloc', () {
    // -----------------------------------------------------------------------
    // BlockUserRequested → success
    // -----------------------------------------------------------------------

    blocTest<BlockBloc, BlockState>(
      'BlockBloc_EmitsSuccess_OnBlockRequested: '
      'emits [BlockLoading, BlockSuccess(action: "blocked")] on success',
      build: () {
        when(() => mockRepo.blockUser(_targetUserId))
            .thenAnswer((_) async => const Success(null));
        return _makeBloc(mockRepo);
      },
      act: (bloc) => bloc.add(const BlockUserRequested()),
      expect: () => [
        const BlockLoading(),
        const BlockSuccess(action: 'blocked'),
      ],
      verify: (_) {
        verify(() => mockRepo.blockUser(_targetUserId)).called(1);
      },
    );

    // -----------------------------------------------------------------------
    // UnblockUserRequested → success
    // -----------------------------------------------------------------------

    blocTest<BlockBloc, BlockState>(
      'emits [BlockLoading, BlockSuccess(action: "unblocked")] on unblock',
      build: () {
        when(() => mockRepo.unblockUser(_targetUserId))
            .thenAnswer((_) async => const Success(null));
        return _makeBloc(mockRepo);
      },
      act: (bloc) => bloc.add(const UnblockUserRequested()),
      expect: () => [
        const BlockLoading(),
        const BlockSuccess(action: 'unblocked'),
      ],
    );

    // -----------------------------------------------------------------------
    // MuteUserRequested → success
    // -----------------------------------------------------------------------

    blocTest<BlockBloc, BlockState>(
      'BlockBloc_EmitsSuccess_OnMuteRequested: '
      'emits [BlockLoading, BlockSuccess(action: "muted")] on success',
      build: () {
        when(() => mockRepo.muteUser(_targetUserId))
            .thenAnswer((_) async => const Success(null));
        return _makeBloc(mockRepo);
      },
      act: (bloc) => bloc.add(const MuteUserRequested()),
      expect: () => [const BlockLoading(), const BlockSuccess(action: 'muted')],
      verify: (_) {
        verify(() => mockRepo.muteUser(_targetUserId)).called(1);
      },
    );

    // -----------------------------------------------------------------------
    // UnmuteUserRequested → success
    // -----------------------------------------------------------------------

    blocTest<BlockBloc, BlockState>(
      'emits [BlockLoading, BlockSuccess(action: "unmuted")] on unmute',
      build: () {
        when(() => mockRepo.unmuteUser(_targetUserId))
            .thenAnswer((_) async => const Success(null));
        return _makeBloc(mockRepo);
      },
      act: (bloc) => bloc.add(const UnmuteUserRequested()),
      expect: () => [
        const BlockLoading(),
        const BlockSuccess(action: 'unmuted'),
      ],
    );

    // -----------------------------------------------------------------------
    // Repository failure → BlockError
    // -----------------------------------------------------------------------

    blocTest<BlockBloc, BlockState>(
      'BlockBloc_EmitsError_OnFailure: '
      'emits [BlockLoading, BlockError] when blockUser returns a failure',
      build: () {
        when(() => mockRepo.blockUser(_targetUserId))
            .thenAnswer((_) async => const Err(NetworkFailure()));
        return _makeBloc(mockRepo);
      },
      act: (bloc) => bloc.add(const BlockUserRequested()),
      expect: () => [
        const BlockLoading(),
        isA<BlockError>().having(
          (s) => s.failure,
          'failure',
          isA<NetworkFailure>(),
        ),
      ],
    );

    // -----------------------------------------------------------------------
    // Mute failure
    // -----------------------------------------------------------------------

    blocTest<BlockBloc, BlockState>(
      'emits [BlockLoading, BlockError] when muteUser returns a failure',
      build: () {
        when(() => mockRepo.muteUser(_targetUserId))
            .thenAnswer((_) async => const Err(ServerFailure()));
        return _makeBloc(mockRepo);
      },
      act: (bloc) => bloc.add(const MuteUserRequested()),
      expect: () => [
        const BlockLoading(),
        isA<BlockError>().having(
          (s) => s.failure,
          'failure',
          isA<ServerFailure>(),
        ),
      ],
    );
  });
}
