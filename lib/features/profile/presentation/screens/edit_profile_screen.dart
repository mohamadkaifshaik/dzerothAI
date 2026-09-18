import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:go_router/go_router.dart';

import '../../domain/entities/own_profile.dart';
import '../bloc/profile_bloc.dart';

/// Edit profile screen.
///
/// Only accessible for the authenticated user's own profile.
/// Authorization is enforced by the router guard; this screen trusts
/// that the loaded profile is an [OwnProfile].
class EditProfileScreen extends StatefulWidget {
  const EditProfileScreen({super.key});

  @override
  State<EditProfileScreen> createState() => _EditProfileScreenState();
}

class _EditProfileScreenState extends State<EditProfileScreen> {
  final _formKey = GlobalKey<FormState>();
  late final TextEditingController _displayNameController;
  late final TextEditingController _bioController;
  late final TextEditingController _locationController;
  late final TextEditingController _websiteController;

  bool _initialized = false;

  @override
  void dispose() {
    _displayNameController.dispose();
    _bioController.dispose();
    _locationController.dispose();
    _websiteController.dispose();
    super.dispose();
  }

  void _initialize(OwnProfile profile) {
    if (_initialized) return;
    _displayNameController = TextEditingController(text: profile.displayName);
    _bioController = TextEditingController(text: profile.bio ?? '');
    _locationController = TextEditingController(text: profile.location ?? '');
    _websiteController = TextEditingController(text: profile.websiteUrl ?? '');
    _initialized = true;
  }

  void _submit() {
    FocusScope.of(context).unfocus();
    if (!_formKey.currentState!.validate()) return;

    context.read<ProfileBloc>().add(
      ProfileUpdateRequested(
        displayName: _displayNameController.text.trim(),
        bio: _bioController.text.trim().isEmpty
            ? null
            : _bioController.text.trim(),
        location: _locationController.text.trim().isEmpty
            ? null
            : _locationController.text.trim(),
        websiteUrl: _websiteController.text.trim().isEmpty
            ? null
            : _websiteController.text.trim(),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Edit profile'),
        leading: IconButton(
          icon: const Icon(Icons.close),
          onPressed: () => context.pop(),
        ),
      ),
      body: BlocConsumer<ProfileBloc, ProfileState>(
        listener: (context, state) {
          if (state is EditProfileSuccess) {
            context.pop();
          }
          if (state is EditProfileError) {
            ScaffoldMessenger.of(context)
              ..hideCurrentSnackBar()
              ..showSnackBar(
                SnackBar(
                  content: Text(state.failure.message),
                  backgroundColor: Theme.of(context).colorScheme.error,
                ),
              );
          }
        },
        builder: (context, state) {
          final profile = switch (state) {
            ProfileLoaded(:final profile) when profile is OwnProfile => profile,
            EditProfileSubmitting(:final current) => current,
            EditProfileError(:final current) => current,
            EditProfileSuccess(:final updated) => updated,
            _ => null,
          };

          if (profile == null) {
            return const Center(child: CircularProgressIndicator());
          }

          _initialize(profile);

          final isSubmitting = state is EditProfileSubmitting;

          return SingleChildScrollView(
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 24),
            child: Form(
              key: _formKey,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  // Display name field
                  TextFormField(
                    controller: _displayNameController,
                    enabled: !isSubmitting,
                    textInputAction: TextInputAction.next,
                    textCapitalization: TextCapitalization.words,
                    decoration: const InputDecoration(
                      labelText: 'Display name',
                    ),
                    validator: (v) {
                      if (v == null || v.trim().isEmpty) {
                        return 'Display name is required.';
                      }
                      return null;
                    },
                  ),
                  const SizedBox(height: 16),

                  // Bio field
                  TextFormField(
                    controller: _bioController,
                    enabled: !isSubmitting,
                    maxLines: 4,
                    maxLength: 280,
                    textInputAction: TextInputAction.newline,
                    decoration: const InputDecoration(labelText: 'Bio'),
                  ),
                  const SizedBox(height: 16),

                  // Location field
                  TextFormField(
                    controller: _locationController,
                    enabled: !isSubmitting,
                    textInputAction: TextInputAction.next,
                    decoration: const InputDecoration(labelText: 'Location'),
                  ),
                  const SizedBox(height: 16),

                  // Website URL field
                  TextFormField(
                    controller: _websiteController,
                    enabled: !isSubmitting,
                    keyboardType: TextInputType.url,
                    textInputAction: TextInputAction.done,
                    autocorrect: false,
                    decoration: const InputDecoration(labelText: 'Website URL'),
                    validator: (v) {
                      if (v != null && v.isNotEmpty) {
                        final uri = Uri.tryParse(v.trim());
                        if (uri == null ||
                            (!uri.hasScheme ||
                                (!v.startsWith('http://') &&
                                    !v.startsWith('https://')))) {
                          return 'Enter a valid URL starting with http:// or https://';
                        }
                      }
                      return null;
                    },
                    onFieldSubmitted: (_) => _submit(),
                  ),
                  const SizedBox(height: 32),

                  // Save button
                  ElevatedButton(
                    onPressed: isSubmitting ? null : _submit,
                    child: isSubmitting
                        ? const SizedBox(
                            height: 20,
                            width: 20,
                            child: CircularProgressIndicator(
                              strokeWidth: 2,
                              color: Colors.white,
                            ),
                          )
                        : const Text('Save'),
                  ),
                ],
              ),
            ),
          );
        },
      ),
    );
  }
}
