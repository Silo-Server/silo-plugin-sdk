package manifest

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protojson"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/capability"
)

// knownCapabilityTypes is the set of capability type strings recognised by
// the manifest validator. Add new capability types here as they are defined.
//
// Note: capability types are recorded purely for discovery — the host exposes
// them via the admin plugins API so SPAs can filter by them client-side.
// They do not drive any dispatch behavior on the server side.
var (
	knownCapabilityTypes = knownCapabilityTypeSet()
	watchSyncSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
)

const (
	maxPresentationDisplayNameRunes = 120
	maxPresentationSummaryRunes     = 240
	maxPresentationMarkdownBytes    = 32 << 10
	maxPresentationURLBytes         = 2048
	maxPresentationIdentityRunes    = 120
)

func knownCapabilityTypeSet() map[string]struct{} {
	out := make(map[string]struct{}, len(capability.KnownTypes))
	for _, typ := range capability.KnownTypes {
		out[typ] = struct{}{}
	}
	return out
}

func Load(data []byte) (*pluginv1.PluginManifest, error) {
	manifest, err := decode(data)
	if err != nil {
		return nil, err
	}
	if err := Validate(manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func MustLoad(data []byte) *pluginv1.PluginManifest {
	manifest, err := Load(data)
	if err != nil {
		panic(err)
	}
	return manifest
}

func LoadFromDisk(path string) (*pluginv1.PluginManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read plugin manifest %q: %w", path, err)
	}
	manifest, err := Load(data)
	if err != nil {
		return nil, fmt.Errorf("load plugin manifest %q: %w", path, err)
	}
	return manifest, nil
}

func decode(data []byte) (*pluginv1.PluginManifest, error) {
	var manifest pluginv1.PluginManifest
	// Use DiscardUnknown so that fields defined outside the proto schema do
	// not cause a decode error. The proto-schema fields are still fully
	// decoded.
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode plugin manifest: %w", err)
	}
	return &manifest, nil
}

// Validate validates the proto plugin manifest.
func Validate(manifest *pluginv1.PluginManifest) error {
	if manifest == nil {
		return fmt.Errorf("plugin manifest is required")
	}
	if manifest.PluginId == "" {
		return fmt.Errorf("plugin manifest plugin_id is required")
	}
	if manifest.Version == "" {
		return fmt.Errorf("plugin manifest version is required")
	}
	if err := validatePresentation(manifest.GetPresentation()); err != nil {
		return err
	}
	for _, capability := range manifest.Capabilities {
		if capability.Type == "" {
			return fmt.Errorf("plugin capability type is required")
		}
		if capability.Id == "" {
			return fmt.Errorf("plugin capability id is required")
		}
		if _, ok := knownCapabilityTypes[capability.Type]; !ok {
			return fmt.Errorf("plugin capability %q: unknown type %q", capability.Id, capability.Type)
		}
		if err := validateWatchSyncCapability(capability); err != nil {
			return err
		}
	}
	for _, schema := range manifest.GlobalConfigSchema {
		if err := validateConfigSchema(schema); err != nil {
			return err
		}
	}
	for _, schema := range manifest.UserConfigSchema {
		if err := validateConfigSchema(schema); err != nil {
			return err
		}
	}
	for _, c := range manifest.GetCapabilities() {
		for _, cs := range c.GetConfigSchema() {
			if err := validateConfigSchema(cs); err != nil {
				return fmt.Errorf("capability %q config schema: %w", c.GetId(), err)
			}
		}
	}
	return nil
}

