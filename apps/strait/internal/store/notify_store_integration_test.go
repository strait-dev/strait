//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestNotifySubscriberUpsertAndLookup(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	sub := &domain.NotifySubscriber{
		ProjectID:  "project-notify-subscribers",
		ExternalID: "user_123",
		Email:      "alice@example.com",
		Attributes: []byte(`{"name":"Alice"}`),
	}
	if err := q.UpsertNotifySubscriber(ctx, sub); err != nil {
		t.Fatalf("UpsertNotifySubscriber() error = %v", err)
	}
	if sub.ID == "" {
		t.Fatal("UpsertNotifySubscriber() did not set ID")
	}

	got, err := q.GetNotifySubscriberByExternalID(ctx, sub.ProjectID, sub.ExternalID)
	if err != nil {
		t.Fatalf("GetNotifySubscriberByExternalID() error = %v", err)
	}
	if got.Email != sub.Email {
		t.Fatalf("Email = %q, want %q", got.Email, sub.Email)
	}
	if got.Locale != "en" {
		t.Fatalf("Locale = %q, want en", got.Locale)
	}

	sub.Email = "alice+new@example.com"
	sub.Status = domain.NotifySubscriberStatusUnsubscribed
	if err := q.UpsertNotifySubscriber(ctx, sub); err != nil {
		t.Fatalf("UpsertNotifySubscriber(update) error = %v", err)
	}

	updated, err := q.GetNotifySubscriber(ctx, sub.ID, sub.ProjectID)
	if err != nil {
		t.Fatalf("GetNotifySubscriber() error = %v", err)
	}
	if updated.Email != "alice+new@example.com" {
		t.Fatalf("updated Email = %q, want alice+new@example.com", updated.Email)
	}
	if updated.Status != domain.NotifySubscriberStatusUnsubscribed {
		t.Fatalf("updated Status = %q, want %q", updated.Status, domain.NotifySubscriberStatusUnsubscribed)
	}
}

