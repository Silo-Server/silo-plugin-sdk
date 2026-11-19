package pluginv1

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// assertOptionalInt32Field pins a field's wire metadata: an int32 field with
// explicit presence at a given number. The Go type and the field number alone
// allow swapping in sint32 or a plain int32, which keeps the same Go pointer and
// changes the contract the wire carries.
func assertOptionalInt32Field(t *testing.T, message protoreflect.ProtoMessage, field string, want protoreflect.FieldNumber) {
	t.Helper()
	got := message.ProtoReflect().Descriptor().Fields().ByName(protoreflect.Name(field))
	if got == nil {
		t.Fatalf("%T descriptor is missing %s", message, field)
	}
	if got.Number() != want {
		t.Fatalf("%T.%s field number = %d, want %d", message, field, got.Number(), want)
	}
	if got.Kind() != protoreflect.Int32Kind {
		t.Fatalf("%T.%s kind = %v, want int32", message, field, got.Kind())
	}
	if got.Cardinality() != protoreflect.Optional {
		t.Fatalf("%T.%s cardinality = %v, want explicit presence", message, field, got.Cardinality())
	}
}

func assertOptionalInt32Presence(t *testing.T, withZero proto.Message, empty proto.Message, get func(proto.Message) *int32) {
	t.Helper()

	data, err := proto.Marshal(withZero)
	if err != nil {
		t.Fatalf("marshal %T with explicit zero: %v", withZero, err)
	}
	decoded := withZero.ProtoReflect().New().Interface()
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal %T with explicit zero: %v", withZero, err)
	}
	value := get(decoded)
	if value == nil || *value != 0 {
		t.Fatalf("%T explicit zero = %v, want present value 0", decoded, value)
	}

	data, err = proto.Marshal(empty)
	if err != nil {
		t.Fatalf("marshal empty %T: %v", empty, err)
	}
	decoded = empty.ProtoReflect().New().Interface()
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal empty %T: %v", empty, err)
	}
	if value := get(decoded); value != nil {
		t.Fatalf("%T absent value = %v, want nil", decoded, value)
	}
}
