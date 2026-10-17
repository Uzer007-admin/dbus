package dbus

import (
	"context"
	"reflect"
	"testing"
)

type contextPropertyObject struct {
	BusObject
	ctx    context.Context
	method string
	args   []any
}

func (object *contextPropertyObject) CallWithContext(ctx context.Context, method string, flags Flags, args ...any) *Call {
	object.ctx = ctx
	object.method = method
	object.args = args
	return &Call{Err: ctx.Err(), Body: []any{MakeVariant(uint32(42))}}
}

func TestPropertyContext(t *testing.T) {
	object := new(contextPropertyObject)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	value, err := GetPropertyWithContext(ctx, object, "org.example.Value")
	if err != nil || value.Value() != uint32(42) || object.ctx != ctx || !reflect.DeepEqual(object.args, []any{"org.example", "Value"}) {
		t.Fatalf("%v %v %+v", value, err, object)
	}
	if err := SetPropertyWithContext(ctx, object, "org.example.Value", uint32(7)); err != nil || object.method != "org.freedesktop.DBus.Properties.Set" || object.args[2].(Variant).Value() != uint32(7) {
		t.Fatalf("set: %v %+v", err, object)
	}
	cancel()
	if _, err := GetPropertyWithContext(ctx, object, "org.example.Value"); err != context.Canceled {
		t.Fatalf("cancel: %v", err)
	}
	for _, p := range []string{"", ".Value", "Value", "org.example."} {
		if _, err := GetPropertyWithContext(ctx, object, p); err == nil {
			t.Errorf("accepted %q", p)
		}
	}
}
