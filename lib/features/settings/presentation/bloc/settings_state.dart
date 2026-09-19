part of 'settings_bloc.dart';

/// All states that [SettingsBloc] can emit.
sealed class SettingsState extends Equatable {
  const SettingsState();

  @override
  List<Object?> get props => [];
}

/// Initial state before any settings have been requested.
final class SettingsInitial extends SettingsState {
  const SettingsInitial();
}

/// Settings fetch is in progress.
final class SettingsLoading extends SettingsState {
  const SettingsLoading();
}

/// Settings were loaded successfully.
final class SettingsLoaded extends SettingsState {
  const SettingsLoaded({required this.isPrivate});

  final bool isPrivate;

  @override
  List<Object?> get props => [isPrivate];
}

/// A settings update is being persisted.
final class SettingsSaving extends SettingsState {
  const SettingsSaving({required this.isPrivate});

  final bool isPrivate;

  @override
  List<Object?> get props => [isPrivate];
}

/// A settings update completed successfully.
final class SettingsSaved extends SettingsState {
  const SettingsSaved({required this.isPrivate});

  final bool isPrivate;

  @override
  List<Object?> get props => [isPrivate];
}

/// A settings operation failed.
final class SettingsError extends SettingsState {
  const SettingsError({required this.failure});

  final Failure failure;

  @override
  List<Object?> get props => [failure];
}

/// Terminal state: the account has been suspended and the session is over.
final class SettingsAccountSuspended extends SettingsState {
  const SettingsAccountSuspended();
}
