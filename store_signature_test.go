package dbus

import (
	"reflect"
	"testing"
)

func TestStorePreservesContainerVariantSignatures(t *testing.T) {
	values := []Variant{
		MakeVariantWithSignature([]any{0.2, 0.4, 0.6}, Signature{"(ddd)"}),
		MakeVariantWithSignature([]any{MakeVariant("nested")}, Signature{"(v)"}),
		MakeVariantWithSignature([][]any{{0.2, 0.4, 0.6}}, Signature{"a(ddd)"}),
	}
	var slice []Variant
	if err := Store([]any{values}, &slice); err != nil {
		t.Fatal(err)
	}
	for i, value := range values {
		if slice[i].Signature() != value.Signature() || !reflect.DeepEqual(slice[i].Value(), value.Value()) {
			t.Fatalf("slice element %d lost wire type: %s -> %s", i, value.Signature(), slice[i].Signature())
		}
	}
	nested := map[string]map[string]Variant{"appearance": {"accent": values[0], "nested": values[1], "array": values[2]}}
	var dictionary map[string]map[string]Variant
	if err := Store([]any{nested}, &dictionary); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dictionary, nested) {
		t.Fatal("nested map variants lost wire signatures or values")
	}
	// Existing conversion into concrete structs must still unwrap the variant.
	type rgb struct{ Red, Green, Blue float64 }
	var colors map[string]rgb
	if err := Store([]any{map[string]Variant{"accent": values[0]}}, &colors); err != nil || colors["accent"] != (rgb{0.2, 0.4, 0.6}) {
		t.Fatalf("map struct conversion changed: %+v; %v", colors, err)
	}
	var colorSlice []rgb
	if err := Store([]any{values[:1]}, &colorSlice); err != nil || len(colorSlice) != 1 || colorSlice[0] != (rgb{0.2, 0.4, 0.6}) {
		t.Fatalf("slice struct conversion changed: %+v; %v", colorSlice, err)
	}
}
