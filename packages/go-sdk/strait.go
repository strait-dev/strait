// Package strait provides a Go SDK for the Strait platform API.
//
// Create a client using functional options:
//
//	client := strait.NewClient(
//		strait.WithBaseURL("https://api.strait.dev"),
//		strait.WithBearerToken("sk_live_..."),
//	)
//
// Or from environment variables:
//
//	client, err := strait.NewClientFromEnv()
//
// The client provides domain services for all API operations:
//
//	job, err := client.Jobs.Create(ctx, CreateJobBody{...})
//	run, err := client.Runs.Get(ctx, "run_123")
package strait
