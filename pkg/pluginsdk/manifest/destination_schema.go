package manifest

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/config"
)

// What a destination schema and form may use. People who aren't administrators fill
// in destination settings, and every client renders the form, so it stays a
// flat list of scalar fields that can always be saved. Loosening this later
// is additive; tightening it would break published plugins.
var (
	destinationSchemaKeywords = []string{"type", "title", "description", "properties", "required", "additionalProperties"}
	destinationFieldKeywords  = []string{"type", "title", "description", "default", "enum", "format", "minimum", "maximum", "minLength", "maxLength", "pattern"}
	destinationFieldTypes     = []string{"string", "integer", "number", "boolean"}

	// The keywords that apply to only some field types.
	destinationStringKeywords = []string{"format", "minLength", "maxLength", "pattern"}
	destinationNumberKeywords = []string{"minimum", "maximum"}

	// A field key: what first-party plugin config keys look like, and safe
	// to use unchanged as a form key, an API field name and an error path.
	destinationFieldKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

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

// destinationProperty is one field of a destination schema.
type destinationProperty struct {
	keywords map[string]json.RawMessage
	typ      string
	// schema checks one value of this field, compiled as hosts compile it.
	schema interface{ Validate(any) error }
}

// ValidateDestinationSchema checks a notification_channel destination_schema:
// a valid ConfigSchema whose json_schema is one object of scalar fields and
// whose admin_form is one flat list of fields that a host can render and
// save.
//
// The json_schema compiles with config.CompileSchema (draft 2020-12; $schema
// is refused, so no other draft applies). Fields are string, integer, number
// or boolean, with optional title, description, default, enum,
// minimum/maximum (integer and number) and format/minLength/maxLength/pattern
// (string). Bounds are ordered, lengths are not negative, and a pattern is
// Go RE2 syntax. additionalProperties may only be false. $ref,
// allOf/anyOf/oneOf/not, conditionals, nested objects and arrays are refused.
//
// The admin_form is required and has at least one field, one per schema
// field and the reverse, each keyed exactly by its property name, which is
// a letter followed by up to 63 letters, digits, underscores or hyphens. A
// form field sets key, label and control and may set description,
// placeholder, required (which must match the schema's required list),
// secret (string fields), default_value and, on a select, options. Controls
// are TEXT and PASSWORD (string), NUMBER (integer or number), SWITCH
// (boolean) and SELECT (string, integer or number, with static options).
// Option values are unique and parse as the field's type, and equal the
// field's enum when it has one; only a SELECT field may declare enum. A
// default, in the schema or the form, is a value of the field's type that
// passes its checks and, on a SELECT, is one of the options. Sections,
// submit_label, show_when, dynamic_options, exclusive_group_field,
// validation, multiline, rows, TEXTAREA and MULTI_SELECT are refused. Hosts
// call it when they accept a notification_channel capability.
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
		if keyword == "$schema" {
			return fmt.Errorf("destination_schema json_schema can't set $schema; hosts compile it as draft 2020-12")
		}
		if !slices.Contains(destinationSchemaKeywords, keyword) {
			return fmt.Errorf("destination_schema json_schema uses %q; a destination form is a flat list of fields", keyword)
		}
	}
	var typ string
	if err := json.Unmarshal(root["type"], &typ); err != nil || typ != "object" {
		return fmt.Errorf("destination_schema json_schema must have type \"object\"")
	}
	var rawProperties map[string]json.RawMessage
	if raw, ok := root["properties"]; ok {
		if err := json.Unmarshal(raw, &rawProperties); err != nil {
			return fmt.Errorf("destination_schema properties must map field names to objects: %w", err)
		}
	}
	properties := make(map[string]*destinationProperty, len(rawProperties))
	for name, raw := range rawProperties {
		property, err := parseDestinationProperty(name, raw)
		if err != nil {
			return err
		}
		properties[name] = property
	}
	// The schema must compile exactly as a host compiles it to check
	// submitted settings, or no destination could ever be saved.
	if _, err := config.CompileSchema(schema.GetJsonSchema()); err != nil {
		return fmt.Errorf("destination_schema json_schema doesn't compile: %w", err)
	}
	// Hosts store only declared fields, so a schema can't allow others.
	if raw, ok := root["additionalProperties"]; ok {
		var allowed bool
		if err := json.Unmarshal(raw, &allowed); err != nil || allowed {
			return fmt.Errorf("destination_schema additionalProperties must be false or absent")
		}
	}
	var required []string
	if raw, ok := root["required"]; ok {
		if err := json.Unmarshal(raw, &required); err != nil {
			return fmt.Errorf("destination_schema required must be a list of field names")
		}
		for _, name := range required {
			if _, ok := properties[name]; !ok {
				return fmt.Errorf("destination_schema requires %q, which isn't a field", name)
			}
		}
	}
	return validateDestinationForm(schema.GetAdminForm(), properties, required)
}

