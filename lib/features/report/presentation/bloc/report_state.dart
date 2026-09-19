part of 'report_bloc.dart';

/// All states that [ReportBloc] can emit.
sealed class ReportState extends Equatable {
  const ReportState();

  @override
  List<Object?> get props => [];
}

/// No report has been submitted yet.
final class ReportInitial extends ReportState {
  const ReportInitial();
}

/// A report submission is in progress.
final class ReportLoading extends ReportState {
  const ReportLoading();
}

/// The report was accepted by the server.
final class ReportSuccess extends ReportState {
  const ReportSuccess();
}

/// The report submission failed.
final class ReportError extends ReportState {
  const ReportError({required this.failure});

  final Failure failure;

  @override
  List<Object?> get props => [failure];
}
