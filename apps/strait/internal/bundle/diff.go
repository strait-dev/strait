package bundle

// ComputeDiff compares a bundle against existing resources and produces
// a list of actions to take during import.
func ComputeDiff(bundle *Bundle, existingJobSlugs map[string]bool, existingWorkflowSlugs map[string]bool, existingEnvSlugs map[string]bool) []DiffEntry {
	resourceCount := len(bundle.Resources.Environments) +
		len(bundle.Resources.Jobs) +
		len(bundle.Resources.Workflows) +
		len(bundle.Resources.WebhookSubscriptions)
	entries := make([]DiffEntry, 0, resourceCount)

	// Environments first (dependencies for jobs).
	for _, env := range bundle.Resources.Environments {
		if env.IsStandard {
			entries = append(entries, DiffEntry{
				ResourceType: "environment",
				Slug:         env.Slug,
				Action:       DiffSkip,
				Details:      "standard environment (auto-created)",
			})
			continue
		}
		if existingEnvSlugs[env.Slug] {
			entries = append(entries, DiffEntry{
				ResourceType: "environment",
				Slug:         env.Slug,
				Action:       DiffUpdate,
			})
		} else {
			entries = append(entries, DiffEntry{
				ResourceType: "environment",
				Slug:         env.Slug,
				Action:       DiffCreate,
			})
		}
	}

	// Jobs next (workflows depend on them).
	for _, job := range bundle.Resources.Jobs {
		if existingJobSlugs[job.Slug] {
			entries = append(entries, DiffEntry{
				ResourceType: "job",
				Slug:         job.Slug,
				Action:       DiffUpdate,
			})
		} else {
			entries = append(entries, DiffEntry{
				ResourceType: "job",
				Slug:         job.Slug,
				Action:       DiffCreate,
			})
		}
	}

	// Workflows last (depend on jobs).
	for _, wf := range bundle.Resources.Workflows {
		if existingWorkflowSlugs[wf.Slug] {
			entries = append(entries, DiffEntry{
				ResourceType: "workflow",
				Slug:         wf.Slug,
				Action:       DiffUpdate,
			})
		} else {
			entries = append(entries, DiffEntry{
				ResourceType: "workflow",
				Slug:         wf.Slug,
				Action:       DiffCreate,
			})
		}
	}

	// Webhook subscriptions.
	for _, sub := range bundle.Resources.WebhookSubscriptions {
		entries = append(entries, DiffEntry{
			ResourceType: "webhook_subscription",
			Slug:         sub.URL,
			Action:       DiffCreate,
			Details:      "webhook subscriptions are always recreated",
		})
	}

	return entries
}

// DependencyOrder returns the import order: environments, jobs, workflows, webhooks.
func DependencyOrder() []string {
	return []string{"environment", "job", "workflow", "webhook_subscription"}
}