// parseDestinationProperty checks one schema field on its own: its
// keywords, its type, the keywords its type allows, its bounds and its
// pattern, and compiles it so values can be checked against it.
func parseDestinationProperty(name string, raw json.RawMessage) (*destinationProperty, error) {
	p := &destinationProperty{}
	if err := json.Unmarshal(raw, &p.keywords); err != nil {
		return nil, fmt.Errorf("destination_schema field %q must be an object: %w", name, err)
	}
	for keyword := range p.keywords {
		if !slices.Contains(destinationFieldKeywords, keyword) {
			return nil, fmt.Errorf("destination_schema field %q uses %q; destination fields are plain scalars", name, keyword)
		}
	}
	if err := json.Unmarshal(p.keywords["type"], &p.typ); err != nil || !slices.Contains(destinationFieldTypes, p.typ) {
		return nil, fmt.Errorf("destination_schema field %q must have type string, integer, number or boolean", name)
	}
	for keyword := range p.keywords {
		switch {
		case slices.Contains(destinationStringKeywords, keyword) && p.typ != "string":
			return nil, fmt.Errorf("destination_schema field %q uses %q, which applies only to a string field", name, keyword)
		case slices.Contains(destinationNumberKeywords, keyword) && p.typ != "integer" && p.typ != "number":
			return nil, fmt.Errorf("destination_schema field %q uses %q, which applies only to an integer or number field", name, keyword)
		}
	}
	var bounds struct {
		Minimum   *float64 `json:"minimum"`
		Maximum   *float64 `json:"maximum"`
		MinLength *int     `json:"minLength"`
		MaxLength *int     `json:"maxLength"`
		Pattern   *string  `json:"pattern"`
	}
	if err := json.Unmarshal(raw, &bounds); err != nil {
		return nil, fmt.Errorf("destination_schema field %q has a bound of the wrong type: %w", name, err)
	}
	if bounds.Minimum != nil && bounds.Maximum != nil && *bounds.Minimum > *bounds.Maximum {
		return nil, fmt.Errorf("destination_schema field %q has minimum %v above maximum %v", name, *bounds.Minimum, *bounds.Maximum)
	}
	// An integer field whose bounds hold no whole number a setting can carry
	// (see MaxSafeInteger) can never be filled in.
	if p.typ == "integer" {
		lowest, highest := float64(-MaxSafeInteger), float64(MaxSafeInteger)
		if bounds.Minimum != nil {
			lowest = math.Max(lowest, math.Ceil(*bounds.Minimum))
		}
		if bounds.Maximum != nil {
			highest = math.Min(highest, math.Floor(*bounds.Maximum))
		}
		if lowest > highest {
			return nil, fmt.Errorf("destination_schema field %q has bounds that hold no whole number a setting can carry (up to %d in either direction)", name, int64(MaxSafeInteger))
		}
	}
	for keyword, length := range map[string]*int{"minLength": bounds.MinLength, "maxLength": bounds.MaxLength} {
		if length != nil && *length < 0 {
			return nil, fmt.Errorf("destination_schema field %q has a negative %s", name, keyword)
		}
	}
	if bounds.MinLength != nil && bounds.MaxLength != nil && *bounds.MinLength > *bounds.MaxLength {
		return nil, fmt.Errorf("destination_schema field %q has minLength %d above maxLength %d", name, *bounds.MinLength, *bounds.MaxLength)
	}
	if bounds.Pattern != nil {
		if _, err := regexp.Compile(*bounds.Pattern); err != nil {
			return nil, fmt.Errorf("destination_schema field %q pattern isn't Go RE2 syntax: %w", name, err)
		}
	}
	compiled, err := config.CompileSchema(string(raw))
	if err != nil {
		return nil, fmt.Errorf("destination_schema field %q doesn't compile: %w", name, err)
	}
	p.schema = compiled
	return p, nil
}

