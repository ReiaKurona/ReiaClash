package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reia-clash/internal/config"
	"strings"
	"testing"
	"time"
)

func subscriptionTestApp(t *testing.T) *App {
	t.Helper()
	paths, err := config.PathsAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	prefs, err := config.OpenStore(paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Profile, []byte("old profile"), 0600); err != nil {
		t.Fatal(err)
	}
	a := &App{Paths: paths, Prefs: prefs, subscriptionWake: make(chan struct{}, 1)}
	a.subscriptionApply = func(profile []byte) error { return os.WriteFile(paths.Profile, profile, 0600) }
	return a
}

func TestSubscriptionLatestURLSurvivesFailureAndColdStart(t *testing.T) {
	a := subscriptionTestApp(t)
	oldURL, newURL := "https://subscription.example/old?token=old", "https://subscription.example/latest?token=new"
	_, err := a.Prefs.Update(func(s *config.Settings) error {
		s.SubscriptionURL = oldURL
		s.SubscriptionUpdatedAt = time.Now().Unix()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	a.subscriptionFetch = func(context.Context, string) ([]byte, error) { return nil, errors.New("network failed at " + newURL) }
	if err := a.ImportSubscription(context.Background(), newURL); err == nil {
		t.Fatal("expected failure")
	}
	saved, err := config.OpenStore(a.Paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Snapshot().SubscriptionURL != newURL || saved.Snapshot().SubscriptionUpdatedAt != 0 {
		t.Fatal("latest link was not persisted for offline recovery")
	}
	profile, _ := os.ReadFile(a.Paths.Profile)
	if string(profile) != "old profile" {
		t.Fatal("failed download replaced usable profile")
	}
	encoded, _ := json.Marshal(a.SubscriptionStatus())
	if strings.Contains(string(encoded), "token=") || strings.Contains(string(encoded), newURL) {
		t.Fatal("status exposed credentials")
	}
	a.subscriptionFetch = func(_ context.Context, url string) ([]byte, error) {
		if url != newURL {
			t.Fatal("offline retry used old link")
		}
		return []byte("new profile"), nil
	}
	if err := a.RefreshSubscription(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a.SubscriptionStatus().LastSuccessAt == 0 || a.SubscriptionStatus().LastError != "" {
		t.Fatal(a.SubscriptionStatus())
	}
}

func TestSubscriptionRefreshSerializesWithNewPush(t *testing.T) {
	a := subscriptionTestApp(t)
	_, _ = a.Prefs.Update(func(s *config.Settings) error { s.SubscriptionURL = "https://subscription.example/old"; return nil })
	started, release := make(chan struct{}), make(chan struct{})
	a.subscriptionFetch = func(_ context.Context, url string) ([]byte, error) {
		if strings.HasSuffix(url, "/old") {
			close(started)
			<-release
		}
		return []byte(url), nil
	}
	background := make(chan error, 1)
	go func() { background <- a.RefreshSubscription(context.Background()) }()
	<-started
	push := make(chan error, 1)
	go func() { push <- a.ImportSubscription(context.Background(), "https://subscription.example/latest") }()
	close(release)
	if err := <-background; err != nil {
		t.Fatal(err)
	}
	if err := <-push; err != nil {
		t.Fatal(err)
	}
	profile, _ := os.ReadFile(a.Paths.Profile)
	if string(profile) != "https://subscription.example/latest" || a.Prefs.Snapshot().SubscriptionURL != string(profile) {
		t.Fatal("old refresh overwrote new link/profile")
	}
}

func TestSubscriptionUnchangedProfileAvoidsRestart(t *testing.T) {
	a := subscriptionTestApp(t)
	count := 0
	a.subscriptionFetch = func(context.Context, string) ([]byte, error) { return []byte("stable profile"), nil }
	a.subscriptionApply = func(profile []byte) error { count++; return os.WriteFile(a.Paths.Profile, profile, 0600) }
	if err := a.ImportSubscription(context.Background(), "https://subscription.example/profile"); err != nil {
		t.Fatal(err)
	}
	if err := a.RefreshSubscription(context.Background()); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("unchanged profile restarted %d times", count)
	}
}

func TestSubscriptionColdStartActuallyRefreshesRecentSavedURL(t *testing.T) {
	a := subscriptionTestApp(t)
	_, _ = a.Prefs.Update(func(s *config.Settings) error {
		s.SubscriptionURL = "https://subscription.example/saved"
		s.SubscriptionUpdatedAt = time.Now().Unix()
		return nil
	})
	fetched := make(chan string, 1)
	a.subscriptionFetch = func(_ context.Context, url string) ([]byte, error) { fetched <- url; return []byte("old profile"), nil }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	go func() { defer close(stopped); a.subscriptionRefreshLoop(ctx) }()
	select {
	case url := <-fetched:
		if !strings.HasSuffix(url, "/saved") {
			t.Fatal(url)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cold start did not refresh")
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh worker did not stop")
	}
}

func TestSubscriptionValidationFailureRetainsProfileAndNewURL(t *testing.T) {
	a := subscriptionTestApp(t)
	a.subscriptionFetch = func(context.Context, string) ([]byte, error) { return []byte("invalid YAML"), nil }
	a.subscriptionApply = func([]byte) error { return errors.New("validation failed") }
	if err := a.ImportSubscription(context.Background(), "https://subscription.example/latest"); err == nil {
		t.Fatal("validation failure ignored")
	}
	profile, _ := os.ReadFile(a.Paths.Profile)
	if string(profile) != "old profile" || !strings.HasSuffix(a.Prefs.Snapshot().SubscriptionURL, "/latest") {
		t.Fatal("lost usable profile or new URL")
	}
}

func TestSubscriptionStartupAndTwelveHourSchedule(t *testing.T) {
	now := time.Now()
	s := config.Settings{SubscriptionURL: "https://subscription.example/profile", SubscriptionUpdatedAt: now.Unix()}
	if !subscriptionDue(s, now, true) || subscriptionDue(s, now.Add(11*time.Hour), false) || !subscriptionDue(s, now.Add(12*time.Hour), false) {
		t.Fatal("incorrect startup/12h schedule")
	}
	s.SubscriptionURL = ""
	if subscriptionDue(s, now, true) {
		t.Fatal("attempted refresh without saved subscription")
	}
	for _, url := range []string{"file:///tmp/profile", "http://127.0.0.1/private", "http://user:pass@subscription.example/", "http://localhost/profile"} {
		if _, err := parseSubscriptionURL(url); err == nil {
			t.Fatal("accepted unsafe URL", url)
		}
	}
}

func TestSubscriptionMetadataSurvivesProxyPreferenceRollback(t *testing.T) {
	previous := config.Settings{UseTun: false, SubscriptionURL: "https://subscription.example/old", SubscriptionUpdatedAt: 1}
	current := config.Settings{UseTun: true, SubscriptionURL: "https://subscription.example/latest", SubscriptionUpdatedAt: 2}
	restoreManagedPreferences(&current, previous)
	if current.UseTun || !strings.HasSuffix(current.SubscriptionURL, "/latest") || current.SubscriptionUpdatedAt != 2 {
		t.Fatal("proxy rollback restored stale subscription")
	}
}

func TestAutomaticSubscriptionSyncSkipsFreshSameURL(t *testing.T) {
	app := subscriptionTestApp(t)
	calls := 0
	app.subscriptionFetch = func(_ context.Context, _ string) ([]byte, error) {
		calls++
		return []byte("updated profile"), nil
	}
	url := "https://subscription.example/profile"
	if err := app.ImportSubscription(t.Context(), url); err != nil {
		t.Fatal(err)
	}
	if err := app.SyncSubscription(t.Context(), url); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("same URL was downloaded %d times", calls)
	}
	if err := app.SyncSubscription(t.Context(), url+"/new"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("rotated URL was not applied")
	}
}
