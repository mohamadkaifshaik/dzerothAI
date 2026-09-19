// ignore_for_file: prefer_initializing_formals
import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/repositories/settings_repository.dart';

part 'settings_event.dart';
part 'settings_state.dart';

/// Manages account settings state: privacy toggle and account suspension.
class SettingsBloc extends Bloc<SettingsEvent, SettingsState> {
  SettingsBloc({required SettingsRepository settingsRepository})
    : _settingsRepository = settingsRepository,
      super(const SettingsInitial()) {
    on<SettingsFetchRequested>(_onFetchRequested);
    on<SettingsPrivacyToggled>(_onPrivacyToggled);
    on<SettingsAccountSuspensionRequested>(_onAccountSuspensionRequested);
  }

  final SettingsRepository _settingsRepository;

  Future<void> _onFetchRequested(
    SettingsFetchRequested event,
    Emitter<SettingsState> emit,
  ) async {
    emit(const SettingsLoading());
    final result = await _settingsRepository.getSettings();
    switch (result) {
      case Success(:final value):
        emit(SettingsLoaded(isPrivate: value.isPrivate));
      case Err(:final failure):
        emit(SettingsError(failure: failure));
    }
  }

  Future<void> _onPrivacyToggled(
    SettingsPrivacyToggled event,
    Emitter<SettingsState> emit,
  ) async {
    // Optimistic: hold the previous privacy value so we can restore on error.
    final previous = switch (state) {
      SettingsLoaded(:final isPrivate) => isPrivate,
      SettingsSaved(:final isPrivate) => isPrivate,
      _ => null,
    };

    emit(SettingsSaving(isPrivate: event.isPrivate));

    final result = await _settingsRepository.updatePrivacy(
      isPrivate: event.isPrivate,
    );

    switch (result) {
      case Success(:final value):
        emit(SettingsSaved(isPrivate: value.isPrivate));
      case Err(:final failure):
        // Roll back to previous value; if unknown, remain at the requested value.
        emit(SettingsError(failure: failure));
        // Re-emit SettingsLoaded so the screen can recover the toggle state.
        if (previous != null) {
          emit(SettingsLoaded(isPrivate: previous));
        }
    }
  }

  Future<void> _onAccountSuspensionRequested(
    SettingsAccountSuspensionRequested event,
    Emitter<SettingsState> emit,
  ) async {
    emit(const SettingsLoading());
    final result = await _settingsRepository.suspendAccount();
    switch (result) {
      case Success():
        emit(const SettingsAccountSuspended());
      case Err(:final failure):
        emit(SettingsError(failure: failure));
    }
  }
}
