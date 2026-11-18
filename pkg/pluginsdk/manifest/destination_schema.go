package manifest

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

// What a destination schema and form may use. People who aren't administrators fill
// in destination settings, and every client renders the form, so it stays a
// flat list of scalar fields. Loosening this later is additive; tightening it
// would break published plugins.
var (
	destinationSchemaKeywords = []string{"$schema", "type", "title", "description", "properties", "required", "additionalProperties"}
	destinationFieldKeywords  = []string{"type", "title", "description", "default", "enum", "format", "minimum", "maximum", "minLength", "maxLength", "pattern"}
	destinationFieldTypes     = []string{"string", "integer", "number", "boolean"}

	// The form controls a destination field may use, and the field types
	// each one accepts.
	destinationControlTypes = map[pluginv1.AdminFormControl][]string{
		pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_TEXT:     {"string"},
		pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_PASSWORD: {"string"},
		pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_NUMBER:   {"integer", "number"},
		pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_SWITCH:   {"boolean"},
		pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_SELECT:   {"string", "integer", "number"},
	}
)

// ValidateDestinationSchema checks a notification_channel destination_schema:
// a valid ConfigSchema whose json_schema is one object of scalar fields and
// whose admin_form is one flat list of fields.
//
// Fields are string, integer, number or boolean, with optional title,
// description, default, enum, format, minimum/maximum and
// minLength/maxLength/pattern. additionalProperties may only be false.
// $ref, allOf/anyOf/oneOf/not, conditionals, nested objects and arrays are
// refused.
//
// The admin_form is required and has at least one field, one per schema
// field and the reverse. A form field sets key, label and control and may set
// description, placeholder, required, secret, default_value and, on a select,
// options. Controls are TEXT and PASSWORD (string), NUMBER (integer or
// number), SWITCH (boolean) and SELECT (string, integer or number, with static
// options). Sections, submit_label, show_when, dynamic_options,
// exclusive_group_field, validation, multiline, rows, TEXTAREA and
// MULTI_SELECT are refused. Hosts call it when they accept a
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
	// Hosts store only declared fields, so a schema can't allow others.
	if raw, ok := root["additionalProperties"]; ok {
		var allowed bool
		if err := json.Unmarshal(raw, &allowed); err != nil || allowed {
			return fmt.Errorf("destination_schema additionalProperties must be false or absent")
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
	return validateDestinationForm(schema.GetAdminForm(), properties)
}

// validateDestinationForm checks the admin_form against the flat-form rules
// above. validateConfigSchema already checked that each form field names a
// property and that keys are unique.
func validateDestinationForm(form *pluginv1.AdminFormDescriptor, properties map[string]map[string]json.RawMessage) error {
	if len(form.GetFields()) == 0 {
		return fmt.Errorf("destination_schema needs an admin_form with at least one field")
	}
	if len(form.GetSections()) > 0 {
		return fmt.Errorf("destination_schema admin_form can't use sections; a destination form is a flat list of fields")
	}
	if form.GetSubmitLabel() != "" {
		return fmt.Errorf("destination_schema admin_form can't set submit_label; the client's dialog owns its buttons")
	}
	inForm := make(map[string]bool, len(form.GetFields()))
	for _, field := range form.GetFields() {
		if field == nil {
			return fmt.Errorf("destination_schema admin_form has an empty field")
		}
		key := field.GetKey()
		inForm[key] = true
		if strings.TrimSpace(field.GetLabel()) == "" {
			return fmt.Errorf("destination_schema admin_form field %q needs a label", key)
		}
		var refused string
		switch {
		case len(field.GetShowWhen()) > 0:
			refused = "show_when"
		case field.GetDynamicOptions():
			refused = "dynamic_options"
		case field.GetExclusiveGroupField() != "":
			refused = "exclusive_group_field"
		case field.GetValidation() != nil:
			refused = "validation (put minimum, maximum, minLength, maxLength and pattern in the json_schema)"
		case field.GetMultiline():
			refused = "multiline"
		case field.GetRows() != 0:
			refused = "rows"
		}
		if refused != "" {
			return fmt.Errorf("destination_schema admin_form field %q can't use %s; a destination form is a flat list of fields", key, refused)
		}
		var fieldType string
		_ = json.Unmarshal(properties[key]["type"], &fieldType)
		control := field.GetControl()
		allowed, ok := destinationControlTypes[control]
		if !ok {
			return fmt.Errorf("destination_schema admin_form field %q uses control %s; use TEXT, PASSWORD, NUMBER, SWITCH or SELECT", key, control)
		}
		if !slices.Contains(allowed, fieldType) {
			return fmt.Errorf("destination_schema admin_form field %q: control %s needs a field of type %s, not %q", key, control, strings.Join(allowed, " or "), fieldType)
		}
		if control == pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_SELECT && len(field.GetOptions()) == 0 {
			return fmt.Errorf("destination_schema admin_form field %q select control requires static options", key)
		}
		if control != pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_SELECT && len(field.GetOptions()) > 0 {
			return fmt.Errorf("destination_schema admin_form field %q has options, which only a SELECT control uses", key)
		}
		if field.GetSecret() && fieldType != "string" {
			return fmt.Errorf("destination_schema admin_form field %q is secret but not a string field", key)
		}
	}
	for name := range properties {
		if !inForm[name] {
			return fmt.Errorf("destination_schema field %q has no admin_form field", name)
		}
	}
	return nil
}
