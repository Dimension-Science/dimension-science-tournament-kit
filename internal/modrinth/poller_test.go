package modrinth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVersionParsing(t *testing.T) {
	jsonInput := `[
		{
			"id": "4H6PLVlz",
			"project_id": "lHBv9YNe",
			"name": "Tournament Timer 0.1.2-beta.1",
			"version_number": "0.1.2-beta.1",
			"changelog": "Fixed timer bugs.\nPauses RTA/IGT correctly.",
			"date_published": "2026-05-28T10:44:31.576555Z",
			"version_type": "beta"
		}
	]`

	var versions []Version
	err := json.Unmarshal([]byte(jsonInput), &versions)
	if err != nil {
		t.Fatalf("Failed to parse json: %v", err)
	}

	if len(versions) != 1 {
		t.Fatalf("Expected 1 version, got %d", len(versions))
	}

	v := versions[0]
	if v.ID != "4H6PLVlz" {
		t.Errorf("Expected ID '4H6PLVlz', got %q", v.ID)
	}
	if v.ProjectID != "lHBv9YNe" {
		t.Errorf("Expected ProjectID 'lHBv9YNe', got %q", v.ProjectID)
	}
	if v.VersionNumber != "0.1.2-beta.1" {
		t.Errorf("Expected version number '0.1.2-beta.1', got %q", v.VersionNumber)
	}
	if v.VersionType != "beta" {
		t.Errorf("Expected version type 'beta', got %q", v.VersionType)
	}
	expectedTime, _ := time.Parse(time.RFC3339Nano, "2026-05-28T10:44:31.576555Z")
	if !v.DatePublished.Equal(expectedTime) {
		t.Errorf("Expected DatePublished %v, got %v", expectedTime, v.DatePublished)
	}
}

func TestFetchVersionsMock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/project/test-project/version" {
			t.Errorf("Expected path /v2/project/test-project/version, got %q", r.URL.Path)
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("Expected User-Agent header to be set")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[
			{
				"id": "v1",
				"project_id": "test-project",
				"name": "Version 1",
				"version_number": "1.0.0",
				"changelog": "Initial release",
				"date_published": "2026-05-28T10:00:00Z",
				"version_type": "release"
			}
		]`))
	}))
	defer server.Close()

	poller := &Poller{
		httpClient: server.Client(),
		apiBase:    server.URL,
	}

	ctx := context.Background()
	versions, err := poller.fetchVersions(ctx, "test-project")
	if err != nil {
		t.Fatalf("Failed to fetch versions: %v", err)
	}

	if len(versions) != 1 {
		t.Fatalf("Expected 1 version, got %d", len(versions))
	}
	if versions[0].ID != "v1" {
		t.Errorf("Expected ID 'v1', got %q", versions[0].ID)
	}
}