func validateWatchSyncCapability(descriptor *pluginv1.CapabilityDescriptor) error {
	watchSync := descriptor.GetWatchSyncProvider()
	if descriptor.GetType() != capability.WatchSyncProvider {
		if watchSync != nil {
			return fmt.Errorf("plugin capability %q: watch_sync_provider descriptor requires type %q", descriptor.GetId(), capability.WatchSyncProvider)
		}
		return nil
	}
	if !watchSyncSlugPattern.MatchString(descriptor.GetId()) {
		return fmt.Errorf("plugin capability %q: watch sync capability id must be a path-safe lowercase slug", descriptor.GetId())
	}
	if watchSync == nil {
		return fmt.Errorf("plugin capability %q: watch_sync_provider descriptor is required", descriptor.GetId())
	}
	if len(watchSync.GetAuthMethods()) == 0 {
		return fmt.Errorf("plugin capability %q: at least one watch sync auth method is required", descriptor.GetId())
	}
	for _, method := range watchSync.GetAuthMethods() {
		if method == pluginv1.WatchSyncAuthMethod_WATCH_SYNC_AUTH_METHOD_UNSPECIFIED {
			return fmt.Errorf("plugin capability %q: watch sync auth method cannot be unspecified", descriptor.GetId())
		}
	}
	if !watchSync.GetExportWatched() && !watchSync.GetExportUnwatched() && !watchSync.GetImportWatched() && !watchSync.GetImportProgress() {
		return fmt.Errorf("plugin capability %q: at least one watch sync operation is required", descriptor.GetId())
	}
	if watchSync.GetMaxBatchSize() < 1 || watchSync.GetMaxBatchSize() > 100 {
		return fmt.Errorf("plugin capability %q: watch sync max_batch_size must be between 1 and 100", descriptor.GetId())
	}
	if len(watchSync.GetSupportedMediaTypes()) == 0 {
		return fmt.Errorf("plugin capability %q: at least one supported media type is required", descriptor.GetId())
	}
	for _, mediaType := range watchSync.GetSupportedMediaTypes() {
		if mediaType == pluginv1.WatchSyncMediaType_WATCH_SYNC_MEDIA_TYPE_UNSPECIFIED {
			return fmt.Errorf("plugin capability %q: watch sync media type cannot be unspecified", descriptor.GetId())
		}
	}
	for _, namespace := range watchSync.GetExternalIdNamespaces() {
		if !watchSyncSlugPattern.MatchString(namespace) {
			return fmt.Errorf("plugin capability %q: invalid external id namespace %q", descriptor.GetId(), namespace)
		}
	}
	return nil
}

// ValidateCatalogPresentation applies the stricter presentation contract used
// by curated catalogs. The general manifest validator keeps presentation
// optional so older and directly uploaded plugins remain compatible.
func ValidateCatalogPresentation(manifest *pluginv1.PluginManifest, expectedSourceURL string) error {
	if err := Validate(manifest); err != nil {
		return err
	}
	presentation := manifest.GetPresentation()
	if presentation == nil {
		return fmt.Errorf("plugin presentation is required for catalog publishing")
	}

	requiredFields := []struct {
		name  string
		value string
	}{
		{name: "display_name", value: presentation.GetDisplayName()},
		{name: "summary", value: presentation.GetSummary()},
		{name: "description_markdown", value: presentation.GetDescriptionMarkdown()},
		{name: "setup_markdown", value: presentation.GetSetupMarkdown()},
		{name: "homepage_url", value: presentation.GetHomepageUrl()},
		{name: "source_url", value: presentation.GetSourceUrl()},
		{name: "support_url", value: presentation.GetSupportUrl()},
		{name: "changelog_url", value: presentation.GetChangelogUrl()},
		{name: "publisher_name", value: presentation.GetPublisherName()},
		{name: "publisher_url", value: presentation.GetPublisherUrl()},
		{name: "license_spdx", value: presentation.GetLicenseSpdx()},
	}
	for _, field := range requiredFields {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("plugin presentation %s is required for catalog publishing", field.name)
		}
	}

	if expectedSourceURL != "" && !equalCanonicalURL(presentation.GetSourceUrl(), expectedSourceURL) {
		return fmt.Errorf("plugin presentation source_url %q must match catalog repository %q", presentation.GetSourceUrl(), expectedSourceURL)
	}
	return nil
}

