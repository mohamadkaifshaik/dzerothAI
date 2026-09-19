import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:go_router/go_router.dart';

import '../../../post/domain/entities/post.dart';
import '../../../post/presentation/widgets/go_touch_grass_widget.dart';
import '../../../post/presentation/widgets/post_card.dart';
import '../../domain/entities/user_search_result.dart';
import '../bloc/search_bloc.dart';

/// The Dzeroth search screen.
///
/// Presents two tabs — "Posts" and "Users" — with a shared search bar at the
/// top. Input triggers [SearchQueryChanged] which the BLoC debounces by 300 ms.
///
/// PUBLIC METRICS LOCKDOWN: Zero social-validation metrics are displayed
/// anywhere on this screen per CLAUDE.md §2.3. No like counts, follower
/// counts, impression counts, bookmark counts, or equivalent fields.
///
/// FINITE FEED: No infinite scrolling. A "Load more" button advances the
/// cursor. When the server returns terminated:true, [GoTouchGrassWidget] is
/// shown and no further requests are made per CLAUDE.md §2.1.
///
/// Auth is optional — this screen functions for unauthenticated users.
class SearchScreen extends StatefulWidget {
  const SearchScreen({super.key});

  @override
  State<SearchScreen> createState() => _SearchScreenState();
}

