package filtering

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strings"

	"github.com/AdguardTeam/golibs/logutil/slogutil"
	"github.com/AdguardTeam/golibs/netutil"
	yaml "go.yaml.in/yaml/v4"
)

const userCustomGroupID = "user_custom"

const customServiceIDPrefix = "custom_"

// userCustomIconSVG is the shared icon for locally defined services.
const userCustomIconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3 4a1 1 0 0 1 1-1h7l10 10a1.4 1.4 0 0 1 0 2l-6 6a1.4 1.4 0 0 1-2 0L3 11z"/><circle cx="7" cy="7" r="1" fill="currentColor" stroke="none"/><path d="M10 13h6m-3-3v6"/></svg>`

// registerCustomServices adds the configured services and their group at startup.
func registerCustomServices(ctx context.Context, l *slog.Logger, path string) {
	services, err := loadCustomServices(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		l.WarnContext(ctx, "custom services file not found; using standard services", "path", path)
	case err != nil:
		l.ErrorContext(ctx, "loading custom services; using standard services", "path", path, slogutil.KeyError, err)
	default:
		if len(services) > 0 {
			serviceGroups = append(serviceGroups, serviceGroup{ID: userCustomGroupID})
			blockedServices = append(blockedServices, services...)
		}
		l.InfoContext(ctx, "loaded custom services", "path", path, "count", len(services))
	}
}

// loadCustomServices reads the operator's service definitions as one unit.
func loadCustomServices(path string) ([]blockedService, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var conf struct {
		Services []struct {
			ID      string   `yaml:"id"`
			Name    string   `yaml:"name"`
			Domains []string `yaml:"domains"`
		} `yaml:"services"`
	}
	err = yaml.Load(data, &conf, yaml.WithKnownFields())
	if err != nil {
		return nil, err
	}
	if conf.Services == nil {
		return nil, fmt.Errorf("missing services list")
	}

	services := make([]blockedService, 0, len(conf.Services))
	usedIDs := make(map[string]bool, len(blockedServices)+len(conf.Services))
	for _, svc := range blockedServices {
		usedIDs[svc.ID] = true
	}
	for i, def := range conf.Services {
		if strings.TrimSpace(def.ID) == "" || strings.TrimSpace(def.Name) == "" || len(def.Domains) == 0 {
			return nil, fmt.Errorf("services[%d]: id, name, and domains are required", i)
		}

		// Preserve IDs from configurations that already include the prefix.
		id := customServiceIDPrefix + strings.TrimPrefix(strings.TrimSpace(def.ID), customServiceIDPrefix)
		if usedIDs[id] {
			return nil, fmt.Errorf("services[%d]: duplicate service ID %q", i, id)
		}
		usedIDs[id] = true

		svc := blockedService{
			ID:      id,
			Name:    def.Name,
			GroupID: userCustomGroupID,
			IconSVG: []byte(userCustomIconSVG),
		}
		for _, domain := range def.Domains {
			host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
			if err = netutil.ValidateHostname(host); err != nil {
				return nil, fmt.Errorf("service %q: invalid domain %q: %w", def.ID, domain, err)
			}
			svc.Rules = append(svc.Rules, "||"+host+"^")
		}
		services = append(services, svc)
	}

	return services, nil
}