func TestNotifyTopicAndMembership(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	sub := &domain.NotifySubscriber{
		ProjectID:  "project-notify-topics",
		ExternalID: "user_topic_1",
		Email:      "topic@example.com",
	}
	if err := q.UpsertNotifySubscriber(ctx, sub); err != nil {
		t.Fatalf("UpsertNotifySubscriber() error = %v", err)
	}

	topic := &domain.NotifyTopic{
		ProjectID: "project-notify-topics",
		TopicKey:  "project-updates",
		Name:      "Project Updates",
	}
	if err := q.CreateNotifyTopic(ctx, topic); err != nil {
		t.Fatalf("CreateNotifyTopic() error = %v", err)
	}

	if err := q.AddNotifyTopicSubscriber(ctx, topic.ID, sub.ID); err != nil {
		t.Fatalf("AddNotifyTopicSubscriber() error = %v", err)
	}
	if err := q.RemoveNotifyTopicSubscriber(ctx, topic.ID, sub.ID); err != nil {
		t.Fatalf("RemoveNotifyTopicSubscriber() error = %v", err)
	}

	listed, err := q.ListNotifyTopics(ctx, topic.ProjectID)
	if err != nil {
		t.Fatalf("ListNotifyTopics() error = %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("ListNotifyTopics() len = %d, want 1", len(listed))
	}
}

func TestNotificationTemplateAndCategory(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	cat := &domain.NotificationCategory{
		ProjectID:   "project-notify-templates",
		CategoryKey: "product-updates",
		Name:        "Product Updates",
	}
	if err := q.CreateNotificationCategory(ctx, cat); err != nil {
		t.Fatalf("CreateNotificationCategory() error = %v", err)
	}

	tmpl := &domain.NotificationTemplate{
		ProjectID:   "project-notify-templates",
		TemplateKey: "welcome",
		Name:        "Welcome",
		Channels:    []byte(`{"inbox":{"title":"Welcome {{subscriber.name}}"}}`),
	}
	if err := q.CreateNotificationTemplate(ctx, tmpl); err != nil {
		t.Fatalf("CreateNotificationTemplate() error = %v", err)
	}

	latest, err := q.GetLatestNotificationTemplateByKey(ctx, tmpl.ProjectID, tmpl.TemplateKey)
	if err != nil {
		t.Fatalf("GetLatestNotificationTemplateByKey() error = %v", err)
	}
	if latest.ID != tmpl.ID {
		t.Fatalf("latest.ID = %q, want %q", latest.ID, tmpl.ID)
	}

	cats, err := q.ListNotificationCategories(ctx, cat.ProjectID)
	if err != nil {
		t.Fatalf("ListNotificationCategories() error = %v", err)
	}
	if len(cats) != 1 {
		t.Fatalf("ListNotificationCategories() len = %d, want 1", len(cats))
	}
}

func TestNotificationPreferenceAndInboxLifecycle(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	pref := &domain.NotificationPreference{
		RecipientType:    domain.NotifyRecipientTypeSubscriber,
		RecipientID:      "sub_pref_1",
		Scope:            "global",
		ChannelPrefs:     []byte(`{"email":true,"inbox":true}`),
		CriticalOverride: true,
	}
	if err := q.UpsertNotificationPreference(ctx, pref); err != nil {
		t.Fatalf("UpsertNotificationPreference() error = %v", err)
	}

	fetchedPref, err := q.GetNotificationPreference(ctx, pref.RecipientType, pref.RecipientID, pref.Scope)
	if err != nil {
		t.Fatalf("GetNotificationPreference() error = %v", err)
	}
	if fetchedPref.ID == "" {
		t.Fatal("GetNotificationPreference() returned empty ID")
	}

	item := &domain.InboxItem{
		RecipientType: pref.RecipientType,
		RecipientID:   pref.RecipientID,
		ProjectID:     "project-notify-inbox",
		Title:         "Your export is ready",
		Body:          "Download is available",
		Actions:       []byte(`[{"label":"View","type":"link","url":"https://example.com"}]`),
	}
	if err := q.CreateInboxItem(ctx, item); err != nil {
		t.Fatalf("CreateInboxItem() error = %v", err)
	}

	now := time.Now().UTC()
	if err := q.UpdateInboxItemState(ctx, item.ID, item.RecipientType, item.RecipientID, domain.NotifyInboxStateRead, map[string]any{
		"read_at": now,
	}); err != nil {
		t.Fatalf("UpdateInboxItemState() error = %v", err)
	}

	updated, err := q.GetInboxItem(ctx, item.ID, item.RecipientType, item.RecipientID)
	if err != nil {
		t.Fatalf("GetInboxItem() error = %v", err)
	}
	if updated.State != domain.NotifyInboxStateRead {
		t.Fatalf("State = %q, want %q", updated.State, domain.NotifyInboxStateRead)
	}
	if updated.ReadAt == nil {
		t.Fatal("ReadAt is nil, want non-nil")
	}
}

func TestDisableNotificationChannelPreference_MergesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	pref := &domain.NotificationPreference{
		RecipientType:    domain.NotifyRecipientTypeSubscriber,
		RecipientID:      "sub_disable_merge",
		Scope:            "global",
		ChannelPrefs:     []byte(`{"email":true,"inbox":true,"sms":true}`),
		CriticalOverride: true,
	}
	if err := q.UpsertNotificationPreference(ctx, pref); err != nil {
		t.Fatalf("UpsertNotificationPreference() error = %v", err)
	}

	if err := q.DisableNotificationChannelPreference(ctx, pref.RecipientType, pref.RecipientID, pref.Scope, "email"); err != nil {
		t.Fatalf("DisableNotificationChannelPreference(first) error = %v", err)
	}
	if err := q.DisableNotificationChannelPreference(ctx, pref.RecipientType, pref.RecipientID, pref.Scope, "email"); err != nil {
		t.Fatalf("DisableNotificationChannelPreference(second) error = %v", err)
	}

	updated, err := q.GetNotificationPreference(ctx, pref.RecipientType, pref.RecipientID, pref.Scope)
	if err != nil {
		t.Fatalf("GetNotificationPreference() error = %v", err)
	}

	channelPrefs := map[string]bool{}
	if err := json.Unmarshal(updated.ChannelPrefs, &channelPrefs); err != nil {
		t.Fatalf("unmarshal channel prefs: %v", err)
	}
	if channelPrefs["email"] {
		t.Fatal("channel_prefs.email = true, want false")
	}
	if !channelPrefs["inbox"] {
		t.Fatal("channel_prefs.inbox = false, want true")
	}
	if !channelPrefs["sms"] {
		t.Fatal("channel_prefs.sms = false, want true")
	}
}

