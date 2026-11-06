package manifest

import (
	"encoding/json"
	"fmt"
	"slices"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

// Keywords a destination form may use. People who aren't administrators fill
// in destination settings, and every client renders the form, so it stays a
// flat list of scalar fields. Loosening this later is additive; tightening it
// would break published plugins.
var (
	destinationSchemaKeywords = []string{"$schema", "type", "title", "description", "properties", "required", "additionalProperties"}
	destinationFieldKeywords  = []string{"type", "title", "description", "default", "enum", "format", "minimum", "maximum", "minLength", "maxLength", "pattern"}
	destinationFieldTypes     = []string{"string", "integer", "number", "boolean"}
)

// ValidateDestinationSchema checks a notification_channel destination_schema:
// a valid ConfigSchema whose json_schema is one object of scalar fields.
// Fields are string, integer, number or boolean, with optional title,
// description, default, enum, format, minimum/maximum and
// minLength/maxLength/pattern. $ref, allOf/anyOf/oneOf/not, conditionals,
// nested objects and arrays are refused. Hosts call it when they accept a
// notification_channel capability.
func ValidateDestinationSchema(schema *pluginv1.ConfigSchema) error {
	if schema == nil {
		return fmt.Errorf("destination_schema is required")
	}
	if err := validateConfigSchema(schema); err != nil {
		return err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(schema.GetJsonSchema()), &root); err != nil {
		return fmt.Errorf("destination_schema json_schema is not a JSON object: %w", err)
	}
	for keyword := range root {
		if !slices.Contains(destinationSchemaKeywords, keyword) {
			return fmt.Errorf("destination_schema json_schema uses %q; a destination form is a flat list of fields", keyword)
		}
	}
	var typ string
	if err := json.Unmarshal(root["type"], &typ); err != nil || typ != "object" {
		return fmt.Errorf("destination_schema json_schema must have type \"object\"")
	}
	var properties map[string]map[string]json.RawMessage
	if raw, ok := root["properties"]; ok {
		if err := json.Unmarshal(raw, &properties); err != nil {
			return fmt.Errorf("destination_schema properties must map field names to objects: %w", err)
		}
	}
	for name, field := range properties {
		for keyword := range field {
			if !slices.Contains(destinationFieldKeywords, keyword) {
				return fmt.Errorf("destination_schema field %q uses %q; destination fields are plain scalars", name, keyword)
			}
		}
		var fieldType string
		if err := json.Unmarshal(field["type"], &fieldType); err != nil || !slices.Contains(destinationFieldTypes, fieldType) {
			return fmt.Errorf("destination_schema field %q must have type string, integer, number or boolean", name)
		}
	}
	if raw, ok := root["additionalProperties"]; ok {
		var allowed bool
		if err := json.Unmarshal(raw, &allowed); err != nil {
			return fmt.Errorf("destination_schema additionalProperties must be a boolean")
		}
	}
	if raw, ok := root["required"]; ok {
		var required []string
		if err := json.Unmarshal(raw, &required); err != nil {
			return fmt.Errorf("destination_schema required must be a list of field names")
		}
		for _, name := range required {
			if _, ok := properties[name]; !ok {
				return fmt.Errorf("destination_schema requires %q, which isn't a field", name)
			}
		}
	}
	return nil
}
