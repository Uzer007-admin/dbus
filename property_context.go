package dbus

import (
	"context"
	"errors"
	"strings"
)

// GetPropertyWithContext retrieves a property with cancellation or a deadline.
// The helper accepts BusObject implementations without extending that interface.
func GetPropertyWithContext(ctx context.Context, object BusObject, property string) (Variant, error) {
	var value Variant
	err := StorePropertyWithContext(ctx, object, property, &value)
	return value, err
}

// StorePropertyWithContext stores a property with cancellation or a deadline.
func StorePropertyWithContext(ctx context.Context, object BusObject, property string, value any) error {
	iface, member, err := propertyParts(property)
	if err != nil {
		return err
	}
	return object.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, iface, member).Store(value)
}

// SetPropertyWithContext sets a property with cancellation or a deadline.
func SetPropertyWithContext(ctx context.Context, object BusObject, property string, value any) error {
	iface, member, err := propertyParts(property)
	if err != nil {
		return err
	}
	variant, ok := value.(Variant)
	if !ok {
		variant = MakeVariant(value)
	}
	return object.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Set", 0, iface, member, variant).Err
}

func propertyParts(property string) (string, string, error) {
	index := strings.LastIndexByte(property, '.')
	if index <= 0 || index+1 == len(property) {
		return "", "", errors.New("dbus: invalid property " + property)
	}
	return property[:index], property[index+1:], nil
}

func (o *Object) GetPropertyWithContext(ctx context.Context, property string) (Variant, error) {
	return GetPropertyWithContext(ctx, o, property)
}
func (o *Object) StorePropertyWithContext(ctx context.Context, property string, value any) error {
	return StorePropertyWithContext(ctx, o, property, value)
}
func (o *Object) SetPropertyWithContext(ctx context.Context, property string, value any) error {
	return SetPropertyWithContext(ctx, o, property, value)
}
