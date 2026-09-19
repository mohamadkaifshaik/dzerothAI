import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:dzeroth/core/error/failures.dart';
import 'package:dzeroth/core/error/result.dart';
import 'package:dzeroth/features/report/domain/repositories/report_repository.dart';
import 'package:dzeroth/features/report/presentation/bloc/report_bloc.dart';

// ---------------------------------------------------------------------------
// Mock
// ---------------------------------------------------------------------------

class MockReportRepository extends Mock implements ReportRepository {}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const _postId = 'post-id-1';
const _userId = 'user-id-1';
const _reason = 'spam';

ReportBloc _makeBloc(MockReportRepository repo) =>
    ReportBloc(reportRepository: repo);

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  late MockReportRepository mockRepo;

  setUp(() {
    mockRepo = MockReportRepository();
  });

  group('ReportBloc', () {
    // -----------------------------------------------------------------------
    // ReportPostSubmitted — success
    // -----------------------------------------------------------------------

    blocTest<ReportBloc, ReportState>(
      'TestReportBloc_SubmitPostReport_Success_EmitsSuccess: '
      'emits [ReportLoading, ReportSuccess] when reportPost succeeds',
      build: () {
        when(
          () => mockRepo.reportPost(
            postId: any(named: 'postId'),
            reason: any(named: 'reason'),
            detail: any(named: 'detail'),
          ),
        ).thenAnswer((_) async => const Success(null));
        return _makeBloc(mockRepo);
      },
      act: (bloc) =>
          bloc.add(const ReportPostSubmitted(postId: _postId, reason: _reason)),
      expect: () => [const ReportLoading(), const ReportSuccess()],
      verify: (_) {
        verify(
          () => mockRepo.reportPost(
            postId: _postId,
            reason: _reason,
            detail: null,
          ),
        ).called(1);
      },
    );

    // -----------------------------------------------------------------------
    // ReportPostSubmitted — error
    // -----------------------------------------------------------------------

    blocTest<ReportBloc, ReportState>(
      'TestReportBloc_SubmitPostReport_Error_EmitsError: '
      'emits [ReportLoading, ReportError] when reportPost returns a failure',
      build: () {
        when(
          () => mockRepo.reportPost(
            postId: any(named: 'postId'),
            reason: any(named: 'reason'),
            detail: any(named: 'detail'),
          ),
        ).thenAnswer((_) async => const Err(NetworkFailure()));
        return _makeBloc(mockRepo);
      },
      act: (bloc) =>
          bloc.add(const ReportPostSubmitted(postId: _postId, reason: _reason)),
      expect: () => [
        const ReportLoading(),
        isA<ReportError>().having(
          (s) => s.failure,
          'failure',
          isA<NetworkFailure>(),
        ),
      ],
    );

    // -----------------------------------------------------------------------
    // ReportUserSubmitted — self-report (backend returns 400 → ValidationFailure)
    // -----------------------------------------------------------------------

    blocTest<ReportBloc, ReportState>(
      'TestReportBloc_SubmitUserReport_SelfReport_EmitsError: '
      'emits [ReportLoading, ReportError(ValidationFailure)] '
      'when the backend rejects a self-report with 400',
      build: () {
        when(
          () => mockRepo.reportUser(
            userId: any(named: 'userId'),
            reason: any(named: 'reason'),
            detail: any(named: 'detail'),
          ),
        ).thenAnswer(
          (_) async => const Err(
            ValidationFailure(message: 'You cannot report yourself.'),
          ),
        );
        return _makeBloc(mockRepo);
      },
      act: (bloc) =>
          bloc.add(const ReportUserSubmitted(userId: _userId, reason: _reason)),
      expect: () => [
        const ReportLoading(),
        isA<ReportError>().having(
          (s) => s.failure,
          'failure',
          isA<ValidationFailure>(),
        ),
      ],
      verify: (_) {
        verify(
          () => mockRepo.reportUser(
            userId: _userId,
            reason: _reason,
            detail: null,
          ),
        ).called(1);
      },
    );

    // -----------------------------------------------------------------------
    // Loading state appears before success
    // -----------------------------------------------------------------------

    blocTest<ReportBloc, ReportState>(
      'TestReportBloc_Loading_EmitsLoadingThenSuccess: '
      'emits ReportLoading before ReportSuccess',
      build: () {
        when(
          () => mockRepo.reportPost(
            postId: any(named: 'postId'),
            reason: any(named: 'reason'),
            detail: any(named: 'detail'),
          ),
        ).thenAnswer((_) async => const Success(null));
        return _makeBloc(mockRepo);
      },
      act: (bloc) => bloc.add(
        const ReportPostSubmitted(postId: _postId, reason: 'harassment'),
      ),
      expect: () => [const ReportLoading(), const ReportSuccess()],
    );

    // -----------------------------------------------------------------------
    // ReportUserSubmitted — success
    // -----------------------------------------------------------------------

    blocTest<ReportBloc, ReportState>(
      'emits [ReportLoading, ReportSuccess] when reportUser succeeds',
      build: () {
        when(
          () => mockRepo.reportUser(
            userId: any(named: 'userId'),
            reason: any(named: 'reason'),
            detail: any(named: 'detail'),
          ),
        ).thenAnswer((_) async => const Success(null));
        return _makeBloc(mockRepo);
      },
      act: (bloc) => bloc.add(
        const ReportUserSubmitted(
          userId: _userId,
          reason: 'harassment',
          detail: 'Additional context',
        ),
      ),
      expect: () => [const ReportLoading(), const ReportSuccess()],
      verify: (_) {
        verify(
          () => mockRepo.reportUser(
            userId: _userId,
            reason: 'harassment',
            detail: 'Additional context',
          ),
        ).called(1);
      },
    );
  });
}
