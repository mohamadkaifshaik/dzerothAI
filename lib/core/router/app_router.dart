import 'dart:async';

import 'package:flutter/widgets.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:go_router/go_router.dart';

import '../../features/auth/presentation/bloc/auth_bloc.dart';
import '../../features/auth/presentation/screens/login_screen.dart';
import '../../features/auth/presentation/screens/register_screen.dart';
import '../../features/post/domain/repositories/post_repository.dart';
import '../../features/post/presentation/bloc/post_compose_bloc.dart';
import '../../features/post/presentation/bloc/post_detail_bloc.dart';
import '../../features/post/presentation/bloc/post_feed_bloc.dart';
import '../../features/post/presentation/screens/post_compose_screen.dart';
import '../../features/post/presentation/screens/post_detail_screen.dart';
import '../../features/profile/domain/repositories/profile_repository.dart';
import '../../features/profile/presentation/bloc/profile_bloc.dart';
import '../../features/profile/presentation/screens/edit_profile_screen.dart';
import '../../features/profile/presentation/screens/profile_screen.dart';
import '../../features/shell/presentation/screens/app_shell.dart';

/// A [ChangeNotifier] that listens to an [AuthBloc] stream and notifies
/// go_router's [refreshListenable] when auth state changes.
class GoRouterRefreshStream extends ChangeNotifier {
  GoRouterRefreshStream(Stream<AuthState> stream) {
    _subscription = stream.listen((_) => notifyListeners());
  }

  late final StreamSubscription<AuthState> _subscription;

  @override
  void dispose() {
    _subscription.cancel();
    super.dispose();
  }
}

/// Builds and returns the application [GoRouter].
///
/// [authBloc] is required to wire the auth guard and [refreshListenable].
/// [profileRepositoryFactory] creates [ProfileRepository] instances for
/// screens that need them.
GoRouter createAppRouter({
  required AuthBloc authBloc,
  required ProfileRepository Function() profileRepositoryFactory,
  required PostRepository Function() postRepositoryFactory,
}) {
  final refreshStream = GoRouterRefreshStream(authBloc.stream);

  return GoRouter(
    refreshListenable: refreshStream,
    initialLocation: '/home/feed',
    redirect: (context, state) {
      final authState = authBloc.state;
      final isAuthRoute = state.matchedLocation.startsWith('/auth');
      final isInitial = authState is AuthInitial || authState is AuthLoading;

      // While checking auth status on startup, show nothing (stay put).
      if (isInitial) return null;

      final isAuthenticated = authState is AuthAuthenticated;

      if (!isAuthenticated && !isAuthRoute) {
        return '/auth/login';
      }
      if (isAuthenticated && isAuthRoute) {
        return '/home/feed';
      }
      return null;
    },

    routes: [
      // Auth routes (unauthenticated)
      GoRoute(
        path: '/auth/login',
        builder: (context, state) => const LoginScreen(),
      ),
      GoRoute(
        path: '/auth/register',
        builder: (context, state) => const RegisterScreen(),
      ),

      // Authenticated shell
      ShellRoute(
        builder: (context, state, child) => AppShell(child: child),
        routes: [
          GoRoute(
            path: '/home/feed',
            builder: (context, state) => const FeedPlaceholderScreen(),
          ),
          GoRoute(
            path: '/home/search',
            builder: (context, state) => const SearchPlaceholderScreen(),
          ),
          GoRoute(
            path: '/home/notifications',
            builder: (context, state) => const NotificationsPlaceholderScreen(),
          ),
          // /home/profile redirects to the current user's profile page.
          GoRoute(
            path: '/home/profile',
            redirect: (context, state) {
              final authState = authBloc.state;
              if (authState is AuthAuthenticated) {
                return '/users/${authState.userId}';
              }
              return '/auth/login';
            },
          ),
        ],
      ),

      // User profile (outside shell so it can be pushed as a full page)
      GoRoute(
        path: '/users/:id',
        builder: (context, state) {
          final userId = state.pathParameters['id']!;
          final authState = authBloc.state;
          final isOwn =
              authState is AuthAuthenticated && authState.userId == userId;

          return BlocProvider(
            create: (_) =>
                ProfileBloc(profileRepository: profileRepositoryFactory())..add(
                  isOwn
                      ? const OwnProfileLoadRequested()
                      : ProfileLoadRequested(userId: userId),
                ),
            child: ProfileScreen(userId: userId, isOwnProfile: isOwn),
          );
        },
        routes: [
          GoRoute(
            path: 'edit',
            builder: (context, state) {
              final userId = state.pathParameters['id']!;
              final authState = authBloc.state;

              // Guard: only own profile can be edited.
              if (authState is! AuthAuthenticated ||
                  authState.userId != userId) {
                // Redirect handled via redirect callback below.
                return const SizedBox.shrink();
              }

              // Reuse the parent route's ProfileBloc via context.
              return const EditProfileScreen();
            },
            redirect: (context, state) {
              final userId = state.pathParameters['id']!;
              final authState = authBloc.state;
              if (authState is! AuthAuthenticated ||
                  authState.userId != userId) {
                return '/users/$userId';
              }
              return null;
            },
          ),
        ],
      ),

      // Settings (outside shell)
      GoRoute(
        path: '/settings',
        builder: (context, state) => const SettingsPlaceholderScreen(),
      ),

      // Post compose — requires authentication; outside shell (full-page).
      GoRoute(
        path: '/posts/new',
        redirect: (context, state) {
          final authState = authBloc.state;
          if (authState is! AuthAuthenticated) return '/auth/login';
          return null;
        },
        builder: (context, state) {
          final postType = state.uri.queryParameters['type'] ?? 'original';
          final parentId = state.uri.queryParameters['parent_id'];
          final quotedPostId = state.uri.queryParameters['quoted_post_id'];
          return BlocProvider(
            create: (_) =>
                PostComposeBloc(postRepository: postRepositoryFactory()),
            child: PostComposeScreen(
              postType: postType,
              parentId: parentId,
              quotedPostId: quotedPostId,
            ),
          );
        },
      ),

      // Post detail — public; outside shell (full-page).
      GoRoute(
        path: '/posts/:postId',
        builder: (context, state) {
          final postId = state.pathParameters['postId']!;
          final postRepository = postRepositoryFactory();
          return MultiBlocProvider(
            providers: [
              BlocProvider(
                create: (_) =>
                    PostDetailBloc(postRepository: postRepository)
                      ..add(PostDetailLoadRequested(postId: postId)),
              ),
              BlocProvider(
                create: (_) => PostFeedBloc(postRepository: postRepository)
                  ..add(
                    PostFeedLoadRequested(
                      subjectId: postId,
                      feedType: FeedType.threadReplies,
                    ),
                  ),
              ),
            ],
            child: PostDetailScreen(postId: postId),
          );
        },
      ),
    ],
  );
}
