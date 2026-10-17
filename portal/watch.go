package portal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Uzer007-admin/dbus/v5"
)

var ErrSettingsUnavailable = errors.New("portal: settings service unavailable")
var ErrWatcherClosed = errors.New("portal: appearance watcher closed")

// WatchOptions configures a watcher without taking ownership of its connection.
// Destination defaults to the desktop portal; it must be a well-known bus name.
// A missing service is observed until its owner appears, without auto-activation.
type WatchOptions struct {
	Destination    string
	ReadTimeout    time.Duration
	SignalCapacity int
	// RetryInterval defaults to 250 ms. Failed reads retry with backoff up to
	// 5 seconds. An absent service waits for NameOwnerChanged, without polling.
	RetryInterval time.Duration
	// IncludeDesktopSettings запрашивает расширение палитры KDE.
	IncludeDesktopSettings bool
}

// AppearanceSnapshot contains an initial authoritative read plus ordered changes
// from its verified unique owner, or an unavailable service/error.
// Preference.Available still distinguishes individual missing
// keys. Generation is local to the watcher, not a server revision number.
type AppearanceSnapshot struct {
	Generation uint64
	Owner      string
	Available  bool
	Appearance Appearance
	Err        error
}

// AppearanceWatcher coalesces refresh requests and keeps one pending update.
// It uses one control goroutine and at most one read worker; slow consumers do
// not accumulate updates or signal-delivery goroutines. Owner changes and raw
// queue overflow trigger authoritative re-reads. It applies no Fyne UI policy.
type AppearanceWatcher struct {
	conn         *dbus.Conn
	options      WatchOptions
	subscription *dbus.SignalSubscription
	cancel       context.CancelFunc
	updates      chan AppearanceSnapshot
	done         chan struct{}
	mu           sync.RWMutex
	latest       AppearanceSnapshot
	changed      chan struct{}
}

func (w *AppearanceWatcher) Updates() <-chan AppearanceSnapshot { return w.updates }
func (w *AppearanceWatcher) Done() <-chan struct{}              { return w.done }
func (w *AppearanceWatcher) DroppedSignals() uint64             { return w.subscription.Dropped() }
func (w *AppearanceWatcher) Snapshot() AppearanceSnapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.latest
}

// WaitReady waits for a complete available snapshot without consuming Updates.
// Any number of callers can await startup/recovery independently of a UI
// observer. Missing keys remain missing. Context timeout bounds caller latency;
// transient read failures remain visible in Snapshot/Updates while retries run.
func (w *AppearanceWatcher) WaitReady(ctx context.Context) (AppearanceSnapshot, error) {
	for {
		if err := ctx.Err(); err != nil {
			return AppearanceSnapshot{}, err
		}
		w.mu.RLock()
		snapshot, changed := w.latest, w.changed
		w.mu.RUnlock()
		if errors.Is(snapshot.Err, ErrWatcherClosed) {
			return snapshot, snapshot.Err
		}
		select {
		case <-w.done:
			return snapshot, errors.Join(ErrWatcherClosed, snapshot.Err)
		default:
		}
		if snapshot.Available {
			return snapshot, nil
		}
		select {
		case <-ctx.Done():
			return AppearanceSnapshot{}, ctx.Err()
		case <-w.done:
			return w.Snapshot(), ErrWatcherClosed
		case <-changed:
		}
	}
}

// Close cancels reads, unregisters this watcher and waits for its workers. It
// leaves the supplied connection and other subscribers/match rules intact.
func (w *AppearanceWatcher) Close() { w.cancel(); <-w.done }