func TestDisableNotificationChannelPreference_ConcurrentCalls(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	const concurrentCalls = 10
	var (
		wg       sync.WaitGroup
		errMu    sync.Mutex
		firstErr error
	)

	for range concurrentCalls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := q.DisableNotificationChannelPreference(ctx, domain.NotifyRecipientTypeSubscriber, "sub_disable_concurrent", "global", "email")
			errMu.Lock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			errMu.Unlock()
		}()
	}
	wg.Wait()

	if firstErr != nil {
		t.Fatalf("DisableNotificationChannelPreference(concurrent) error = %v", firstErr)
	}

	updated, err := q.GetNotificationPreference(ctx, domain.NotifyRecipientTypeSubscriber, "sub_disable_concurrent", "global")
	if err != nil {
		t.Fatalf("GetNotificationPreference() error = %v", err)
	}
	channelPrefs := map[string]bool{}
	if err := json.Unmarshal(updated.ChannelPrefs, &channelPrefs); err != nil {
		t.Fatalf("unmarshal channel prefs: %v", err)
	}
	if channelPrefs["email"] {
		t.Fatal("channel_prefs.email = true, want false")
	}
}

func TestEnableNotificationChannelPreference_SetsChannelTrue(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	pref := &domain.NotificationPreference{
		RecipientType:    domain.NotifyRecipientTypeSubscriber,
		RecipientID:      "sub_enable_1",
		Scope:            "global",
		ChannelPrefs:     []byte(`{"email":false,"inbox":true}`),
		CriticalOverride: true,
	}
	if err := q.UpsertNotificationPreference(ctx, pref); err != nil {
		t.Fatalf("UpsertNotificationPreference() error = %v", err)
	}

	if err := q.EnableNotificationChannelPreference(ctx, pref.RecipientType, pref.RecipientID, pref.Scope, "email"); err != nil {
		t.Fatalf("EnableNotificationChannelPreference() error = %v", err)
	}

	updated, err := q.GetNotificationPreference(ctx, pref.RecipientType, pref.RecipientID, pref.Scope)
	if err != nil {
		t.Fatalf("GetNotificationPreference() error = %v", err)
	}

	channelPrefs := map[string]bool{}
	if err := json.Unmarshal(updated.ChannelPrefs, &channelPrefs); err != nil {
		t.Fatalf("unmarshal channel prefs: %v", err)
	}
	if !channelPrefs["email"] {
		t.Fatal("channel_prefs.email = false, want true")
	}
}

