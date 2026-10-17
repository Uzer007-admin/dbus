package dbus

// codecError identifies errors raised by our parser/encoder. Arbitrary panics
// from caller-provided readers and writers retain their normal Go semantics.
type codecError struct{ err error }

func codecPanic(err error) { panic(codecError{err}) }
