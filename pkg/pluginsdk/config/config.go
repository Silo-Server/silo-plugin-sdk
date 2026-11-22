package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

func ValidateManifestGlobalValue(manifest *pluginv1.PluginManifest, key string, value map[string]any) error {
	if manifest == nil {
		return fmt.Errorf("plugin manifest is required")
	}
	return ValidateValue(FindSchema(manifest.GetGlobalConfigSchema(), key), "plugin global config", key, value)
}

func ValidateManifestUserValue(manifest *pluginv1.PluginManifest, key string, value map[string]any) error {
	if manifest == nil {
		return fmt.Errorf("plugin manifest is required")
	}
	return ValidateValue(FindSchema(manifest.GetUserConfigSchema(), key), "plugin user config", key, value)
}

func FindSchema(schemas []*pluginv1.ConfigSchema, key string) *pluginv1.ConfigSchema {
	for _, schema := range schemas {
		if schema != nil && schema.GetKey() == key {
			return schema
		}
	}
	return nil
}

func ValidateValue(schema *pluginv1.ConfigSchema, kind string, key string, value map[string]any) error {
	if schema == nil {
		return fmt.Errorf("%s key %q is not declared in the manifest schema", kind, key)
	}

	schemaJSON := strings.TrimSpace(schema.GetJsonSchema())
	if schemaJSON == "" {
		schemaJSON = `{}`
	}

	compiled, err := CompileSchema(schemaJSON)
	if err != nil {
		return fmt.Errorf("compile %s schema %q: %w", kind, key, err)
	}

	configValue := value
	if configValue == nil {
		configValue = map[string]any{}
	}
	if err := compiled.Validate(configValue); err != nil {
		return fmt.Errorf("validate %s %q: %w", kind, key, err)
	}

	return nil
}

// CompileSchema compiles a JSON Schema the way ValidateValue does: draft
// 2020-12 unless the schema names another with $schema, and Go regexp
// (RE2) syntax for pattern. Manifest validation uses it to refuse a schema
// a host could never check values against.
func CompileSchema(schemaJSON string) (*jsonschema.Schema, error) {
	var schemaDoc any
	if err := json.Unmarshal([]byte(schemaJSON), &schemaDoc); err != nil {
		return nil, fmt.Errorf("parse schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	const resourceName = "memory://config/schema.json"
	if err := compiler.AddResource(resourceName, schemaDoc); err != nil {
		return nil, fmt.Errorf("load schema: %w", err)
	}
	return compiler.Compile(resourceName)
}
