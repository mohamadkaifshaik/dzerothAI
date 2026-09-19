// ignore_for_file: prefer_initializing_formals
import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/repositories/report_repository.dart';

part 'report_event.dart';
part 'report_state.dart';

/// Manages a single report submission (post or user).
///
/// A new instance is created inside [ReportSheet] so state is scoped to the
/// modal and discarded when the sheet closes.  No global BLoC registration is
/// required.
///
/// No countdown is required for reports — CLAUDE.md §2.2 covers share/quote
/// only.
class ReportBloc extends Bloc<ReportEvent, ReportState> {
  ReportBloc({required ReportRepository reportRepository})
    : _repository = reportRepository,
      super(const ReportInitial()) {
    on<ReportPostSubmitted>(_onReportPostSubmitted);
    on<ReportUserSubmitted>(_onReportUserSubmitted);
  }

  final ReportRepository _repository;

  Future<void> _onReportPostSubmitted(
    ReportPostSubmitted event,
    Emitter<ReportState> emit,
  ) async {
    emit(const ReportLoading());
    final result = await _repository.reportPost(
      postId: event.postId,
      reason: event.reason,
      detail: event.detail,
    );
    switch (result) {
      case Success():
        emit(const ReportSuccess());
      case Err(:final failure):
        emit(ReportError(failure: failure));
    }
  }

  Future<void> _onReportUserSubmitted(
    ReportUserSubmitted event,
    Emitter<ReportState> emit,
  ) async {
    emit(const ReportLoading());
    final result = await _repository.reportUser(
      userId: event.userId,
      reason: event.reason,
      detail: event.detail,
    );
    switch (result) {
      case Success():
        emit(const ReportSuccess());
      case Err(:final failure):
        emit(ReportError(failure: failure));
    }
  }
}
