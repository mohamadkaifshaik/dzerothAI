part of 'studio_bloc.dart';

/// All events that can be dispatched to [StudioBloc].
sealed class StudioEvent {
  const StudioEvent();
}

/// Load the first page of analytics.
final class StudioFetchRequested extends StudioEvent {
  const StudioFetchRequested();
}

/// Load the next page of analytics.
///
/// Ignored when the BLoC is in [StudioTerminated] state.
final class StudioNextPageRequested extends StudioEvent {
  const StudioNextPageRequested();
}
