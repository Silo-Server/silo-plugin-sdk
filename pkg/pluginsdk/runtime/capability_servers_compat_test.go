package runtime

// Compile-time guard for the CapabilityServers unkeyed composite literal
// shape. v0.12 through v0.15 had twelve fields; v0.16.0 appended
// NetworkAccessProvider as the thirteenth. Plugins should use keyed literals;
// any further optional servers belong behind additive registration APIs
// (see ServeManifestOption), not here.
var _ = CapabilityServers{
	nil,
	nil,
	nil,
	nil,
	nil,
	nil,
	nil,
	nil,
	nil,
	nil,
	nil,
	nil,
	nil,
}
