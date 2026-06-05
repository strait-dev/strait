package worker

import (
	"context"
	"time"
)

type HeartbeatStore interface {
	UpdateHeartbeat(ctx context.Context, id string) error
	BatchUpdateHeartbeat(ctx context.Context, ids []string) error
}

type heartbeatLegacyStore interface {
	UpdateHeartbeat(ctx context.Context, id string) error
}

type heartbeatSideTableStore interface {
	UpsertHeartbeatSideTable(ctx context.Context, id string) error
	BatchUpsertHeartbeatSideTable(ctx context.Context, ids []string) error
}

type heartbeatStoreAdapter struct {
	store heartbeatLegacyStore
}

func (a heartbeatStoreAdapter) UpdateHeartbeat(ctx context.Context, id string) error {
	return a.store.UpdateHeartbeat(ctx, id)
}

func (a heartbeatStoreAdapter) BatchUpdateHeartbeat(ctx context.Context, ids []string) error {
	for _, id := range ids {
		if err := a.store.UpdateHeartbeat(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

type heartbeatSideTableAdapter struct {
	store heartbeatSideTableStore
}

func (a heartbeatSideTableAdapter) UpdateHeartbeat(ctx context.Context, id string) error {
	return a.store.UpsertHeartbeatSideTable(ctx, id)
}

func (a heartbeatSideTableAdapter) BatchUpdateHeartbeat(ctx context.Context, ids []string) error {
	return a.store.BatchUpsertHeartbeatSideTable(ctx, ids)
}

func NewHeartbeatSender(s heartbeatLegacyStore, interval time.Duration) *HeartbeatManager {
	if store, ok := s.(heartbeatSideTableStore); ok {
		return NewHeartbeatManager(heartbeatSideTableAdapter{store: store}, interval)
	}
	if store, ok := s.(HeartbeatStore); ok {
		return NewHeartbeatManager(store, interval)
	}
	return NewHeartbeatManager(heartbeatStoreAdapter{store: s}, interval)
}
