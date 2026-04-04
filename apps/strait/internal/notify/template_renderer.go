package notify

import (
	"encoding/json"
	"fmt"

	"strait/internal/domain"

	"github.com/mailgun/raymond/v2"
)

// RenderedTemplate contains per-channel rendered payloads.
type RenderedTemplate struct {
	Channels map[string]any
}

// RenderTemplate renders a notification template for a specific locale using
// Handlebars-compatible templates.
func RenderTemplate(tmpl *domain.NotificationTemplate, locale string, context map[string]any) (*RenderedTemplate, error) {
	if tmpl == nil {
		return nil, fmt.Errorf("template is nil")
	}
	if len(tmpl.Channels) == 0 {
		return nil, fmt.Errorf("template channels are required")
	}

	baseChannels := map[string]any{}
	if err := json.Unmarshal(tmpl.Channels, &baseChannels); err != nil {
		return nil, fmt.Errorf("unmarshal template channels: %w", err)
	}

	resolved := deepCopyMap(baseChannels)
	locale = resolveLocale(locale, tmpl.DefaultLocale)
	if locale != "" {
		overrides, err := localeChannelOverrides(tmpl.LocaleTemplates, locale)
		if err != nil {
			return nil, fmt.Errorf("resolve locale overrides: %w", err)
		}
		if len(overrides) > 0 {
			resolved = mergeMaps(resolved, overrides)
		}
	}

	rendered, err := renderValue(resolved, context)
	if err != nil {
		return nil, err
	}
	out, ok := rendered.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("rendered template channels must be an object")
	}

	return &RenderedTemplate{Channels: out}, nil
}

func resolveLocale(locale, defaultLocale string) string {
	if locale != "" {
		return locale
	}
	if defaultLocale != "" {
		return defaultLocale
	}
	return "en"
}

func localeChannelOverrides(raw json.RawMessage, locale string) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var all map[string]map[string]any
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, fmt.Errorf("unmarshal locale templates: %w", err)
	}

	entry, ok := all[locale]
	if !ok {
		return nil, nil
	}

	channels, ok := entry["channels"].(map[string]any)
	if !ok {
		return nil, nil
	}
	return channels, nil
}

func renderValue(value any, context map[string]any) (any, error) {
	switch v := value.(type) {
	case string:
		rendered, err := raymond.Render(v, context)
		if err != nil {
			return nil, fmt.Errorf("render handlebars template: %w", err)
		}
		return rendered, nil
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			renderedChild, err := renderValue(child, context)
			if err != nil {
				return nil, err
			}
			out[key] = renderedChild
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			renderedChild, err := renderValue(child, context)
			if err != nil {
				return nil, err
			}
			out[i] = renderedChild
		}
		return out, nil
	default:
		return value, nil
	}
}

func deepCopyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		switch v := value.(type) {
		case map[string]any:
			out[key] = deepCopyMap(v)
		case []any:
			copied := make([]any, len(v))
			for i, child := range v {
				if m, ok := child.(map[string]any); ok {
					copied[i] = deepCopyMap(m)
					continue
				}
				copied[i] = child
			}
			out[key] = copied
		default:
			out[key] = value
		}
	}
	return out
}

func mergeMaps(base, override map[string]any) map[string]any {
	out := deepCopyMap(base)
	for key, value := range override {
		if existing, ok := out[key]; ok {
			existingMap, existingIsMap := existing.(map[string]any)
			overrideMap, overrideIsMap := value.(map[string]any)
			if existingIsMap && overrideIsMap {
				out[key] = mergeMaps(existingMap, overrideMap)
				continue
			}
		}
		out[key] = value
	}
	return out
}
