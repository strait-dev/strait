package notification

import (
	"context"

	"strait/internal/domain"
)

// ChannelSender sends a notification delivery to a specific channel type.
type ChannelSender interface {
	Send(ctx context.Context, channel *domain.NotificationChannel, delivery *domain.NotificationDelivery) error
}
