# Dzeroth Frontend Rules

## UX Goal

Dzeroth should provide a polished, familiar X/Twitter-like social-network
experience while remaining its own implementation and visual identity.

Expected patterns include:

- home timeline
- profile
- post composer
- reply/thread view
- quote post
- repost/share
- notifications
- search/discovery
- settings
- responsive navigation

Do not copy proprietary X assets, source code, or private APIs.

## BLoC

Use explicit:

```text
Event
State
Bloc
```

States should clearly represent meaningful lifecycle conditions such as:

- initial
- loading
- loaded
- empty
- error
- terminated
- submitting
- countdown
- unauthorized

Do not create unnecessary state variants.

## UI States

Every major screen should handle:

- loading
- success
- empty
- error
- retry
- unauthorized where applicable
- network failure where applicable

## Finite Feed

Never implement infinite scrolling.

When the backend indicates termination:

- stop requesting
- show the boundary state
- show the appropriate Dzeroth termination experience

## Share / Quote

The five-second countdown must be visible.

Do not create a client path that bypasses the rule.

Text quote validation must enforce the five-distinct-word requirement before
submission, while backend enforcement remains authoritative.

## Public Metrics

Do not display public:

- likes
- impressions
- bookmark counts
- follower counts
- equivalent popularity metrics

Do not accidentally render fields merely because an API model contains them.

## Models

Use explicit DTO → domain mapping.

Do not pass arbitrary maps through the application.

## Widgets

Avoid business logic inside widgets.

Reuse shared components.

Before creating a widget, search for an existing equivalent.

## Accessibility

Support where applicable:

- semantic labels
- adequate touch targets
- keyboard navigation
- screen-reader usability
- meaningful focus order
- readable error states

## Validation

Run relevant:

- `dart format`
- `flutter analyze`
- tests
- build validation
