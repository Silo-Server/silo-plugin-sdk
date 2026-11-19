package manifest_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"google.golang.org/protobuf/types/known/structpb"

	publicmanifest "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
)

func TestLoadNotificationChannel(t *testing.T) {
	manifest, err := publicmanifest.Load([]byte(`{
	  "plugin_id":"silo.ntfy", "version":"1.0.0", "silo_api_version":"v1",
	  "capabilities":[{
	    "type":"notification_channel.v1", "id":"ntfy",
	    "notification_channel":{
	      "display_name":"ntfy",
	      "destination_schema":{
	        "key":"destination",
	        "json_schema":"{\"type\":\"object\",\"properties\":{\"topic\":{\"type\":\"string\"},\"token\":{\"type\":\"string\"}}}",
	        "admin_form":{"fields":[{"key":"topic","label":"Topic","control":"ADMIN_FORM_CONTROL_TEXT"},{"key":"token","label":"Access token","control":"ADMIN_FORM_CONTROL_PASSWORD","secret":true}]}
	      }
	    }
	  }]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	channel := manifest.GetCapabilities()[0].GetNotificationChannel()
	if channel.GetDisplayName() != "ntfy" || len(channel.GetDestinationSchema().GetAdminForm().GetFields()) != 2 {
		t.Fatalf("notification channel descriptor = %v", channel)
	}
}

func TestValidateNotificationChannel(t *testing.T) {
	schema := &pluginv1.ConfigSchema{Key: "destination", JsonSchema: `{"type":"object","properties":{"topic":{"type":"string"}}}`}
	for _, tc := range []struct {
		name       string
		capability *pluginv1.CapabilityDescriptor
		wantErr    string
	}{
		{name: "missing descriptor", capability: &pluginv1.CapabilityDescriptor{Type: "notification_channel.v1", Id: "ntfy"}, wantErr: "descriptor is required"},
		{name: "missing display name", capability: &pluginv1.CapabilityDescriptor{Type: "notification_channel.v1", Id: "ntfy",
			NotificationChannel: &pluginv1.NotificationChannelDescriptor{DestinationSchema: schema}}, wantErr: "display_name"},
		{name: "missing schema", capability: &pluginv1.CapabilityDescriptor{Type: "notification_channel.v1", Id: "ntfy",
			NotificationChannel: &pluginv1.NotificationChannelDescriptor{DisplayName: "ntfy"}}, wantErr: "destination_schema"},
		{name: "form field not in schema", capability: &pluginv1.CapabilityDescriptor{Type: "notification_channel.v1", Id: "ntfy",
			NotificationChannel: &pluginv1.NotificationChannelDescriptor{DisplayName: "ntfy", DestinationSchema: &pluginv1.ConfigSchema{
				Key: "destination", JsonSchema: schema.JsonSchema,
				AdminForm: &pluginv1.AdminFormDescriptor{Fields: []*pluginv1.AdminFormField{{Key: "server"}}},
			}}}, wantErr: "destination schema"},
		{name: "nested object field", capability: channelWithSchema(`{"type":"object","properties":{"auth":{"type":"object","properties":{"token":{"type":"string"}}}}}`), wantErr: "scalar"},
		{name: "array field", capability: channelWithSchema(`{"type":"object","properties":{"tags":{"type":"array"}}}`), wantErr: "type string, integer"},
		{name: "ref", capability: channelWithSchema(`{"type":"object","properties":{"topic":{"$ref":"#/$defs/topic"}}}`), wantErr: "plain scalars"},
		{name: "conditional", capability: channelWithSchema(`{"type":"object","properties":{"topic":{"type":"string"}},"allOf":[{"required":["topic"]}]}`), wantErr: "flat list"},
		{name: "required field without a form field", capability: channelWithForm(`{"type":"object","properties":{"topic":{"type":"string"},"token":{"type":"string"}},"required":["token"]}`, "topic"), wantErr: `"token" has no admin_form field`},
		{name: "fields without a form", capability: channelWithSchema(`{"type":"object","properties":{"topic":{"type":"string"}}}`), wantErr: "needs an admin_form"},
		{name: "no fields", capability: channelWithForm(`{"type":"object","properties":{}}`), wantErr: "needs an admin_form"},
		{name: "additional properties allowed", capability: channelWithForm(`{"type":"object","properties":{"topic":{"type":"string"}},"additionalProperties":true}`, "topic"), wantErr: "additionalProperties"},
		{name: "additional properties schema", capability: channelWithForm(`{"type":"object","properties":{"topic":{"type":"string"}},"additionalProperties":{"type":"string"}}`, "topic"), wantErr: "additionalProperties"},
		{name: "nullable field", capability: channelWithForm(`{"type":"object","properties":{"topic":{"type":["string","null"]}}}`, "topic"), wantErr: "json_schema"},
		{name: "required names no field", capability: channelWithSchema(`{"type":"object","properties":{"topic":{"type":"string"}},"required":["server"]}`), wantErr: "isn't a field"},
		{name: "id not a slug", capability: &pluginv1.CapabilityDescriptor{Type: "notification_channel.v1", Id: "Ntfy Push",
			NotificationChannel: &pluginv1.NotificationChannelDescriptor{DisplayName: "ntfy", DestinationSchema: schema}}, wantErr: "slug"},
		{name: "descriptor on another type", capability: &pluginv1.CapabilityDescriptor{Type: "scheduled_task.v1", Id: "task",
			NotificationChannel: &pluginv1.NotificationChannelDescriptor{DisplayName: "ntfy", DestinationSchema: schema}}, wantErr: "requires type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := publicmanifest.Validate(&pluginv1.PluginManifest{PluginId: "silo.test", Version: "1.0.0",
				Capabilities: []*pluginv1.CapabilityDescriptor{tc.capability}})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func channelWithSchema(jsonSchema string) *pluginv1.CapabilityDescriptor {
	return &pluginv1.CapabilityDescriptor{Type: "notification_channel.v1", Id: "ntfy",
		NotificationChannel: &pluginv1.NotificationChannelDescriptor{DisplayName: "ntfy",
			DestinationSchema: &pluginv1.ConfigSchema{Key: "destination", JsonSchema: jsonSchema}}}
}

// channelWithForm gives each named field the control its schema type calls for.
func channelWithForm(jsonSchema string, formKeys ...string) *pluginv1.CapabilityDescriptor {
	descriptor := channelWithSchema(jsonSchema)
	var parsed struct {
		Properties map[string]struct {
			Type any `json:"type"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	_ = json.Unmarshal([]byte(jsonSchema), &parsed)
	form := &pluginv1.AdminFormDescriptor{}
	for _, key := range formKeys {
		control := pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_TEXT
		switch parsed.Properties[key].Type {
		case "integer", "number":
			control = pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_NUMBER
		case "boolean":
			control = pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_SWITCH
		}
		form.Fields = append(form.Fields, &pluginv1.AdminFormField{Key: key, Label: key, Control: control,
			Required: slices.Contains(parsed.Required, key)})
	}
	descriptor.GetNotificationChannel().GetDestinationSchema().AdminForm = form
	return descriptor
}

// TestValidateDestinationFormRefusals covers every form feature a destination
// form can't use. Each case changes one thing on an otherwise valid form.
func TestValidateDestinationFormRefusals(t *testing.T) {
	const jsonSchema = `{"type":"object","properties":{"topic":{"type":"string"},"priority":{"type":"integer"},"markdown":{"type":"boolean"}}}`
	field := func(form *pluginv1.AdminFormDescriptor, key string) *pluginv1.AdminFormField {
		for _, f := range form.GetFields() {
			if f.GetKey() == key {
				return f
			}
		}
		t.Fatalf("no field %q", key)
		return nil
	}
	for _, tc := range []struct {
		name    string
		change  func(form *pluginv1.AdminFormDescriptor)
		wantErr string
	}{
		{name: "section", change: func(form *pluginv1.AdminFormDescriptor) {
			form.Sections = []*pluginv1.AdminFormSection{{Key: "all", FieldKeys: []string{"topic", "priority", "markdown"}}}
		}, wantErr: "can't use sections"},
		{name: "submit label", change: func(form *pluginv1.AdminFormDescriptor) { form.SubmitLabel = "Save" }, wantErr: "submit_label"},
		{name: "show_when", change: func(form *pluginv1.AdminFormDescriptor) {
			field(form, "priority").ShowWhen = []*pluginv1.AdminFormCondition{{Field: "markdown", Equals: []string{"true"}}}
		}, wantErr: "show_when"},
		{name: "dynamic options", change: func(form *pluginv1.AdminFormDescriptor) {
			f := field(form, "topic")
			f.Control = pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_SELECT
			f.DynamicOptions = true
		}, wantErr: "dynamic_options"},
		{name: "dynamic options with static fallback", change: func(form *pluginv1.AdminFormDescriptor) {
			f := field(form, "topic")
			f.Control = pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_SELECT
			f.Options = []*pluginv1.AdminFormOption{{Value: "a", Label: "A"}}
			f.DynamicOptions = true
		}, wantErr: "dynamic_options"},
		{name: "exclusive group", change: func(form *pluginv1.AdminFormDescriptor) { field(form, "markdown").ExclusiveGroupField = "topic" }, wantErr: "exclusive_group_field"},
		{name: "form validation", change: func(form *pluginv1.AdminFormDescriptor) {
			field(form, "priority").Validation = &pluginv1.AdminFormValidation{HasMin: true, Min: 1}
		}, wantErr: "validation"},
		{name: "multiline", change: func(form *pluginv1.AdminFormDescriptor) { field(form, "topic").Multiline = true }, wantErr: "multiline"},
		{name: "rows", change: func(form *pluginv1.AdminFormDescriptor) { field(form, "topic").Rows = 3 }, wantErr: "rows"},
		{name: "textarea", change: func(form *pluginv1.AdminFormDescriptor) {
			field(form, "topic").Control = pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_TEXTAREA
		}, wantErr: "uses control"},
		{name: "multi select", change: func(form *pluginv1.AdminFormDescriptor) {
			f := field(form, "topic")
			f.Control = pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_MULTI_SELECT
			f.Options = []*pluginv1.AdminFormOption{{Value: "a", Label: "A"}}
		}, wantErr: "multi_select"}, // an array property, which MULTI_SELECT needs, is refused too
		{name: "no control", change: func(form *pluginv1.AdminFormDescriptor) {
			field(form, "topic").Control = pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_UNSPECIFIED
		}, wantErr: "uses control"},
		{name: "select without options", change: func(form *pluginv1.AdminFormDescriptor) {
			field(form, "topic").Control = pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_SELECT
		}, wantErr: "select control requires options"},
		{name: "options on a text field", change: func(form *pluginv1.AdminFormDescriptor) {
			field(form, "topic").Options = []*pluginv1.AdminFormOption{{Value: "a", Label: "A"}}
		}, wantErr: "only a SELECT"},
		{name: "switch on a string", change: func(form *pluginv1.AdminFormDescriptor) {
			field(form, "topic").Control = pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_SWITCH
		}, wantErr: "needs a field of type boolean"},
		{name: "number on a boolean", change: func(form *pluginv1.AdminFormDescriptor) {
			field(form, "markdown").Control = pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_NUMBER
		}, wantErr: "needs a field of type integer or number"},
		{name: "select on a boolean", change: func(form *pluginv1.AdminFormDescriptor) {
			f := field(form, "markdown")
			f.Control = pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_SELECT
			f.Options = []*pluginv1.AdminFormOption{{Value: "true", Label: "Yes"}}
		}, wantErr: "needs a field of type string or integer or number"},
		{name: "password on a number", change: func(form *pluginv1.AdminFormDescriptor) {
			field(form, "priority").Control = pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_PASSWORD
		}, wantErr: "needs a field of type string"},
		{name: "secret switch", change: func(form *pluginv1.AdminFormDescriptor) { field(form, "markdown").Secret = true }, wantErr: "secret but not a string"},
		{name: "no label", change: func(form *pluginv1.AdminFormDescriptor) { field(form, "topic").Label = " " }, wantErr: "needs a label"},
		{name: "empty field", change: func(form *pluginv1.AdminFormDescriptor) { form.Fields = append(form.Fields, nil) }, wantErr: "empty field"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capability := channelWithForm(jsonSchema, "topic", "priority", "markdown")
			tc.change(capability.GetNotificationChannel().GetDestinationSchema().GetAdminForm())
			err := publicmanifest.Validate(&pluginv1.PluginManifest{PluginId: "silo.test", Version: "1.0.0",
				Capabilities: []*pluginv1.CapabilityDescriptor{capability}})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateDestinationSchemaAcceptsFlatForm(t *testing.T) {
	for _, jsonSchema := range []string{
		`{"type":"object","properties":{"topic":{"type":"string","minLength":1},"priority":{"type":"integer","minimum":1,"maximum":5},"markdown":{"type":"boolean"}},"required":["topic"],"additionalProperties":false}`,
		`{"type":"object","properties":{"topic":{"type":"string"},"priority":{"type":"integer"},"markdown":{"type":"boolean"}}}`,
	} {
		err := publicmanifest.Validate(&pluginv1.PluginManifest{PluginId: "silo.test", Version: "1.0.0",
			Capabilities: []*pluginv1.CapabilityDescriptor{channelWithForm(jsonSchema, "topic", "priority", "markdown")}})
		if err != nil {
			t.Fatalf("Validate(%s) = %v", jsonSchema, err)
		}
	}
}

// everyControlSchema and everyControlForm are a destination form that uses
// every allowed control with options, defaults, enum and bounds.
const everyControlSchema = `{"type":"object","properties":{` +
	`"topic":{"type":"string","title":"Topic","minLength":1,"maxLength":64,"pattern":"^[A-Za-z0-9_-]+$","default":"alerts"},` +
	`"token":{"type":"string","format":"password"},` +
	`"priority":{"type":"integer","minimum":1,"maximum":5,"default":3},` +
	`"ratio":{"type":"number","minimum":0,"maximum":1},` +
	`"markdown":{"type":"boolean","default":true},` +
	`"server":{"type":"string","enum":["a","b"],"default":"a"},` +
	`"level":{"type":"integer","enum":[1,2]}},` +
	`"required":["topic"],"additionalProperties":false}`

func everyControlForm() *pluginv1.AdminFormDescriptor {
	return &pluginv1.AdminFormDescriptor{Fields: []*pluginv1.AdminFormField{
		{Key: "topic", Label: "Topic", Description: "Your topic.", Placeholder: "alerts", Required: true, Control: pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_TEXT, DefaultValue: structpb.NewStringValue("alerts")},
		{Key: "token", Label: "Token", Control: pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_PASSWORD, Secret: true},
		{Key: "priority", Label: "Priority", Control: pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_NUMBER, DefaultValue: structpb.NewNumberValue(3)},
		{Key: "ratio", Label: "Ratio", Control: pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_NUMBER, DefaultValue: structpb.NewNumberValue(0.5)},
		{Key: "markdown", Label: "Markdown", Control: pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_SWITCH, DefaultValue: structpb.NewBoolValue(true)},
		{Key: "server", Label: "Server", Control: pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_SELECT, Options: []*pluginv1.AdminFormOption{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}}},
		{Key: "level", Label: "Level", Control: pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_SELECT, DefaultValue: structpb.NewNumberValue(2), Options: []*pluginv1.AdminFormOption{{Value: "1", Label: "Low"}, {Value: "2", Label: "High"}}},
	}}
}

func validateChannel(jsonSchema string, form *pluginv1.AdminFormDescriptor) error {
	capability := channelWithSchema(jsonSchema)
	capability.GetNotificationChannel().GetDestinationSchema().AdminForm = form
	return publicmanifest.Validate(&pluginv1.PluginManifest{PluginId: "silo.test", Version: "1.0.0",
		Capabilities: []*pluginv1.CapabilityDescriptor{capability}})
}

func TestValidateDestinationSchemaAcceptsEveryAllowedControl(t *testing.T) {
	if err := validateChannel(everyControlSchema, everyControlForm()); err != nil {
		t.Fatal(err)
	}
}

// TestValidateDestinationSchemaRefusesUnsavableForms covers forms that would
// install but could never be rendered or saved. Each case changes one thing
// on the form TestValidateDestinationSchemaAcceptsEveryAllowedControl accepts.
func TestValidateDestinationSchemaRefusesUnsavableForms(t *testing.T) {
	field := func(form *pluginv1.AdminFormDescriptor, key string) *pluginv1.AdminFormField {
		for _, f := range form.GetFields() {
			if f.GetKey() == key {
				return f
			}
		}
		t.Fatalf("no field %q", key)
		return nil
	}
	for _, tc := range []struct {
		name    string
		schema  func(string) string // nil keeps the schema
		form    func(*pluginv1.AdminFormDescriptor)
		rekey   [2]string // renames a field in the schema and the form
		wantErr string
	}{
		{name: "pattern outside RE2", schema: replace(`"^[A-Za-z0-9_-]+$"`, `"^(?!foo)"`), wantErr: "isn't Go RE2 syntax"},
		{name: "$schema", schema: replace(`{"type":"object",`, `{"$schema":"http://json-schema.org/draft-07/schema#","type":"object",`), wantErr: "can't set $schema"},
		{name: "integer option not an integer", form: func(form *pluginv1.AdminFormDescriptor) {
			field(form, "level").Options[0].Value = "abc"
		}, wantErr: `option "abc" isn't an integer`},
		{name: "option outside enum", form: func(form *pluginv1.AdminFormDescriptor) {
			field(form, "server").Options[1].Value = "c"
		}, wantErr: `option "c" fails the field's checks`},
		{name: "enum value without an option", form: func(form *pluginv1.AdminFormDescriptor) {
			f := field(form, "server")
			f.Options = f.Options[:1]
		}, wantErr: "options and enum must match"},
		{name: "duplicate option", form: func(form *pluginv1.AdminFormDescriptor) {
			field(form, "level").Options[1].Value = "01"
		}, wantErr: "twice"},
		{name: "option fails a bound", schema: replace(`"level":{"type":"integer","enum":[1,2]}`, `"level":{"type":"integer","minimum":2}`),
			wantErr: `option "1" fails the field's checks`},
		{name: "enum on a text field", schema: replace(`"title":"Topic",`, `"title":"Topic","enum":["alerts"],`), wantErr: "declares enum"},
		{name: "default_value of another type", form: func(form *pluginv1.AdminFormDescriptor) {
			field(form, "priority").DefaultValue = structpb.NewStringValue("3")
		}, wantErr: "default_value must be numeric"},
		{name: "fractional integer default_value", form: func(form *pluginv1.AdminFormDescriptor) {
			field(form, "priority").DefaultValue = structpb.NewNumberValue(2.5)
		}, wantErr: "default_value isn't of type integer"},
		{name: "default_value out of range", form: func(form *pluginv1.AdminFormDescriptor) {
			field(form, "priority").DefaultValue = structpb.NewNumberValue(9)
		}, wantErr: "default_value fails the field's checks"},
		{name: "default_value fails pattern", form: func(form *pluginv1.AdminFormDescriptor) {
			field(form, "topic").DefaultValue = structpb.NewStringValue("no spaces")
		}, wantErr: "default_value fails the field's checks"},
		{name: "select default_value not an option", form: func(form *pluginv1.AdminFormDescriptor) {
			field(form, "level").DefaultValue = structpb.NewNumberValue(3)
		}, wantErr: "default_value fails the field's checks"},
		{name: "select default not an option without enum", schema: replace(`"level":{"type":"integer","enum":[1,2]}`, `"level":{"type":"integer","default":3}`),
			wantErr: "default isn't one of its options"},
		{name: "schema default of another type", schema: replace(`"default":3`, `"default":"3"`), wantErr: "default isn't of type integer"},
		{name: "schema default object", schema: replace(`"default":"alerts"`, `"default":{}`), wantErr: "default isn't of type string"},
		{name: "schema default out of range", schema: replace(`"default":3`, `"default":0`), wantErr: "default fails the field's checks"},
		{name: "required only on the form", form: func(form *pluginv1.AdminFormDescriptor) {
			field(form, "token").Required = true
		}, wantErr: "must agree"},
		{name: "required only in the schema", form: func(form *pluginv1.AdminFormDescriptor) {
			field(form, "topic").Required = false
		}, wantErr: "must agree"},
		{name: "key with trailing space", rekey: [2]string{"token", "token "}, wantErr: "must be a letter followed by"},
		{name: "key with a dot", rekey: [2]string{"token", "a.b"}, wantErr: "must be a letter followed by"},
		{name: "key starting with a digit", rekey: [2]string{"token", "1token"}, wantErr: "must be a letter followed by"},
		{name: "duplicate key", form: func(form *pluginv1.AdminFormDescriptor) {
			form.Fields = append(form.Fields, &pluginv1.AdminFormField{Key: "token", Label: "Again", Control: pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_PASSWORD})
		}, wantErr: "duplicated"},
		{name: "minimum above maximum", schema: replace(`"minimum":1,"maximum":5`, `"minimum":5,"maximum":1`), wantErr: "minimum 5 above maximum 1"},
		{name: "minLength above maxLength", schema: replace(`"minLength":1,"maxLength":64`, `"minLength":65,"maxLength":64`), wantErr: "minLength 65 above maxLength 64"},
		{name: "negative length", schema: replace(`"minLength":1`, `"minLength":-1`), wantErr: "negative minLength"},
		{name: "minimum on a string", schema: replace(`"format":"password"`, `"minimum":1`), wantErr: "applies only to an integer or number field"},
		{name: "pattern on a number", schema: replace(`"type":"number","minimum":0`, `"type":"number","pattern":"x","minimum":0`), wantErr: "applies only to a string field"},
		{name: "format on a boolean", schema: replace(`"type":"boolean"`, `"type":"boolean","format":"date"`), wantErr: "applies only to a string field"},
		{name: "secret number", form: func(form *pluginv1.AdminFormDescriptor) { field(form, "priority").Secret = true }, wantErr: "secret but not a string"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			jsonSchema := everyControlSchema
			if tc.schema != nil {
				jsonSchema = tc.schema(jsonSchema)
				if jsonSchema == everyControlSchema {
					t.Fatal("schema change didn't apply")
				}
			}
			form := everyControlForm()
			if from, to := tc.rekey[0], tc.rekey[1]; from != "" {
				jsonSchema = strings.Replace(jsonSchema, `"`+from+`":`, `"`+to+`":`, 1)
				field(form, from).Key = to
			}
			if tc.form != nil {
				tc.form(form)
			}
			err := validateChannel(jsonSchema, form)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

// replace returns a schema change that replaces old with new once.
func replace(old, new string) func(string) string {
	return func(s string) string { return strings.Replace(s, old, new, 1) }
}
