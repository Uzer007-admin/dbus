package dbus

import "reflect"

// Containers must use the type that their elements actually decode to. With
// ancillary descriptors present, 'h' decodes to UnixFD rather than UnixFDIndex.
// Do not change the shared signature/type table, which is used concurrently.
func (dec *decoder) valueType(sig string) reflect.Type {
	typeForSignature := typeFor(sig)
	if dec.fds == nil {
		return typeForSignature
	}
	return resolvedFDType(typeForSignature)
}

func resolvedFDType(t reflect.Type) reflect.Type {
	if t == unixFDIndexType {
		return unixFDType
	}
	switch t.Kind() {
	case reflect.Slice:
		elem := resolvedFDType(t.Elem())
		if elem != t.Elem() {
			return reflect.SliceOf(elem)
		}
	case reflect.Map:
		key, elem := resolvedFDType(t.Key()), resolvedFDType(t.Elem())
		if key != t.Key() || elem != t.Elem() {
			return reflect.MapOf(key, elem)
		}
	}
	return t
}
