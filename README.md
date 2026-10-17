# dbus: библиотека проекта Granio

**Исходный репозиторий:** [godbus/dbus](https://github.com/godbus/dbus).
**Цель форка:** Развитие D-Bus 5.3.0: исправления обмена, наблюдение за порталом системной темы, палитра и метаданные KDE.

Репозиторий развивается самостоятельно. Исходная история, лицензия и авторство
сохранены. Минимальная версия Go — **1.27.1**. Собственный адрес модуля:
`github.com/Uzer007-admin/dbus/v5`
.

Платформенные ограничения и результаты проверок:
[развитие графического стека](https://github.com/Uzer007-admin/granio/blob/dev/granio/docs/NATIVE_STACK_IMPLEMENTATION.md).
Длительные проверки производительности в этот этап не входят.

---

## Документация исходного проекта и прежних этапов

Адреса установки в сохранённых примерах ниже могут относиться к исходнику.

dbus
----


dbus is a simple library that implements native Go client bindings for the
D-Bus message bus system.

This local fork prepares version **5.3.0**, requires **Go 1.27.1**, and keeps the
`github.com/godbus/dbus/v5` import path. See [CHANGELOG.md](CHANGELOG.md) for fixes,
added context helpers and validation limits. To consume the unpublished fork:

```sh
go mod edit -require=github.com/godbus/dbus/v5@v5.3.0
go mod edit -replace=github.com/godbus/dbus/v5=./deps/dbus
```

### Features

* Complete native implementation of the D-Bus message protocol
* Go-like API (channels for signals / asynchronous method calls, Goroutine-safe connections)
* Subpackages that help with the introspection / property interfaces
* The `portal` subpackage reads standardized XDG appearance settings using
  cancellable `ReadAll`, preserving missing preferences. It covers color scheme,
  contrast, reduced motion and accent color, and validates `SettingChanged`
  payloads. It owns neither the supplied bus connection nor UI subscriptions.
  Shared-connection consumers must validate a signal's sender against the
  portal's unique owner before applying changes.

`Conn.SubscribeSignals(capacity)` adds an owned bounded FIFO alongside any
existing SignalHandler. Full queues drop new signals, increment `Dropped` and
coalesce `Overflow` notifications without delivery goroutines; consumers must
re-read authoritative state after overflow. Match rules remain explicit, shared
connection signals must be validated, and delivered Signal objects are shared.
The original `Signal` API keeps its existing delivery behavior.

`portal.WatchAppearance` uses this stream for a cancellable settings watcher.
It performs owner-validated ReadAll, handles NameOwnerChanged/restart and queue
loss, cancels old reads, and publishes at most one pending complete snapshot.
One read worker is active at a time. Close stops the watcher and removes its
own subscriptions/rules; it leaves the caller's connection open. An absent
service is observed without auto-activation. UI scheduling and user overrides
remain the consumer's responsibility.

Once the owner and initial snapshot are verified, standardized `SettingChanged`
values update the snapshot directly through `ApplyAppearanceChange`, without a
new ReadAll or owner round-trips for each change. Sender validation and incoming
message sequences prevent spoofing and queued old values from rolling back a
newer read. Changes emitted while ReadAll is pending are not discarded merely
because their message preceded its response. Malformed values, owner changes,
unknown ordering and raw queue loss keep the authoritative recovery path.
The watcher checks the monotonic dropped counter before accepting reads/deltas,
in addition to the coalesced overflow notification, and ignores unrelated keys.

### Usage

The complete package documentation and some simple examples are available at
[pkg.go.dev](https://pkg.go.dev/github.com/godbus/dbus/v5). Also, the
[\_examples](https://github.com/godbus/dbus/tree/master/_examples) directory
gives a short overview over the basic usage.

#### Projects using godbus
- [bluetooth](https://github.com/tinygo-org/bluetooth): cross-platform Bluetooth API for Go and TinyGo.
- [fyne](https://github.com/fyne-io/fyne): a cross platform GUI in Go inspired by Material Design.
- [fynedesk](https://github.com/fyne-io/fynedesk): a full desktop environment for Linux/Unix using Fyne.
- [go-systemd](https://github.com/coreos/go-systemd): Go bindings to systemd.
- [notify](https://github.com/esiqveland/notify) provides desktop notifications over dbus into a library.

Please note that the API is considered unstable for now and may change without
further notice.

### License

The library is available under the Simplified BSD License; see LICENSE for the full
text.

Nearly all of the credit for this library goes to https://github.com/guelfey.