func WatchAppearance(ctx context.Context, conn *dbus.Conn, options WatchOptions) (*AppearanceWatcher, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if options.Destination == "" {
		options.Destination = SettingsDestination
	}
	if !validDestination(options.Destination) {
		return nil, fmt.Errorf("portal: invalid well-known destination %q", options.Destination)
	}
	if options.ReadTimeout == 0 {
		options.ReadTimeout = 5 * time.Second
	}
	if options.ReadTimeout < 0 {
		return nil, errors.New("portal: negative read timeout")
	}
	if options.SignalCapacity == 0 {
		options.SignalCapacity = 32
	}
	if options.RetryInterval == 0 {
		options.RetryInterval = 250 * time.Millisecond
	}
	if options.RetryInterval < 0 || options.RetryInterval > 5*time.Second {
		return nil, errors.New("portal: retry interval must be between 0 and 5 seconds")
	}
	subscription, err := conn.SubscribeSignals(options.SignalCapacity)
	if err != nil {
		return nil, err
	}
	rules := watchRules(options.Destination, options.IncludeDesktopSettings)
	installed := 0
	for _, rule := range rules {
		matchCtx, cancel := context.WithTimeout(ctx, options.ReadTimeout)
		err := conn.AddMatchSignalContext(matchCtx, rule...)
		cancel()
		if err != nil {
			removeRules(conn, rules[:installed])
			subscription.Close()
			return nil, err
		}
		installed++
	}
	watchCtx, cancel := context.WithCancel(ctx)
	watcher := &AppearanceWatcher{conn: conn, options: options, subscription: subscription, cancel: cancel, updates: make(chan AppearanceSnapshot, 1), done: make(chan struct{}), changed: make(chan struct{})}
	go watcher.run(watchCtx, rules)
	return watcher, nil
}

func validDestination(name string) bool {
	if len(name) > 255 {
		return false
	}
	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if len(part) == 0 || part[0] >= '0' && part[0] <= '9' {
			return false
		}
		for _, c := range []byte(part) {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
				return false
			}
		}
	}
	return true
}

func watchRules(destination string, desktop ...bool) [][]dbus.MatchOption {
	rules := [][]dbus.MatchOption{
		{dbus.WithMatchSender("org.freedesktop.DBus"), dbus.WithMatchObjectPath("/org/freedesktop/DBus"), dbus.WithMatchInterface("org.freedesktop.DBus"), dbus.WithMatchMember("NameOwnerChanged"), dbus.WithMatchArg(0, destination)},
		{dbus.WithMatchSender(destination), dbus.WithMatchObjectPath(SettingsPath), dbus.WithMatchInterface(SettingsInterface), dbus.WithMatchMember("SettingChanged"), dbus.WithMatchArg(0, AppearanceNamespace)},
	}
	if len(desktop) > 0 && desktop[0] {
		rules = append(rules, []dbus.MatchOption{dbus.WithMatchSender(destination), dbus.WithMatchObjectPath(SettingsPath), dbus.WithMatchInterface(SettingsInterface), dbus.WithMatchMember("SettingChanged"), dbus.WithMatchArg0Namespace(DesktopNamespace)})
	}
	return rules
}

func removeRules(conn *dbus.Conn, rules [][]dbus.MatchOption) {
	for _, rule := range rules {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = conn.RemoveMatchSignalContext(ctx, rule...)
		cancel()
	}
}

func (w *AppearanceWatcher) publish(snapshot AppearanceSnapshot) {
	w.mu.Lock()
	previous := w.latest
	sameError := previous.Err == nil && snapshot.Err == nil || previous.Err != nil && snapshot.Err != nil && previous.Err.Error() == snapshot.Err.Error()
	if previous.Generation != 0 && previous.Owner == snapshot.Owner && previous.Available == snapshot.Available && previous.Appearance == snapshot.Appearance && sameError {
		w.mu.Unlock()
		return
	}
	snapshot.Generation = w.latest.Generation + 1
	w.latest = snapshot
	if w.changed != nil {
		close(w.changed)
	}
	w.changed = make(chan struct{})
	w.mu.Unlock()
	select {
	case w.updates <- snapshot:
		return
	default:
	}
	select {
	case <-w.updates:
	default:
	}
	select {
	case w.updates <- snapshot:
	default:
	}
}

type appearanceRead struct {
	epoch                          uint64
	snapshot                       AppearanceSnapshot
	settingSequence, ownerSequence dbus.Sequence
}