class _SearchScreenState extends State<SearchScreen>
    with SingleTickerProviderStateMixin {
  late final TabController _tabController;
  final TextEditingController _queryController = TextEditingController();

  /// The last query that was sent to the BLoC.
  String _lastQuery = '';

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: 2, vsync: this);
    _tabController.addListener(_onTabChanged);
  }

  @override
  void dispose() {
    _tabController.removeListener(_onTabChanged);
    _tabController.dispose();
    _queryController.dispose();
    super.dispose();
  }

  void _onTabChanged() {
    if (_tabController.indexIsChanging) return;

    // When switching to the Users tab with a non-empty query, automatically
    // request user results if the BLoC is not already holding user state.
    if (_tabController.index == 1 && _lastQuery.isNotEmpty) {
      final current = context.read<SearchBloc>().state;
      if (current is! SearchUsersLoaded && current is! SearchUsersTerminated) {
        _requestUsersForCurrentQuery();
      }
    }
  }

  void _requestUsersForCurrentQuery() {
    if (_lastQuery.isEmpty) return;
    context.read<SearchBloc>().add(SearchUsersRequested(query: _lastQuery));
  }

  void _onSearchSubmitted(String value) {
    _lastQuery = value.trim();
    context.read<SearchBloc>().add(SearchQueryChanged(query: value));
  }

  void _onSearchChanged(String value) {
    _lastQuery = value.trim();
    context.read<SearchBloc>().add(SearchQueryChanged(query: value));
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: _SearchBar(
          controller: _queryController,
          onChanged: _onSearchChanged,
          onSubmitted: _onSearchSubmitted,
        ),
        bottom: TabBar(
          controller: _tabController,
          tabs: const [
            Tab(text: 'Posts'),
            Tab(text: 'Users'),
          ],
        ),
      ),
      body: BlocBuilder<SearchBloc, SearchState>(
        builder: (context, state) {
          return TabBarView(
            controller: _tabController,
            children: [
              _PostsTabContent(state: state),
              _UsersTabContent(
                state: state,
                onRequestUsers: _requestUsersForCurrentQuery,
                lastQuery: _lastQuery,
              ),
            ],
          );
        },
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Search bar
// ---------------------------------------------------------------------------

class _SearchBar extends StatelessWidget {
  const _SearchBar({
    required this.controller,
    required this.onChanged,
    required this.onSubmitted,
  });

  final TextEditingController controller;
  final ValueChanged<String> onChanged;
  final ValueChanged<String> onSubmitted;

  @override
  Widget build(BuildContext context) {
    final colorScheme = Theme.of(context).colorScheme;
    return TextField(
      controller: controller,
      onChanged: onChanged,
      onSubmitted: onSubmitted,
      textInputAction: TextInputAction.search,
      decoration: InputDecoration(
        hintText: 'Search Dzeroth',
        prefixIcon: const Icon(Icons.search),
        suffixIcon: ValueListenableBuilder<TextEditingValue>(
          valueListenable: controller,
          builder: (context, value, _) {
            if (value.text.isEmpty) return const SizedBox.shrink();
            return IconButton(
              icon: const Icon(Icons.clear),
              tooltip: 'Clear search',
              onPressed: () {
                controller.clear();
                onChanged('');
              },
            );
          },
        ),
        filled: true,
        fillColor: colorScheme.surfaceContainerHighest,
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(24),
          borderSide: BorderSide.none,
        ),
        contentPadding: const EdgeInsets.symmetric(vertical: 0),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Posts tab
// ---------------------------------------------------------------------------

class _PostsTabContent extends StatelessWidget {
  const _PostsTabContent({required this.state});

  final SearchState state;

  @override
  Widget build(BuildContext context) {
    return switch (state) {
      SearchInitial() => _buildInitialHint(context),
      SearchLoading() => _buildLoading(),
      SearchPostsLoaded(:final posts, :final hasMore) => _buildPostsList(
        context,
        posts: posts,
        hasMore: hasMore,
        terminated: false,
      ),
      SearchPostsTerminated(:final posts) => _buildPostsList(
        context,
        posts: posts,
        hasMore: false,
        terminated: true,
      ),
      SearchEmpty(:final query, :final searchType) =>
        searchType == 'posts'
            ? _buildEmpty(context, query)
            : _buildInitialHint(context),
      SearchError(:final message) => _buildError(context, message),
      // User states: show hint to switch tabs or re-query.
      SearchUsersLoaded() ||
      SearchUsersTerminated() => _buildInitialHint(context),
    };
  }

  Widget _buildInitialHint(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.search,
              size: 64,
              color: Theme.of(context).colorScheme.onSurfaceVariant,
            ),
            const SizedBox(height: 16),
            Text(
              'Search for posts',
              style: Theme.of(context).textTheme.titleMedium,
            ),
            const SizedBox(height: 8),
            Text(
              'Type in the search bar above to find posts.',
              style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                color: Theme.of(context).colorScheme.onSurfaceVariant,
              ),
              textAlign: TextAlign.center,
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildLoading() {
    return const Center(child: CircularProgressIndicator());
  }

  Widget _buildEmpty(BuildContext context, String query) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.search_off,
              size: 64,
              color: Theme.of(context).colorScheme.onSurfaceVariant,
            ),
            const SizedBox(height: 16),
            Text(
              'No posts found',
              style: Theme.of(context).textTheme.titleMedium,
            ),
            const SizedBox(height: 8),
            Text(
              'No results for "$query".',
              style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                color: Theme.of(context).colorScheme.onSurfaceVariant,
              ),
              textAlign: TextAlign.center,
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildError(BuildContext context, String message) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.error_outline,
              size: 48,
              color: Theme.of(context).colorScheme.error,
            ),
            const SizedBox(height: 16),
            Text(
              message,
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.bodyLarge,
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildPostsList(
    BuildContext context, {
    required List<Post> posts,
    required bool hasMore,
    required bool terminated,
  }) {
    return ListView.separated(
      itemCount: posts.length + 1,
      separatorBuilder: (_, _) => const Divider(height: 1),
      itemBuilder: (context, index) {
        if (index < posts.length) {
          return PostCard(post: posts[index]);
        }

        if (terminated) {
          return const GoTouchGrassWidget();
        }

        return _LoadMoreButton(
          hasMore: hasMore,
          onPressed: () => context.read<SearchBloc>().add(
            const SearchPostsNextPageRequested(),
          ),
        );
      },
    );
  }
}

// ---------------------------------------------------------------------------
// Users tab
// ---------------------------------------------------------------------------

class _UsersTabContent extends StatelessWidget {
  const _UsersTabContent({
    required this.state,
    required this.onRequestUsers,
    required this.lastQuery,
  });

  final SearchState state;
  final VoidCallback onRequestUsers;
  final String lastQuery;

  @override
  Widget build(BuildContext context) {
    return switch (state) {
      SearchInitial() => _buildInitialHint(context),
      SearchLoading() => const Center(child: CircularProgressIndicator()),
      SearchUsersLoaded(:final users, :final hasMore) => _buildUsersList(
        context,
        users: users,
        hasMore: hasMore,
        terminated: false,
      ),
      SearchUsersTerminated(:final users) => _buildUsersList(
        context,
        users: users,
        hasMore: false,
        terminated: true,
      ),
      SearchEmpty(:final query, :final searchType) =>
        searchType == 'users'
            ? _buildEmpty(context, query)
            : _buildSwitchPrompt(context),
      SearchError(:final message) => _buildError(context, message),
      // Post states: if we have a query in flight, show loading; otherwise
      // prompt the user to request user results.
      SearchPostsLoaded() ||
      SearchPostsTerminated() => _buildSwitchPrompt(context),
    };
  }

  Widget _buildInitialHint(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.person_search_outlined,
              size: 64,
              color: Theme.of(context).colorScheme.onSurfaceVariant,
            ),
            const SizedBox(height: 16),
            Text(
              'Search for people',
              style: Theme.of(context).textTheme.titleMedium,
            ),
            const SizedBox(height: 8),
            Text(
              'Type in the search bar above to find users.',
              style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                color: Theme.of(context).colorScheme.onSurfaceVariant,
              ),
              textAlign: TextAlign.center,
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildSwitchPrompt(BuildContext context) {
    if (lastQuery.isEmpty) return _buildInitialHint(context);
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.person_search_outlined,
              size: 64,
              color: Theme.of(context).colorScheme.onSurfaceVariant,
            ),
            const SizedBox(height: 16),
            Text(
              'Search for people',
              style: Theme.of(context).textTheme.titleMedium,
            ),
            const SizedBox(height: 8),
            Text(
              'Find users matching "$lastQuery".',
              style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                color: Theme.of(context).colorScheme.onSurfaceVariant,
              ),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 24),
            FilledButton.icon(
              onPressed: onRequestUsers,
              icon: const Icon(Icons.search),
              label: const Text('Search users'),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildEmpty(BuildContext context, String query) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.person_off_outlined,
              size: 64,
              color: Theme.of(context).colorScheme.onSurfaceVariant,
            ),
            const SizedBox(height: 16),
            Text(
              'No users found',
              style: Theme.of(context).textTheme.titleMedium,
            ),
            const SizedBox(height: 8),
            Text(
              'No results for "$query".',
              style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                color: Theme.of(context).colorScheme.onSurfaceVariant,
              ),
              textAlign: TextAlign.center,
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildError(BuildContext context, String message) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.error_outline,
              size: 48,
              color: Theme.of(context).colorScheme.error,
            ),
            const SizedBox(height: 16),
            Text(
              message,
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.bodyLarge,
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildUsersList(
    BuildContext context, {
    required List<UserSearchResult> users,
    required bool hasMore,
    required bool terminated,
  }) {
    return ListView.separated(
      itemCount: users.length + 1,
      separatorBuilder: (_, _) => const Divider(height: 1),
      itemBuilder: (context, index) {
        if (index < users.length) {
          return _UserResultTile(user: users[index]);
        }

        if (terminated) {
          return const GoTouchGrassWidget();
        }

        return _LoadMoreButton(
          hasMore: hasMore,
          onPressed: () => context.read<SearchBloc>().add(
            const SearchUsersNextPageRequested(),
          ),
        );
      },
    );
  }
}

// ---------------------------------------------------------------------------
// User result tile
// ---------------------------------------------------------------------------

/// A list tile for a single user search result.
///
/// PUBLIC METRICS LOCKDOWN: This tile intentionally shows zero social-
/// validation metrics per CLAUDE.md §2.3. Only handle, display name, and
/// avatar are rendered. No follower count, following count, or any equivalent
/// field is displayed.
class _UserResultTile extends StatelessWidget {
  const _UserResultTile({required this.user});

  final UserSearchResult user;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final colorScheme = theme.colorScheme;
    final url = user.avatarUrl;

    return InkWell(
      onTap: () => context.push('/users/${user.id}'),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        child: Row(
          children: [
            CircleAvatar(
              radius: 22,
              backgroundImage: url != null ? NetworkImage(url) : null,
              child: url == null
                  ? Text(
                      user.displayName.isNotEmpty
                          ? user.displayName[0].toUpperCase()
                          : '?',
                      style: const TextStyle(fontWeight: FontWeight.bold),
                    )
                  : null,
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    user.displayName,
                    style: theme.textTheme.titleSmall?.copyWith(
                      fontWeight: FontWeight.w600,
                    ),
                    overflow: TextOverflow.ellipsis,
                  ),
                  Text(
                    '@${user.handle}',
                    style: theme.textTheme.bodySmall?.copyWith(
                      color: colorScheme.onSurfaceVariant,
                    ),
                    overflow: TextOverflow.ellipsis,
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Load more button
// ---------------------------------------------------------------------------

class _LoadMoreButton extends StatelessWidget {
  const _LoadMoreButton({required this.hasMore, required this.onPressed});

  final bool hasMore;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    if (!hasMore) {
      return const Padding(
        padding: EdgeInsets.symmetric(vertical: 16),
        child: Center(child: CircularProgressIndicator()),
      );
    }

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 16),
      child: Center(
        child: OutlinedButton(
          onPressed: onPressed,
          child: const Text('Load more'),
        ),
      ),
    );
  }
}