// MaxSafeInteger is the largest whole number a setting carries exactly:
// settings travel as JSON numbers and protobuf values, which are doubles.
// Hosts refuse an integer setting beyond it in either direction.
const MaxSafeInteger = 1<<53 - 1

// checkValue reports why value can't be stored in the field: it isn't a
// scalar of the field's type, or it fails the field's own checks.
func (p *destinationProperty) checkValue(value any) error {
	ok := false
	switch v := value.(type) {
	case string:
		ok = p.typ == "string"
	case bool:
		ok = p.typ == "boolean"
	case float64:
		ok = p.typ == "number" || (p.typ == "integer" && v == math.Trunc(v))
		if ok && p.typ == "integer" && math.Abs(v) > MaxSafeInteger {
			return fmt.Errorf("is outside the whole numbers a setting can hold exactly (up to %d); use a string field for a larger identifier", int64(MaxSafeInteger))
		}
	}
	if !ok {
		return fmt.Errorf("isn't of type %s", p.typ)
	}
	if err := p.schema.Validate(value); err != nil {
		return fmt.Errorf("fails the field's checks: %#v", err)
	}
	return nil
}

// optionValue parses a select option's value as the field's type, the way
// a client sends it.
func (p *destinationProperty) optionValue(raw string) (any, error) {
	switch p.typ {
	case "string":
		return raw, nil
	case "integer":
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("isn't an integer")
		}
		if n > MaxSafeInteger || n < -MaxSafeInteger {
			return nil, fmt.Errorf("is outside the whole numbers a setting can hold exactly (up to %d); use a string field for a larger identifier", int64(MaxSafeInteger))
		}
		return float64(n), nil
	case "number":
		n, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
			return nil, fmt.Errorf("isn't a number")
		}
		return n, nil
	}
	return nil, fmt.Errorf("can't be a %s option", p.typ)
}