func validatePresentation(presentation *pluginv1.PluginPresentation) error {
	if presentation == nil {
		return nil
	}

	singleLineFields := []struct {
		name  string
		value string
		limit int
	}{
		{name: "display_name", value: presentation.GetDisplayName(), limit: maxPresentationDisplayNameRunes},
		{name: "summary", value: presentation.GetSummary(), limit: maxPresentationSummaryRunes},
		{name: "publisher_name", value: presentation.GetPublisherName(), limit: maxPresentationIdentityRunes},
		{name: "license_spdx", value: presentation.GetLicenseSpdx(), limit: maxPresentationIdentityRunes},
	}
	for _, field := range singleLineFields {
		if field.value != "" && field.value != strings.TrimSpace(field.value) {
			return fmt.Errorf("plugin presentation %s must not have leading or trailing whitespace", field.name)
		}
		if utf8.RuneCountInString(field.value) > field.limit {
			return fmt.Errorf("plugin presentation %s exceeds %d characters", field.name, field.limit)
		}
		if hasDisallowedControl(field.value, false) {
			return fmt.Errorf("plugin presentation %s contains control characters", field.name)
		}
	}

	markdownFields := []struct {
		name  string
		value string
	}{
		{name: "description_markdown", value: presentation.GetDescriptionMarkdown()},
		{name: "setup_markdown", value: presentation.GetSetupMarkdown()},
	}
	for _, field := range markdownFields {
		if len(field.value) > maxPresentationMarkdownBytes {
			return fmt.Errorf("plugin presentation %s exceeds %d bytes", field.name, maxPresentationMarkdownBytes)
		}
		if hasDisallowedControl(field.value, true) {
			return fmt.Errorf("plugin presentation %s contains control characters", field.name)
		}
	}

	urlFields := []struct {
		name  string
		value string
	}{
		{name: "homepage_url", value: presentation.GetHomepageUrl()},
		{name: "source_url", value: presentation.GetSourceUrl()},
		{name: "support_url", value: presentation.GetSupportUrl()},
		{name: "changelog_url", value: presentation.GetChangelogUrl()},
		{name: "publisher_url", value: presentation.GetPublisherUrl()},
	}
	for _, field := range urlFields {
		if err := validatePresentationURL(field.value); err != nil {
			return fmt.Errorf("plugin presentation %s: %w", field.name, err)
		}
	}
	return nil
}

