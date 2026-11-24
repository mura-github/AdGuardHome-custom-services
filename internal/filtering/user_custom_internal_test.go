package filtering

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitModule_customServices(t *testing.T) {
	standard, groups, originalRules := blockedServices, serviceGroups, serviceRules
	t.Cleanup(func() {
		blockedServices, serviceGroups, serviceRules = standard, groups, originalRules
	})

	dir := t.TempDir()
	writeConfig := func(name, contents string) string {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
		return path
	}

	for _, tc := range []struct {
		name, path, level string
		count             int
	}{
		{"example", filepath.Join("..", "..", "examples", "custom-services.yaml"), "INFO", 2},
		{"empty", writeConfig("empty.yaml", "services: []"), "INFO", 0},
		{"missing", filepath.Join(dir, "missing.yaml"), "WARN", 0},
		{"duplicate_id", writeConfig("duplicate.yaml", `services:
  - {id: first, name: First, domains: [example.org]}
  - {id: custom_first, name: Duplicate, domains: [example.net]}`), "ERROR", 0},
		{"malformed", writeConfig("malformed.yaml", "services: ["), "ERROR", 0},
		{"invalid_domain", writeConfig("invalid.yaml", `services:
  - {id: custom_first, name: First, domains: [example.org]}
  - {id: custom_bad, name: Bad, domains: ["https://example.org/path"]}`), "ERROR", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blockedServices, serviceGroups = standard, groups
			var logs bytes.Buffer
			InitModule(t.Context(), slog.New(slog.NewTextHandler(&logs, nil)), tc.path)
			require.Contains(t, logs.String(), "level="+tc.level)
			require.Contains(t, serviceRules, "youtube")
			require.Len(t, blockedServices, len(standard)+tc.count)
			if tc.count == 0 {
				require.Len(t, serviceGroups, len(groups))
				require.NotContains(t, serviceRules, "custom_first")
				return
			}
			require.Len(t, serviceGroups, len(groups)+1)
			require.Equal(t, userCustomGroupID, serviceGroups[len(groups)].ID)
			for _, svc := range blockedServices[len(standard):] {
				require.Equal(t, userCustomGroupID, svc.GroupID)
				require.Len(t, serviceRules[svc.ID], len(svc.Rules))
			}
			require.Equal(t, "||wikipedia.org^", serviceRules["custom_wikipedia"][0].Text())
			stackRules := serviceRules["custom_stack_overflow"]
			require.Len(t, stackRules, 2)
			require.Equal(t, "||stackoverflow.com^", stackRules[0].Text())
			require.Equal(t, "||stackoverflow.blog^", stackRules[1].Text())
		})
	}
}

func TestLoadCustomServices_idPrefix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom-services.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`services:
  - {id: youtube, name: Local YouTube, domains: [example.org]}
  - {id: custom_pixiv, name: Pixiv, domains: [pixiv.net]}`), 0o600))

	services, err := loadCustomServices(path)
	require.NoError(t, err)
	require.Len(t, services, 2)
	require.Equal(t, "custom_youtube", services[0].ID)
	require.Equal(t, "custom_pixiv", services[1].ID)
}