func (w *AppearanceWatcher) run(ctx context.Context, rules [][]dbus.MatchOption) {
	results := make(chan appearanceRead, 1)
	var workers sync.WaitGroup
	var cancelRead context.CancelFunc
	var retry *time.Timer
	var retryC <-chan time.Time
	retryDelay := w.options.RetryInterval
	stopRetry := func() {
		if retry != nil {
			retry.Stop()
		}
		retryC = nil
	}
	armRetry := func() {
		if retry == nil {
			retry = time.NewTimer(retryDelay)
		} else {
			retry.Reset(retryDelay)
		}
		retryC = retry.C
		if retryDelay < 5*time.Second {
			retryDelay *= 2
			if retryDelay > 5*time.Second {
				retryDelay = 5 * time.Second
			}
		}
	}
	defer func() {
		w.cancel()
		stopRetry()
		last := w.Snapshot()
		w.publish(AppearanceSnapshot{Owner: last.Owner, Err: errors.Join(ErrWatcherClosed, ctx.Err(), w.conn.Context().Err())})
		if cancelRead != nil {
			cancelRead()
		}
		workers.Wait()
		removeRules(w.conn, rules)
		w.subscription.Close()
		close(w.updates)
		close(w.done)
	}()
	var epoch uint64
	var settingSequence, ownerSequence dbus.Sequence
	var dropped uint64
	owner := ""
	dirty, busy := true, false
	checkOverflow := func() {
		if count := w.subscription.Dropped(); count != dropped {
			dropped = count
			epoch++
			owner, dirty = "", true
			settingSequence, ownerSequence = dbus.NoSequence, dbus.NoSequence
			stopRetry()
			if cancelRead != nil {
				cancelRead()
			}
		}
	}
	for {
		checkOverflow()
		if dirty && !busy {
			stopRetry()
			readCtx, cancel := context.WithTimeout(ctx, w.options.ReadTimeout)
			cancelRead, dirty, busy = cancel, false, true
			readEpoch := epoch
			workers.Go(func() {
				defer cancel()
				snapshot, settingsSeq, ownerSeq := w.read(readCtx)
				result := appearanceRead{readEpoch, snapshot, settingsSeq, ownerSeq}
				select {
				case results <- result:
				case <-ctx.Done():
				}
			})
		}
		select {
		case <-ctx.Done():
			return
		case <-w.conn.Context().Done():
			return
		case <-w.subscription.Done():
			return
		case <-retryC:
			retryC = nil
			dirty = true
		case <-w.subscription.Overflow():
			checkOverflow()
		case result := <-results:
			busy = false
			checkOverflow()
			if result.epoch != epoch {
				continue
			}
			owner = result.snapshot.Owner
			settingSequence, ownerSequence = result.settingSequence, result.ownerSequence
			w.publish(result.snapshot)
			if retryableAppearance(result.snapshot) {
				armRetry()
			} else {
				stopRetry()
				retryDelay = w.options.RetryInterval
			}
		case signal, open := <-w.subscription.Signals():
			if !open {
				return
			}
			checkOverflow()
			if next, matched := changedOwner(signal, w.options.Destination); matched {
				// Queued acquisition/loss notifications already covered by the
				// final GetNameOwner reply must not roll back a newer snapshot.
				if signal.Sequence != dbus.NoSequence && signal.Sequence <= ownerSequence {
					continue
				}
				stopRetry()
				retryDelay = w.options.RetryInterval
				epoch++
				// Even loss requires a read: a later acquisition may have been
				// dropped after this queued event and its overflow token consumed.
				owner, dirty = next, true
				settingSequence, ownerSequence = dbus.NoSequence, dbus.NoSequence
				if cancelRead != nil {
					cancelRead()
				}
				w.publish(AppearanceSnapshot{Owner: next, Err: ErrSettingsUnavailable})
				continue
			}
			change, matched, err := ParseSettingChanged(signal)
			desktopChange := w.options.IncludeDesktopSettings && strings.HasPrefix(change.Namespace, DesktopNamespace+".")
			if !matched || err == nil && !desktopChange && (change.Namespace != AppearanceNamespace || !appearanceKey(change.Key)) {
				continue
			}
			if owner != "" && signal.Sender != owner {
				continue
			}
			if signal.Sequence != dbus.NoSequence && signal.Sequence <= settingSequence {
				continue
			}
			current := w.Snapshot()
			if err == nil && !desktopChange && !busy && !dirty && owner != "" && current.Available && current.Owner == owner && signal.Sequence != dbus.NoSequence {
				next, _, applyErr := ApplyAppearanceChange(current.Appearance, change)
				if applyErr == nil {
					current.Appearance = next
					settingSequence = signal.Sequence
					w.publish(current)
					continue
				}
			}
			// During reads, or after malformed/lost/unordered messages, retain
			// the authoritative recovery path. Cancel the old epoch so its
			// response cannot overwrite a change observed while it was pending.
			epoch++
			dirty = true
			if cancelRead != nil {
				cancelRead()
			}
		}
	}
}

