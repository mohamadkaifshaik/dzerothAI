part of 'settings_bloc.dart';

/// All events that can be dispatched to [SettingsBloc].
sealed class SettingsEvent {
  const SettingsEvent();
}

/// Load the authenticated user's current settings.
final class SettingsFetchRequested extends SettingsEvent {
  const SettingsFetchRequested();
}

/// Toggle the account privacy flag.
final class SettingsPrivacyToggled extends SettingsEvent {
  const SettingsPrivacyToggled({required this.isPrivate});

  final bool isPrivate;
}

/// The user confirmed account suspension.
final class SettingsAccountSuspensionRequested extends SettingsEvent {
  const SettingsAccountSuspensionRequested();
}