func validatePresentationURL(raw string) error {
	if raw == "" {
		return nil
	}
	if len(raw) > maxPresentationURLBytes {
		return fmt.Errorf("exceeds %d bytes", maxPresentationURLBytes)
	}
	if raw != strings.TrimSpace(raw) || strings.ContainsAny(raw, " \t\r\n") || hasDisallowedControl(raw, false) {
		return fmt.Errorf("contains whitespace or control characters")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("must be a valid absolute http or https URL: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("must be an absolute http or https URL")
	}
	if parsed.User != nil {
		return fmt.Errorf("must not contain user credentials")
	}
	return nil
}

func equalCanonicalURL(left, right string) bool {
	return strings.EqualFold(strings.TrimSuffix(left, "/"), strings.TrimSuffix(right, "/"))
}

func hasDisallowedControl(value string, allowMarkdownWhitespace bool) bool {
	for _, char := range value {
		if !unicode.IsControl(char) {
			continue
		}
		if allowMarkdownWhitespace && (char == '\n' || char == '\r' || char == '\t') {
			continue
		}
		return true
	}
	return false
}

func validateConfigSchema(schema *pluginv1.ConfigSchema) error {
	if schema == nil {
		return nil
	}
	form := schema.GetAdminForm()
	if form == nil {
		return nil
	}

	var parsed struct {
		Type       string `json:"type"`
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(schema.GetJsonSchema()), &parsed); err != nil {
		return fmt.Errorf("plugin config schema %q has invalid json_schema: %w", schema.GetKey(), err)
	}
	if parsed.Type != "object" {
		return fmt.Errorf("plugin config schema %q admin_form requires an object json_schema", schema.GetKey())
	}

	seen := make(map[string]struct{}, len(form.GetFields()))
	for _, field := range form.GetFields() {
		if field == nil {
			continue
		}
		key := field.GetKey()
		if key == "" {
			return fmt.Errorf("plugin config schema %q admin_form field key is required", schema.GetKey())
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("plugin config schema %q admin_form field %q is duplicated", schema.GetKey(), key)
		}
		seen[key] = struct{}{}
		property, ok := parsed.Properties[key]
		if !ok {
			return fmt.Errorf("plugin config schema %q admin_form field %q is not declared in json_schema", schema.GetKey(), key)
		}
		switch field.GetControl() {
		case pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_MULTI_SELECT:
			if property.Type != "array" {
				return fmt.Errorf("plugin config schema %q admin_form field %q multi_select control requires an array json_schema property", schema.GetKey(), key)
			}
			// A static multi_select must enumerate its options. A field declaring
			// dynamic_options is exempt: the plugin supplies options at runtime
			// via ListConfigOptions, so it legitimately carries no static set.
			if !field.GetDynamicOptions() && len(field.GetOptions()) == 0 {
				return fmt.Errorf("plugin config schema %q admin_form field %q select control requires options", schema.GetKey(), key)
			}
		case pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_SELECT:
			// A static select must enumerate its options. A field declaring
			// dynamic_options is exempt: the plugin supplies options at runtime
			// via ListConfigOptions, so it legitimately carries no static set.
			// No type constraint: SELECT value may be string or numeric.
			if !field.GetDynamicOptions() && len(field.GetOptions()) == 0 {
				return fmt.Errorf("plugin config schema %q admin_form field %q select control requires options", schema.GetKey(), key)
			}
		}
		if defaultValue := field.GetDefaultValue(); defaultValue != nil {
			switch property.Type {
			case "boolean":
				if _, ok := defaultValue.AsInterface().(bool); !ok {
					return fmt.Errorf("plugin config schema %q admin_form field %q default_value must be boolean", schema.GetKey(), key)
				}
			case "integer", "number":
				switch defaultValue.AsInterface().(type) {
				case float64:
				default:
					return fmt.Errorf("plugin config schema %q admin_form field %q default_value must be numeric", schema.GetKey(), key)
				}
			case "string":
				if _, ok := defaultValue.AsInterface().(string); !ok {
					return fmt.Errorf("plugin config schema %q admin_form field %q default_value must be string", schema.GetKey(), key)
				}
			}
		}
	}

	// Cross-field references are validated in a second pass so a field may
	// reference another declared later (e.g. show_when pointing at a field below).
	hasReference := func(key string) bool {
		if _, ok := seen[key]; ok {
			return true
		}
		_, ok := parsed.Properties[key]
		return ok
	}
	for _, field := range form.GetFields() {
		if field == nil {
			continue
		}
		key := field.GetKey()
		for _, cond := range field.GetShowWhen() {
			if cond.GetField() == "" {
				return fmt.Errorf("plugin config schema %q admin_form field %q show_when.field is required", schema.GetKey(), key)
			}
			if !hasReference(cond.GetField()) {
				return fmt.Errorf("plugin config schema %q admin_form field %q show_when references unknown field %q", schema.GetKey(), key, cond.GetField())
			}
		}
		if eg := field.GetExclusiveGroupField(); eg != "" {
			if !hasReference(eg) {
				return fmt.Errorf("plugin config schema %q admin_form field %q exclusive_group_field references unknown field %q", schema.GetKey(), key, eg)
			}
		}
	}
	for _, section := range form.GetSections() {
		if section == nil {
			continue
		}
		for _, fk := range section.GetFieldKeys() {
			if _, ok := seen[fk]; !ok {
				return fmt.Errorf("plugin config schema %q admin_form section %q references unknown field %q", schema.GetKey(), section.GetKey(), fk)
			}
		}
		for _, cond := range section.GetShowWhen() {
			if cond.GetField() == "" {
				return fmt.Errorf("plugin config schema %q admin_form section %q show_when.field is required", schema.GetKey(), section.GetKey())
			}
			if !hasReference(cond.GetField()) {
				return fmt.Errorf("plugin config schema %q admin_form section %q show_when references unknown field %q", schema.GetKey(), section.GetKey(), cond.GetField())
			}
		}
	}
	return nil
}

func RegisterHTTPRoutes(manifest *pluginv1.PluginManifest, routes ...*pluginv1.HttpRouteDescriptor) error {
	if err := Validate(manifest); err != nil {
		return err
	}
	manifest.HttpRoutes = append(manifest.HttpRoutes, routes...)
	return nil
}

func RegisterAssets(manifest *pluginv1.PluginManifest, assets ...*pluginv1.PackagedAsset) error {
	if err := Validate(manifest); err != nil {
		return err
	}
	manifest.Assets = append(manifest.Assets, assets...)
	return nil
}

func Asset(path string, filesystem fs.FS) (*pluginv1.PackagedAsset, error) {
	if _, err := fs.Stat(filesystem, path); err != nil {
		return nil, fmt.Errorf("plugin asset %q: %w", path, err)
	}
	return &pluginv1.PackagedAsset{Path: path}, nil
}
