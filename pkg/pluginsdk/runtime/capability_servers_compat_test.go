package runtime

// Compile-time guard for the released v0.12 unkeyed composite literal shape.
// New optional servers belong behind additive registration APIs, not here.
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
}