func TestNotifySuppressionEvents_CreateAndList(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	evt1 := &domain.NotifySuppressionEvent{
		ProjectID:     "project-notify-suppression-events",
		RecipientType: domain.NotifyRecipientTypeSubscriber,
		RecipientID:   "sub_evt_1",
		Scope:         "global",
		Channel:       "email",
		Action:        domain.NotifySuppressionActionSuppressed,
		Reason:        "provider_callback:email.bounced",
		Source:        domain.NotifySuppressionSourceProviderCallback,
		Metadata:      []byte(`{"message_id":"msg_1"}`),
	}
	if err := q.CreateNotifySuppressionEvent(ctx, evt1); err != nil {
		t.Fatalf("CreateNotifySuppressionEvent(evt1) error = %v", err)
	}

	evt2 := &domain.NotifySuppressionEvent{
		ProjectID:     evt1.ProjectID,
		RecipientType: domain.NotifyRecipientTypeSubscriber,
		RecipientID:   evt1.RecipientID,
		Scope:         "global",
		Channel:       "email",
		Action:        domain.NotifySuppressionActionUnsuppressed,
		Reason:        "manual_unsuppress",
		Source:        domain.NotifySuppressionSourceAdminAPI,
	}
	if err := q.CreateNotifySuppressionEvent(ctx, evt2); err != nil {
		t.Fatalf("CreateNotifySuppressionEvent(evt2) error = %v", err)
	}

	listed, err := q.ListNotifySuppressionEvents(ctx, evt1.ProjectID, evt1.RecipientType, evt1.RecipientID, 10, nil)
	if err != nil {
		t.Fatalf("ListNotifySuppressionEvents() error = %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("ListNotifySuppressionEvents() len = %d, want 2", len(listed))
	}

	latest, err := q.GetLatestNotifySuppressionEvent(ctx, evt1.ProjectID, evt1.RecipientType, evt1.RecipientID, "global", "email")
	if err != nil {
		t.Fatalf("GetLatestNotifySuppressionEvent() error = %v", err)
	}
	if latest.Action != domain.NotifySuppressionActionUnsuppressed {
		t.Fatalf("latest.Action = %q, want %q", latest.Action, domain.NotifySuppressionActionUnsuppressed)
	}
}

func TestNotificationMessageAndProviderLifecycle(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	rateLimit := 100
	provider := &domain.NotificationProvider{
		ProjectID: "project-notify-messages",
		Channel:   "email",
		Provider:  "sendgrid",
		Name:      "SendGrid Primary",
		ConfigEnc: []byte(`{"api_key":"test"}`),
		Health:    "healthy",
		RateLimit: &rateLimit,
	}
	if err := q.CreateNotificationProvider(ctx, provider); err != nil {
		t.Fatalf("CreateNotificationProvider() error = %v", err)
	}

	msg := &domain.NotificationMessage{
		ProjectID:       provider.ProjectID,
		RecipientType:   domain.NotifyRecipientTypeSubscriber,
		RecipientID:     "sub_msg_1",
		Channel:         "email",
		ProviderID:      provider.ID,
		RenderedContent: []byte(`{"subject":"Hello","html_body":"<p>Hi</p>"}`),
		Status:          domain.NotifyMessageStatusPending,
	}
	if err := q.CreateNotificationMessage(ctx, msg); err != nil {
		t.Fatalf("CreateNotificationMessage() error = %v", err)
	}

	deliveredAt := time.Now().UTC()
	if err := q.UpdateNotificationMessageStatus(ctx, msg.ID, msg.ProjectID, domain.NotifyMessageStatusPending, domain.NotifyMessageStatusDelivered, map[string]any{
		"delivered_at": deliveredAt,
		"attempts":     1,
	}); err != nil {
		t.Fatalf("UpdateNotificationMessageStatus() error = %v", err)
	}

	got, err := q.GetNotificationMessage(ctx, msg.ID, msg.ProjectID)
	if err != nil {
		t.Fatalf("GetNotificationMessage() error = %v", err)
	}
	if got.Status != domain.NotifyMessageStatusDelivered {
		t.Fatalf("Status = %q, want %q", got.Status, domain.NotifyMessageStatusDelivered)
	}
	if got.DeliveredAt == nil {
		t.Fatal("DeliveredAt is nil, want non-nil")
	}

	providers, err := q.ListNotificationProviders(ctx, provider.ProjectID, "email")
	if err != nil {
		t.Fatalf("ListNotificationProviders() error = %v", err)
	}
	if len(providers) != 1 {
		t.Fatalf("ListNotificationProviders() len = %d, want 1", len(providers))
	}
}

func TestClaimDueScheduledNotificationMessages(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-notify-claim"
	base := time.Now().UTC()
	dueAt := base.Add(-1 * time.Minute)
	futureAt := base.Add(10 * time.Minute)

	due := &domain.NotificationMessage{
		ProjectID:     projectID,
		RecipientType: domain.NotifyRecipientTypeSubscriber,
		RecipientID:   "sub_due",
		Channel:       "inbox",
		Status:        domain.NotifyMessageStatusScheduled,
		ScheduledAt:   &dueAt,
	}
	if err := q.CreateNotificationMessage(ctx, due); err != nil {
		t.Fatalf("CreateNotificationMessage(due) error = %v", err)
	}

	future := &domain.NotificationMessage{
		ProjectID:     projectID,
		RecipientType: domain.NotifyRecipientTypeSubscriber,
		RecipientID:   "sub_future",
		Channel:       "inbox",
		Status:        domain.NotifyMessageStatusScheduled,
		ScheduledAt:   &futureAt,
	}
	if err := q.CreateNotificationMessage(ctx, future); err != nil {
		t.Fatalf("CreateNotificationMessage(future) error = %v", err)
	}

	claimed, err := q.ClaimDueScheduledNotificationMessages(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimDueScheduledNotificationMessages() error = %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("ClaimDueScheduledNotificationMessages() len = %d, want 1", len(claimed))
	}
	if claimed[0].ID != due.ID {
		t.Fatalf("claimed ID = %q, want %q", claimed[0].ID, due.ID)
	}
	if claimed[0].Status != domain.NotifyMessageStatusProcessing {
		t.Fatalf("claimed status = %q, want %q", claimed[0].Status, domain.NotifyMessageStatusProcessing)
	}
	if claimed[0].Attempts != 1 {
		t.Fatalf("claimed attempts = %d, want 1", claimed[0].Attempts)
	}

	storedDue, err := q.GetNotificationMessage(ctx, due.ID, projectID)
	if err != nil {
		t.Fatalf("GetNotificationMessage(due) error = %v", err)
	}
	if storedDue.Status != domain.NotifyMessageStatusProcessing {
		t.Fatalf("stored due status = %q, want %q", storedDue.Status, domain.NotifyMessageStatusProcessing)
	}

	storedFuture, err := q.GetNotificationMessage(ctx, future.ID, projectID)
	if err != nil {
		t.Fatalf("GetNotificationMessage(future) error = %v", err)
	}
	if storedFuture.Status != domain.NotifyMessageStatusScheduled {
		t.Fatalf("stored future status = %q, want %q", storedFuture.Status, domain.NotifyMessageStatusScheduled)
	}
}

func TestEscalationStateLifecycle(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	next := time.Now().UTC().Add(-1 * time.Minute)
	state := &domain.EscalationState{
		ProjectID:        "project-notify-escalation",
		StepRunID:        "step_1",
		WorkflowRunID:    "wf_run_1",
		CurrentTier:      1,
		TotalTiers:       3,
		NextEscalationAt: &next,
		Status:           domain.NotifyEscalationStatusActive,
	}
	if err := q.UpsertEscalationState(ctx, state); err != nil {
		t.Fatalf("UpsertEscalationState() error = %v", err)
	}

	loaded, err := q.GetActiveEscalationStateByStepRun(ctx, state.ProjectID, state.StepRunID)
	if err != nil {
		t.Fatalf("GetActiveEscalationStateByStepRun() error = %v", err)
	}
	if loaded.ID != state.ID {
		t.Fatalf("loaded ID = %q, want %q", loaded.ID, state.ID)
	}

	claimed, err := q.ClaimDueEscalationStates(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimDueEscalationStates() error = %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("ClaimDueEscalationStates() len = %d, want 1", len(claimed))
	}
	if claimed[0].Status != domain.NotifyEscalationStatusProcessing {
		t.Fatalf("claimed status = %q, want %q", claimed[0].Status, domain.NotifyEscalationStatusProcessing)
	}

	nextActive := time.Now().UTC().Add(15 * time.Minute)
	if err := q.AdvanceEscalationState(ctx, state.ID, state.ProjectID, 2, &nextActive, domain.NotifyEscalationStatusActive); err != nil {
		t.Fatalf("AdvanceEscalationState() error = %v", err)
	}

	ackAt := time.Now().UTC()
	if err := q.AcknowledgeEscalationState(ctx, state.ID, state.ProjectID, "user_1", ackAt); err != nil {
		t.Fatalf("AcknowledgeEscalationState() error = %v", err)
	}

	if _, err := q.GetActiveEscalationStateByStepRun(ctx, state.ProjectID, state.StepRunID); !errors.Is(err, store.ErrEscalationStateNotFound) {
		t.Fatalf("GetActiveEscalationStateByStepRun() after ack error = %v, want ErrEscalationStateNotFound", err)
	}

	state2 := &domain.EscalationState{
		ProjectID:        state.ProjectID,
		StepRunID:        "step_2",
		WorkflowRunID:    "wf_run_2",
		CurrentTier:      1,
		TotalTiers:       3,
		NextEscalationAt: &next,
		Status:           domain.NotifyEscalationStatusActive,
	}
	if err := q.UpsertEscalationState(ctx, state2); err != nil {
		t.Fatalf("UpsertEscalationState(state2) error = %v", err)
	}
	if err := q.AcknowledgeActiveEscalationStateByStepRun(ctx, state2.StepRunID, "user_2", time.Now().UTC()); err != nil {
		t.Fatalf("AcknowledgeActiveEscalationStateByStepRun() error = %v", err)
	}
	if _, err := q.GetActiveEscalationStateByStepRun(ctx, state2.ProjectID, state2.StepRunID); !errors.Is(err, store.ErrEscalationStateNotFound) {
		t.Fatalf("GetActiveEscalationStateByStepRun() after ack by step error = %v, want ErrEscalationStateNotFound", err)
	}

	state3 := &domain.EscalationState{
		ProjectID:        state.ProjectID,
		StepRunID:        "step_3",
		WorkflowRunID:    "wf_run_3",
		CurrentTier:      1,
		TotalTiers:       3,
		NextEscalationAt: &next,
		Status:           domain.NotifyEscalationStatusActive,
	}
	if err := q.UpsertEscalationState(ctx, state3); err != nil {
		t.Fatalf("UpsertEscalationState(state3) error = %v", err)
	}
	if err := q.CompleteActiveEscalationStateByStepRun(ctx, state3.StepRunID, domain.NotifyEscalationStatusCompleted); err != nil {
		t.Fatalf("CompleteActiveEscalationStateByStepRun() error = %v", err)
	}
	if _, err := q.GetActiveEscalationStateByStepRun(ctx, state3.ProjectID, state3.StepRunID); !errors.Is(err, store.ErrEscalationStateNotFound) {
		t.Fatalf("GetActiveEscalationStateByStepRun() after complete by step error = %v, want ErrEscalationStateNotFound", err)
	}
}

func TestNotificationBatchLifecycle(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	batch := &domain.NotificationBatch{
		ProjectID:     "project-notify-batch",
		RecipientType: domain.NotifyRecipientTypeSubscriber,
		RecipientID:   "sub_batch_1",
		BatchKey:      "hourly:inbox:welcome:_",
		Channel:       "inbox",
		WindowEnd:     time.Now().UTC().Add(-1 * time.Minute),
	}
	if err := q.AppendNotificationBatchEvent(ctx, batch, []byte(`{"channel_payload":{"title":"A"}}`)); err != nil {
		t.Fatalf("AppendNotificationBatchEvent(first) error = %v", err)
	}
	if batch.EventCount != 1 {
		t.Fatalf("EventCount(first) = %d, want 1", batch.EventCount)
	}

	if err := q.AppendNotificationBatchEvent(ctx, batch, []byte(`{"channel_payload":{"title":"B"}}`)); err != nil {
		t.Fatalf("AppendNotificationBatchEvent(second) error = %v", err)
	}
	if batch.EventCount != 2 {
		t.Fatalf("EventCount(second) = %d, want 2", batch.EventCount)
	}

	claimed, err := q.ClaimDueNotificationBatches(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimDueNotificationBatches() error = %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("ClaimDueNotificationBatches() len = %d, want 1", len(claimed))
	}
	if claimed[0].Status != domain.NotifyBatchStatusProcessing {
		t.Fatalf("claimed status = %q, want %q", claimed[0].Status, domain.NotifyBatchStatusProcessing)
	}

	if err := q.MarkNotificationBatchSent(ctx, claimed[0].ID, claimed[0].ProjectID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkNotificationBatchSent() error = %v", err)
	}
}

func TestNotifyDedupAndUnsubscribeToken(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	allowed, err := q.TryNotifyDedupKey(ctx, "project-dedup", "key-1", time.Hour)
	if err != nil {
		t.Fatalf("TryNotifyDedupKey(first) error = %v", err)
	}
	if !allowed {
		t.Fatal("TryNotifyDedupKey(first) = false, want true")
	}

	allowed, err = q.TryNotifyDedupKey(ctx, "project-dedup", "key-1", time.Hour)
	if err != nil {
		t.Fatalf("TryNotifyDedupKey(second) error = %v", err)
	}
	if allowed {
		t.Fatal("TryNotifyDedupKey(second) = true, want false")
	}

	tok := &domain.UnsubscribeToken{
		ProjectID:    "project-dedup",
		SubscriberID: "sub_1",
		Scope:        "global",
		Token:        "tok_test_1",
		ExpiresAt:    time.Now().UTC().Add(time.Hour),
	}
	if err := q.CreateUnsubscribeToken(ctx, tok); err != nil {
		t.Fatalf("CreateUnsubscribeToken() error = %v", err)
	}

	loaded, err := q.GetUnsubscribeToken(ctx, tok.Token)
	if err != nil {
		t.Fatalf("GetUnsubscribeToken() error = %v", err)
	}
	if loaded.TokenHash == "" {
		t.Fatal("TokenHash is empty, want hashed token")
	}

	if err := q.UseUnsubscribeToken(ctx, tok.Token, time.Now().UTC()); err != nil {
		t.Fatalf("UseUnsubscribeToken() error = %v", err)
	}
	if _, err := q.GetUnsubscribeToken(ctx, tok.Token); !errors.Is(err, store.ErrUnsubscribeTokenNotFound) {
		t.Fatalf("GetUnsubscribeToken(after use) error = %v, want ErrUnsubscribeTokenNotFound", err)
	}
}

func TestNotifyPolicyOverrideLifecycle(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	retryAttempts := 6
	baseDelay := 30
	maxDelay := 300

	policy := &domain.NotifyPolicyOverride{
		ProjectID:          "project-notify-policy",
		ScopeType:          domain.NotifyPolicyScopeProject,
		ScopeKey:           "*",
		DigestPolicy:       "hourly",
		RetryMaxAttempts:   &retryAttempts,
		RetryBaseDelaySecs: &baseDelay,
		RetryMaxDelaySecs:  &maxDelay,
		Enabled:            true,
	}
	if err := q.UpsertNotifyPolicyOverride(ctx, policy); err != nil {
		t.Fatalf("UpsertNotifyPolicyOverride() error = %v", err)
	}
	if policy.ID == "" {
		t.Fatal("policy ID is empty")
	}

	loaded, err := q.GetNotifyPolicyOverride(ctx, policy.ID, policy.ProjectID)
	if err != nil {
		t.Fatalf("GetNotifyPolicyOverride() error = %v", err)
	}
	if loaded.DigestPolicy != "hourly" {
		t.Fatalf("DigestPolicy = %q, want hourly", loaded.DigestPolicy)
	}

	list, err := q.ListNotifyPolicyOverrides(ctx, policy.ProjectID, nil)
	if err != nil {
		t.Fatalf("ListNotifyPolicyOverrides() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListNotifyPolicyOverrides() len = %d, want 1", len(list))
	}

	if err := q.DeleteNotifyPolicyOverride(ctx, policy.ID, policy.ProjectID); err != nil {
		t.Fatalf("DeleteNotifyPolicyOverride() error = %v", err)
	}
	if _, err := q.GetNotifyPolicyOverride(ctx, policy.ID, policy.ProjectID); !errors.Is(err, store.ErrNotifyPolicyNotFound) {
		t.Fatalf("GetNotifyPolicyOverride(after delete) error = %v, want ErrNotifyPolicyNotFound", err)
	}
}

func TestResolveNotifyPolicyOverride_Precedence(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-notify-policy-precedence"
	projectAttempts := 3
	categoryAttempts := 4
	stepAttempts := 5

	policies := []*domain.NotifyPolicyOverride{
		{ProjectID: projectID, ScopeType: domain.NotifyPolicyScopeProject, ScopeKey: "*", Channel: "", RetryMaxAttempts: &projectAttempts, Enabled: true},
		{ProjectID: projectID, ScopeType: domain.NotifyPolicyScopeCategory, ScopeKey: "billing", Channel: "", RetryMaxAttempts: &categoryAttempts, Enabled: true},
		{ProjectID: projectID, ScopeType: domain.NotifyPolicyScopeWorkflowStep, ScopeKey: "step_123", Channel: "email", RetryMaxAttempts: &stepAttempts, Enabled: true},
	}
	for _, policy := range policies {
		if err := q.UpsertNotifyPolicyOverride(ctx, policy); err != nil {
			t.Fatalf("UpsertNotifyPolicyOverride(%s) error = %v", policy.ScopeType, err)
		}
	}

	resolved, err := q.ResolveNotifyPolicyOverride(ctx, projectID, "step_123", "billing", "email")
	if err != nil {
		t.Fatalf("ResolveNotifyPolicyOverride(step) error = %v", err)
	}
	if resolved.ScopeType != domain.NotifyPolicyScopeWorkflowStep {
		t.Fatalf("resolved scope = %q, want %q", resolved.ScopeType, domain.NotifyPolicyScopeWorkflowStep)
	}

	resolved, err = q.ResolveNotifyPolicyOverride(ctx, projectID, "", "billing", "inbox")
	if err != nil {
		t.Fatalf("ResolveNotifyPolicyOverride(category) error = %v", err)
	}
	if resolved.ScopeType != domain.NotifyPolicyScopeCategory {
		t.Fatalf("resolved scope = %q, want %q", resolved.ScopeType, domain.NotifyPolicyScopeCategory)
	}

	resolved, err = q.ResolveNotifyPolicyOverride(ctx, projectID, "", "", "email")
	if err != nil {
		t.Fatalf("ResolveNotifyPolicyOverride(project) error = %v", err)
	}
	if resolved.ScopeType != domain.NotifyPolicyScopeProject {
		t.Fatalf("resolved scope = %q, want %q", resolved.ScopeType, domain.NotifyPolicyScopeProject)
	}
}

func TestNotifyProviderCallbackReceipt_DedupAndDelete(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	now := time.Now().UTC()
	inserted, err := q.RecordNotifyProviderCallbackReceipt(
		ctx,
		"project-notify-callback-receipts",
		"provider-1",
		"resend",
		"cb_1",
		"email.delivered",
		"msg_1",
		"hash1",
		now.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("RecordNotifyProviderCallbackReceipt(first) error = %v", err)
	}
	if !inserted {
		t.Fatal("RecordNotifyProviderCallbackReceipt(first) inserted = false, want true")
	}

	inserted, err = q.RecordNotifyProviderCallbackReceipt(
		ctx,
		"project-notify-callback-receipts",
		"provider-1",
		"resend",
		"cb_1",
		"email.delivered",
		"msg_1",
		"hash1",
		now.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("RecordNotifyProviderCallbackReceipt(duplicate) error = %v", err)
	}
	if inserted {
		t.Fatal("RecordNotifyProviderCallbackReceipt(duplicate) inserted = true, want false")
	}

	if err := q.DeleteNotifyProviderCallbackReceipt(ctx, "project-notify-callback-receipts", "provider-1", "cb_1"); err != nil {
		t.Fatalf("DeleteNotifyProviderCallbackReceipt() error = %v", err)
	}

	inserted, err = q.RecordNotifyProviderCallbackReceipt(
		ctx,
		"project-notify-callback-receipts",
		"provider-1",
		"resend",
		"cb_1",
		"email.delivered",
		"msg_1",
		"hash1",
		now.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("RecordNotifyProviderCallbackReceipt(after delete) error = %v", err)
	}
	if !inserted {
		t.Fatal("RecordNotifyProviderCallbackReceipt(after delete) inserted = false, want true")
	}
}

func TestNotifyProviderCallbackReceipt_ConcurrentDedup(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	const attempts = 12
	var (
		wg        sync.WaitGroup
		errMu     sync.Mutex
		firstErr  error
		insertedN int
	)

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			inserted, err := q.RecordNotifyProviderCallbackReceipt(
				ctx,
				"project-notify-callback-concurrent",
				"provider-1",
				"resend",
				"cb-concurrent",
				"email.delivered",
				"msg_1",
				"hash1",
				time.Now().UTC().Add(24*time.Hour),
			)
			errMu.Lock()
			defer errMu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			if inserted {
				insertedN++
			}
		}()
	}
	wg.Wait()

	if firstErr != nil {
		t.Fatalf("RecordNotifyProviderCallbackReceipt(concurrent) error = %v", firstErr)
	}
	if insertedN != 1 {
		t.Fatalf("RecordNotifyProviderCallbackReceipt(concurrent) inserted count = %d, want 1", insertedN)
	}
}

func TestNotifyStoreSentinelErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{name: "ErrNotifySubscriberNotFound", err: store.ErrNotifySubscriberNotFound, msg: "notify subscriber not found"},
		{name: "ErrNotificationTemplateNotFound", err: store.ErrNotificationTemplateNotFound, msg: "notification template not found"},
		{name: "ErrNotificationCategoryNotFound", err: store.ErrNotificationCategoryNotFound, msg: "notification category not found"},
		{name: "ErrNotificationPreferenceNotFound", err: store.ErrNotificationPreferenceNotFound, msg: "notification preference not found"},
		{name: "ErrNotificationMessageNotFound", err: store.ErrNotificationMessageNotFound, msg: "notification message not found"},
		{name: "ErrNotificationBatchNotFound", err: store.ErrNotificationBatchNotFound, msg: "notification batch not found"},
		{name: "ErrEscalationStateNotFound", err: store.ErrEscalationStateNotFound, msg: "escalation state not found"},
		{name: "ErrNotificationProviderNotFound", err: store.ErrNotificationProviderNotFound, msg: "notification provider not found"},
		{name: "ErrInboxItemNotFound", err: store.ErrInboxItemNotFound, msg: "inbox item not found"},
		{name: "ErrInboxItemAlreadyExists", err: store.ErrInboxItemAlreadyExists, msg: "inbox item already exists"},
		{name: "ErrNotifyPolicyNotFound", err: store.ErrNotifyPolicyNotFound, msg: "notify policy override not found"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.err.Error() != tt.msg {
				t.Fatalf("error message = %q, want %q", tt.err.Error(), tt.msg)
			}
		})
	}
}
