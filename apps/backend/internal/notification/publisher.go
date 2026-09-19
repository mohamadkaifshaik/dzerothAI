package notification

import "context"

// NotificationPublisher is the interface used by other packages (reaction,
// follow, post) to emit notification events without importing each other or
// this package's concrete types. The interface lives here so that consumers
// import internal/notification and use this interface — no circular dependency
// is introduced because the concrete implementation (Service) also lives here.
type NotificationPublisher interface {
	Publish(ctx context.Context, event PublishEvent) error
}