// validateDestinationForm checks the admin_form against the flat-form rules
// above. validateConfigSchema already checked that each form field names a
// property and that keys are unique.
func validateDestinationForm(form *pluginv1.AdminFormDescriptor, properties map[string]*destinationProperty, required []string) error {
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
		if !destinationFieldKey.MatchString(key) {
			return fmt.Errorf("destination_schema field key %q must be a letter followed by up to 63 letters, digits, underscores or hyphens", key)
		}
		if inForm[key] {
			return fmt.Errorf("destination_schema admin_form field %q is duplicated", key)
		}
		inForm[key] = true
		property := properties[key]
		if property == nil {
			return fmt.Errorf("destination_schema admin_form field %q isn't a schema field", key)
		}
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
		control := field.GetControl()
		allowed, ok := destinationControlTypes[control]
		if !ok {
			return fmt.Errorf("destination_schema admin_form field %q uses control %s; use TEXT, PASSWORD, NUMBER, SWITCH or SELECT", key, control)
		}
		if !slices.Contains(allowed, property.typ) {
			return fmt.Errorf("destination_schema admin_form field %q: control %s needs a field of type %s, not %q", key, control, strings.Join(allowed, " or "), property.typ)
		}
		if field.GetSecret() && property.typ != "string" {
			return fmt.Errorf("destination_schema admin_form field %q is secret but not a string field", key)
		}
		// A secret is typed, never picked: clients show it as a masked input.
		if field.GetSecret() && control != pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_PASSWORD && control != pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_TEXT {
			return fmt.Errorf("destination_schema admin_form field %q is secret, so its control must be PASSWORD or TEXT, not %s", key, control)
		}
		if field.GetRequired() != slices.Contains(required, key) {
			return fmt.Errorf("destination_schema admin_form field %q: required on the form field and the schema's required list must agree", key)
		}
		isSelect := control == pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_SELECT
		if isSelect && len(field.GetOptions()) == 0 {
			return fmt.Errorf("destination_schema admin_form field %q select control requires static options", key)
		}
		if !isSelect && len(field.GetOptions()) > 0 {
			return fmt.Errorf("destination_schema admin_form field %q has options, which only a SELECT control uses", key)
		}
		if _, ok := property.keywords["enum"]; ok && !isSelect {
			return fmt.Errorf("destination_schema field %q declares enum, which only a SELECT control shows; use a SELECT with those options", key)
		}
		options, err := destinationOptions(field, property)
		if err != nil {
			return err
		}
		if err := checkDestinationDefaults(field, property, options); err != nil {
			return err
		}
	}
	for name := range properties {
		if !inForm[name] {
			return fmt.Errorf("destination_schema field %q has no admin_form field", name)
		}
	}
	return nil
}

// destinationOptions parses a select's options as the field's type and
// checks that they are unique, pass the field's checks, and equal its enum
// when it has one. It returns the parsed values.
func destinationOptions(field *pluginv1.AdminFormField, property *destinationProperty) ([]any, error) {
	key := field.GetKey()
	values := make([]any, 0, len(field.GetOptions()))
	for _, option := range field.GetOptions() {
		value, err := property.optionValue(option.GetValue())
		if err != nil {
			return nil, fmt.Errorf("destination_schema admin_form field %q option %q %w", key, option.GetValue(), err)
		}
		if slices.Contains(values, value) {
			return nil, fmt.Errorf("destination_schema admin_form field %q has option %q twice", key, option.GetValue())
		}
		if err := property.checkValue(value); err != nil {
			return nil, fmt.Errorf("destination_schema admin_form field %q option %q %w", key, option.GetValue(), err)
		}
		values = append(values, value)
	}
	if raw, ok := property.keywords["enum"]; ok {
		var enum []any
		if err := json.Unmarshal(raw, &enum); err != nil {
			return nil, fmt.Errorf("destination_schema field %q enum must be a list", key)
		}
		// Every option already passed the enum; the enum may not offer a
		// value the form can't choose.
		for _, value := range enum {
			if !slices.Contains(values, value) {
				return nil, fmt.Errorf("destination_schema field %q enum value %v isn't one of its options; options and enum must match", key, value)
			}
		}
	}
	return values, nil
}

// checkDestinationDefaults checks the schema's default and the form's
// default_value: each is a scalar of the field's type that passes the
// field's checks and, on a select, is one of its options.
func checkDestinationDefaults(field *pluginv1.AdminFormField, property *destinationProperty, options []any) error {
	key := field.GetKey()
	check := func(source string, value any) error {
		if err := property.checkValue(value); err != nil {
			return fmt.Errorf("destination_schema field %q %s %w", key, source, err)
		}
		if field.GetControl() == pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_SELECT && !slices.Contains(options, value) {
			return fmt.Errorf("destination_schema field %q %s isn't one of its options", key, source)
		}
		return nil
	}
	if raw, ok := property.keywords["default"]; ok {
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("destination_schema field %q default: %w", key, err)
		}
		if err := check("default", value); err != nil {
			return err
		}
	}
	if value := field.GetDefaultValue(); value != nil {
		if err := check("default_value", value.AsInterface()); err != nil {
			return err
		}
	}
	return nil
}