func retryableAppearance(snapshot AppearanceSnapshot) bool {
	if snapshot.Available || snapshot.Err == nil {
		return false
	}
	var pointer *dbus.Error
	var value dbus.Error
	name := ""
	if errors.As(snapshot.Err, &pointer) && pointer != nil {
		name = pointer.Name
	} else if errors.As(snapshot.Err, &value) {
		name = value.Name
	}
	return name != "org.freedesktop.DBus.Error.NameHasNoOwner" && name != "org.freedesktop.DBus.Error.ServiceUnknown"
}

func changedOwner(signal *dbus.Signal, destination string) (string, bool) {
	if signal == nil || signal.Sender != "org.freedesktop.DBus" || signal.Path != "/org/freedesktop/DBus" || signal.Name != "org.freedesktop.DBus.NameOwnerChanged" || len(signal.Body) != 3 {
		return "", false
	}
	name, a := signal.Body[0].(string)
	old, b := signal.Body[1].(string)
	next, c := signal.Body[2].(string)
	if !a || !b || !c || name != destination || old != "" && !strings.HasPrefix(old, ":") || next != "" && !strings.HasPrefix(next, ":") {
		return "", false
	}
	return next, true
}

func (w *AppearanceWatcher) read(ctx context.Context) (AppearanceSnapshot, dbus.Sequence, dbus.Sequence) {
	var owner string
	begin := w.conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetNameOwner", 0, w.options.Destination)
	if err := begin.Store(&owner); err != nil {
		return AppearanceSnapshot{Err: errors.Join(ErrSettingsUnavailable, err)}, dbus.NoSequence, dbus.NoSequence
	}
	namespaces := []string{AppearanceNamespace}
	if w.options.IncludeDesktopSettings {
		namespaces = append(namespaces, DesktopNamespace+".*")
	}
	call := w.conn.Object(owner, SettingsPath).CallWithContext(ctx, SettingsInterface+".ReadAll", 0, namespaces)
	var values map[string]map[string]dbus.Variant
	if err := call.Store(&values); err != nil {
		return AppearanceSnapshot{Owner: owner, Err: err}, dbus.NoSequence, dbus.NoSequence
	}
	appearance, err := ParseAppearance(values[AppearanceNamespace])
	if err != nil {
		return AppearanceSnapshot{Owner: owner, Err: err}, dbus.NoSequence, dbus.NoSequence
	}
	if w.options.IncludeDesktopSettings {
		appearance.Desktop = ParseDesktopAppearance(values)
	}
	var current string
	verified := w.conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetNameOwner", 0, w.options.Destination)
	if err := verified.Store(&current); err != nil {
		return AppearanceSnapshot{Err: errors.Join(ErrSettingsUnavailable, err)}, dbus.NoSequence, dbus.NoSequence
	}
	if current != owner {
		return AppearanceSnapshot{Err: ErrSettingsUnavailable}, dbus.NoSequence, dbus.NoSequence
	}
	// A service may emit a newer change while ReadAll's older snapshot is still
	// being serialized. Ignore only signals preceding the read's start barrier,
	// not those preceding its reply; the latter must update the snapshot.
	return AppearanceSnapshot{Owner: owner, Available: true, Appearance: appearance}, begin.ResponseSequence, verified.ResponseSequence
}
