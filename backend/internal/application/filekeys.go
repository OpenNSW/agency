package application

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/OpenNSW/nsw-agency/backend/internal/nswclient"
)

// FileResolver resolves opaque object-storage keys to time-limited, directly
// fetchable download URLs. It is the consumer-side view of internal/nswclient's
// storage API, satisfied directly by *nswclient.Client.
type FileResolver interface {
	GetDownloadURL(ctx context.Context, key string) (*nswclient.DownloadMetadata, error)
}

// resolveFileKeys walks formDoc (a {"schema":..., "uiSchema":...} artifact, as
// returned by generictemplate.Load) alongside data, replacing every value at a
// JSON Schema path marked format:"file" with a presigned download URL.
//
// Submitted application data otherwise carries raw object-storage keys
// straight through from the NSW service. The backend, not the browser, is the
// trust boundary that's allowed to exchange a key for a presigned URL, so raw
// keys must never reach the frontend for data that's already been submitted.
func resolveFileKeys(ctx context.Context, resolver FileResolver, formDoc json.RawMessage, data map[string]any) {
	if resolver == nil || len(formDoc) == 0 || len(data) == 0 {
		return
	}

	var doc struct {
		Schema map[string]any `json:"schema"`
	}
	if err := json.Unmarshal(formDoc, &doc); err != nil || doc.Schema == nil {
		return
	}

	walkSchema(ctx, resolver, doc.Schema, data)
}

// walkSchema mirrors data against schema, mutating data in place wherever a
// string (or array/object of strings) is marked format:"file" in the schema.
func walkSchema(ctx context.Context, resolver FileResolver, schema map[string]any, data any) {
	switch schema["type"] {
	case "object":
		obj, ok := data.(map[string]any)
		props, propsOK := schema["properties"].(map[string]any)
		if !ok || !propsOK {
			return
		}
		for name, propSchema := range props {
			ps, ok := propSchema.(map[string]any)
			if !ok {
				continue
			}
			if val, exists := obj[name]; exists {
				obj[name] = resolveValue(ctx, resolver, ps, val)
			}
		}
	case "array":
		arr, ok := data.([]any)
		items, itemsOK := schema["items"].(map[string]any)
		if !ok || !itemsOK {
			return
		}
		for i, v := range arr {
			arr[i] = resolveValue(ctx, resolver, items, v)
		}
	}
}

// resolveValue applies schema to a single value, returning either the value
// with any file keys replaced by presigned URLs, or the value unchanged.
func resolveValue(ctx context.Context, resolver FileResolver, schema map[string]any, value any) any {
	if schema["type"] == "string" && schema["format"] == "file" {
		key, ok := value.(string)
		if !ok || key == "" {
			return value
		}
		return resolveKey(ctx, resolver, key)
	}

	switch v := value.(type) {
	case map[string]any, []any:
		walkSchema(ctx, resolver, schema, v)
		return v
	default:
		return value
	}
}

// resolveKey exchanges a single storage key for a presigned download URL. On
// failure, the raw key is dropped rather than forwarded: a missing file link
// is a display glitch, a leaked key is a security issue.
func resolveKey(ctx context.Context, resolver FileResolver, key string) string {
	metadata, err := resolver.GetDownloadURL(ctx, key)
	if err != nil {
		slog.WarnContext(ctx, "failed to resolve file key to a presigned URL; omitting from response", "error", err)
		return ""
	}
	return metadata.DownloadURL
}
