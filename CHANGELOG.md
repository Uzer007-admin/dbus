# Local fork 5.3.0

Prepared from upstream `4ad75260bdbdd09d51af62b316a17dd60c6aa0e3` (5.2.3).
This is a local release candidate; no upstream or fork release tag was published.

- Require Go 1.27.1 and update `golang.org/x/sys` to 0.49.0.
- Restore Windows compilation and test system bus address overrides.
- Preserve panics from caller-provided readers and writers (#440).
- Reject incomplete writes, malformed string/signature terminators, invalid
  UTF-8/object paths, array boundaries, message length overflow and body mismatch.
- Write numeric fields without `encoding/binary` reflection allocations; encode
  and decode byte arrays in bulk; allocate string conversion storage lazily.
- Add cancellable property helpers without changing the `BusObject` interface
  (#195). Concrete `Object` also implements the new methods.
- Discover session bus sockets under `XDG_RUNTIME_DIR` (#372).
- Close shared connections recreated by tests (#383); report unavailable Linux
  user/PID namespaces as an explicit test skip.
- Preserve `FlagAllowInteractiveAuthorization` in object method calls.
- Correct no-reply send errors and closed completion channels; ignore caller
  channels as documented, honor pre-cancelled contexts and retire unused serials.
  Keep serial zero reserved through wraparound.
- Batch fixed-width numeric arrays with both byte orders and owned decoder data;
  preserve UnixFD encoding and NaN/signed-zero bit patterns.
- Decode UnixFD arrays, nested arrays and dictionaries into their actual descriptor
  types; reject invalid indices instead of panicking in reflection operations.
- Preserve original Variant signatures when storing map and slice containers;
  decoded struct variants no longer become arrays of variants during Store.
- Add the `portal` Settings client and typed standardized appearance preferences.
  Preserve missing settings and cancellation, normalize unknown preference enums,
  validate accent values and parse `SettingChanged` without owning connections or
  spawning UI subscriptions. ReadAll and change signals are tested through a
  real isolated Linux D-Bus daemon with an exported test service.
- Add opt-in bounded FIFO subscriptions with overflow notification and counters,
  independent of custom SignalHandlers; legacy Signal behavior is unchanged.
- Add a coalescing portal appearance watcher with unique-owner validation,
  restart/overflow re-read, cancellable reads, one pending update and explicit
  cleanup of borrowed connection subscriptions. Suppress unchanged snapshots.
- Add bounded exponential retries after temporary portal read failures, without
  polling an absent service. `WaitReady(ctx)` broadcasts readiness to multiple
  callers without consuming UI updates. Shutdown publishes unavailable state;
  a closed connection no longer advertises an available settings snapshot.

The normal and `godbus_no_tcp` suites passed with a real Linux session daemon,
including race checks. Native Windows daemon interoperability, BSD/macOS runtime
and the user-namespace authentication case remain separate validation targets.
