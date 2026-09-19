part of 'report_bloc.dart';

/// All events that can be dispatched to [ReportBloc].
sealed class ReportEvent {
  const ReportEvent();
}

/// Submit a report for the post identified by [postId].
final class ReportPostSubmitted extends ReportEvent {
  const ReportPostSubmitted({
    required this.postId,
    required this.reason,
    this.detail,
  });

  final String postId;
  final String reason;
  final String? detail;
}

/// Submit a report for the user identified by [userId].
final class ReportUserSubmitted extends ReportEvent {
  const ReportUserSubmitted({
    required this.userId,
    required this.reason,
    this.detail,
  });

  final String userId;
  final String reason;
  final String? detail;
}
