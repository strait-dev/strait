package testutil

import (
	"testing"
)

type cmpFixture struct {
	Name string
	Tags []string
}

func TestAssertEqual_AllowsCmpOptions(t *testing.T) {
	t.Parallel()

	got := cmpFixture{Name: "job"}
	want := cmpFixture{Name: "job", Tags: []string{}}
	AssertEqual(t, got, want, EquateEmpty())
}

func TestAssertJSONEqual_NormalizesObjects(t *testing.T) {
	t.Parallel()

	AssertJSONEqual(t,
		[]byte(`{"name":"job","config":{"retries":3,"enabled":true}}`),
		[]byte(`{"config":{"enabled":true,"retries":3},"name":"job"}`),
	)
}

func TestIgnoreFields_IgnoresSelectedFields(t *testing.T) {
	t.Parallel()

	got := cmpFixture{Name: "job", Tags: []string{"live"}}
	want := cmpFixture{Name: "job", Tags: []string{"ignored"}}
	AssertEqual(t, got, want, IgnoreFields(cmpFixture{}, "Tags"))
}
