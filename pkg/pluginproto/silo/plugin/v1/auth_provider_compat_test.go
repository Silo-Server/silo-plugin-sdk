package pluginv1_test

import (
	"context"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

// legacyDirectAuthProvider implements AuthProviderServer without embedding
// UnimplementedAuthProviderServer, as a plugin generated with
// require_unimplemented_servers=false may. Keep this assertion so a later RPC
// cannot silently break source compatibility; new auth RPCs belong on
// AuthProviderChecks.
type legacyDirectAuthProvider struct{}

func (legacyDirectAuthProvider) Authenticate(context.Context, *pluginv1.AuthenticateRequest) (*pluginv1.AuthenticateResponse, error) {
	return nil, nil
}

func (legacyDirectAuthProvider) InitAuthorize(context.Context, *pluginv1.InitAuthorizeRequest) (*pluginv1.InitAuthorizeResponse, error) {
	return nil, nil
}

func (legacyDirectAuthProvider) ExchangeCode(context.Context, *pluginv1.ExchangeCodeRequest) (*pluginv1.AuthenticateResponse, error) {
	return nil, nil
}

func (legacyDirectAuthProvider) RefreshSession(context.Context, *pluginv1.RefreshSessionRequest) (*pluginv1.AuthenticateResponse, error) {
	return nil, nil
}

var _ pluginv1.AuthProviderServer = legacyDirectAuthProvider{}

// legacyAuthenticateResponse encodes the four fields a plugin built before
// v0.22.0 can send.
func legacyAuthenticateResponse(t *testing.T) []byte {
	t.Helper()
	claims, err := proto.Marshal(&structpb.Struct{})
	if err != nil {
		t.Fatal(err)
	}
	var b []byte
	b = protowire.AppendTag(b, 1, protowire.BytesType)
	b = protowire.AppendString(b, "sub-1")
	b = protowire.AppendTag(b, 2, protowire.BytesType)
	b = protowire.AppendString(b, "Ada")
	b = protowire.AppendTag(b, 3, protowire.BytesType)
	b = protowire.AppendString(b, "ada@example.com")
	b = protowire.AppendTag(b, 4, protowire.BytesType)
	b = protowire.AppendBytes(b, claims)
	return b
}

func TestAuthenticateResponseFromLegacyPluginLeavesNewFieldsUnset(t *testing.T) {
	var got pluginv1.AuthenticateResponse
	if err := proto.Unmarshal(legacyAuthenticateResponse(t), &got); err != nil {
		t.Fatalf("unmarshal legacy response: %v", err)
	}
	if got.GetExternalSubject() != "sub-1" || got.GetDisplayName() != "Ada" || got.GetEmail() != "ada@example.com" {
		t.Fatalf("legacy fields = %v", &got)
	}
	if got.EmailVerified != nil {
		t.Fatalf("email_verified = %v, want unset", *got.EmailVerified)
	}
	if got.GetRefreshState() != nil {
		t.Fatalf("refresh_state = %v, want unset", got.GetRefreshState())
	}
	if got.GetManagedRole() != pluginv1.AuthManagedRole_AUTH_MANAGED_ROLE_UNSPECIFIED {
		t.Fatalf("managed_role = %v, want UNSPECIFIED", got.GetManagedRole())
	}
	if got.GetDenial() != pluginv1.AuthDenial_AUTH_DENIAL_UNSPECIFIED {
		t.Fatalf("denial = %v, want UNSPECIFIED", got.GetDenial())
	}
	if got.GetIssuer() != "" || got.GetUsername() != "" || got.GetPictureUrl() != "" ||
		len(got.GetGroups()) != 0 || got.GetDenialDetail() != "" {
		t.Fatalf("new string fields set on legacy response: %v", &got)
	}
}

func TestAuthenticateResponseEmailVerifiedPresence(t *testing.T) {
	data, err := proto.Marshal(&pluginv1.AuthenticateResponse{EmailVerified: proto.Bool(false)})
	if err != nil {
		t.Fatal(err)
	}
	var got pluginv1.AuthenticateResponse
	if err := proto.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.EmailVerified == nil || *got.EmailVerified {
		t.Fatalf("explicit false email_verified = %v, want present false", got.EmailVerified)
	}
}

func TestAuthenticateResponseRefreshStateEmptyIsPresent(t *testing.T) {
	data, err := proto.Marshal(&pluginv1.AuthenticateResponse{RefreshState: &structpb.Struct{}})
	if err != nil {
		t.Fatal(err)
	}
	var got pluginv1.AuthenticateResponse
	if err := proto.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.GetRefreshState() == nil {
		t.Fatal("empty refresh_state decoded as unset; an empty Struct must stay present so it can clear stored state")
	}
}

func TestAuthenticateResponseNewFieldsRoundTrip(t *testing.T) {
	refresh, err := structpb.NewStruct(map[string]any{"refresh_token": "rt-1"})
	if err != nil {
		t.Fatal(err)
	}
	want := &pluginv1.AuthenticateResponse{
		ExternalSubject: "sub-1",
		Issuer:          "https://id.example",
		Username:        "ada",
		EmailVerified:   proto.Bool(true),
		Groups:          []string{"media", "admins"},
		PictureUrl:      "https://id.example/ada.png",
		ManagedRole:     pluginv1.AuthManagedRole_AUTH_MANAGED_ROLE_ADMIN,
		RefreshState:    refresh,
	}
	for name, codec := range map[string]struct {
		marshal   func(proto.Message) ([]byte, error)
		unmarshal func([]byte, proto.Message) error
	}{
		"binary": {proto.Marshal, proto.Unmarshal},
		"json":   {protojson.Marshal, protojson.Unmarshal},
	} {
		data, err := codec.marshal(want)
		if err != nil {
			t.Fatalf("%s marshal: %v", name, err)
		}
		got := &pluginv1.AuthenticateResponse{}
		if err := codec.unmarshal(data, got); err != nil {
			t.Fatalf("%s unmarshal: %v", name, err)
		}
		if !proto.Equal(got, want) {
			t.Fatalf("%s round trip = %v, want %v", name, got, want)
		}
	}
}

func TestInitAuthorizeRequestPromptAndLoginHint(t *testing.T) {
	data, err := proto.Marshal(&pluginv1.InitAuthorizeRequest{
		RedirectUri: "https://silo.example/callback",
		Prompt:      "select_account",
		LoginHint:   "ada@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	var got pluginv1.InitAuthorizeRequest
	if err := proto.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.GetPrompt() != "select_account" || got.GetLoginHint() != "ada@example.com" {
		t.Fatalf("prompt/login_hint = %q/%q", got.GetPrompt(), got.GetLoginHint())
	}
}

// A plugin that returns an empty CheckAccountResponse sends zero bytes. It must
// decode as UNSPECIFIED, which hosts handle like UNAVAILABLE and never as
// ACTIVE.
func TestCheckAccountResponseZeroValueIsUnspecified(t *testing.T) {
	data, err := proto.Marshal(&pluginv1.CheckAccountResponse{})
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("zero-value response encodes to %d bytes, want 0", len(data))
	}
	var got pluginv1.CheckAccountResponse
	if err := proto.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.GetStatus() != pluginv1.CheckAccountStatus_CHECK_ACCOUNT_STATUS_UNSPECIFIED {
		t.Fatalf("status = %v, want UNSPECIFIED", got.GetStatus())
	}
	if got.GetAccount() != nil {
		t.Fatalf("account = %v, want unset", got.GetAccount())
	}
}

// Enum numbers are wire contract; they must never move.
func TestAuthEnumNumbersAreStable(t *testing.T) {
	cases := []struct {
		value interface {
			Number() protoreflect.EnumNumber
		}
		want protoreflect.EnumNumber
	}{
		{pluginv1.AuthManagedRole_AUTH_MANAGED_ROLE_USER, 1},
		{pluginv1.AuthManagedRole_AUTH_MANAGED_ROLE_ADMIN, 2},
		{pluginv1.AuthDenial_AUTH_DENIAL_INVALID_CREDENTIALS, 1},
		{pluginv1.AuthDenial_AUTH_DENIAL_NOT_PERMITTED, 2},
		{pluginv1.AuthDenial_AUTH_DENIAL_ACCOUNT_DISABLED, 3},
		{pluginv1.AuthDenial_AUTH_DENIAL_PASSWORD_EXPIRED, 4},
		{pluginv1.AuthDenial_AUTH_DENIAL_PROVIDER_UNAVAILABLE, 5},
		{pluginv1.CheckAccountStatus_CHECK_ACCOUNT_STATUS_ACTIVE, 1},
		{pluginv1.CheckAccountStatus_CHECK_ACCOUNT_STATUS_NOT_FOUND, 2},
		{pluginv1.CheckAccountStatus_CHECK_ACCOUNT_STATUS_DISABLED, 3},
		{pluginv1.CheckAccountStatus_CHECK_ACCOUNT_STATUS_NOT_PERMITTED, 4},
		{pluginv1.CheckAccountStatus_CHECK_ACCOUNT_STATUS_UNSUPPORTED, 5},
		{pluginv1.CheckAccountStatus_CHECK_ACCOUNT_STATUS_UNAVAILABLE, 6},
	}
	for _, tc := range cases {
		if got := tc.value.Number(); got != tc.want {
			t.Errorf("%v = %d, want %d", tc.value, got, tc.want)
		}
	}
}
