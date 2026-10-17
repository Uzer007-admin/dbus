package dbus

import (
	"context"
	"errors"
	"testing"
	"time"
)

type recordingTransport struct {
	transport
	message *Message
	err     error
}

func (t *recordingTransport) SendMessage(msg *Message) error {
	t.message = msg
	return t.err
}

func sendTestConnection() (*Conn, *recordingTransport) {
	transport := new(recordingTransport)
	conn := &Conn{transport: transport, calls: newCallTracker(), serialGen: newSerialGenerator()}
	conn.outHandler = &outputHandler{conn: conn}
	return conn, transport
}

func testMethodCall(flags Flags) *Message {
	return &Message{Type: TypeMethodCall, Flags: flags, Headers: map[HeaderField]Variant{
		FieldDestination: MakeVariant("org.example.Service"),
		FieldPath:        MakeVariant(ObjectPath("/")),
		FieldMember:      MakeVariant("Ping"),
	}}
}

func assertNoPendingSerials(t *testing.T, conn *Conn) {
	t.Helper()
	gen := conn.serialGen.(*serialGenerator)
	gen.lck.Lock()
	defer gen.lck.Unlock()
	if len(gen.serialUsed) != 1 || !gen.serialUsed[0] {
		t.Errorf("serials retained: %v", gen.serialUsed)
	}
}

func awaitSendCompletion(t *testing.T, call *Call) *Call {
	t.Helper()
	select {
	case completed := <-call.Done:
		return completed
	case <-time.After(time.Second):
		t.Fatal("call completion timed out")
		return nil
	}
}

func TestSendPreCancelledSerial(t *testing.T) {
	conn, transport := sendTestConnection()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, flags := range []Flags{0, FlagNoReplyExpected} {
		call := conn.SendWithContext(ctx, testMethodCall(flags), nil)
		if call = awaitSendCompletion(t, call); call.Err != context.Canceled {
			t.Fatalf("flags %v: %v", flags, call.Err)
		}
	}
	if transport.message != nil {
		t.Error("cancelled call was sent")
	}
	assertNoPendingSerials(t, conn)
}

func TestSendNoReplyResult(t *testing.T) {
	for _, sendErr := range []error{nil, errors.New("transport failure")} {
		conn, transport := sendTestConnection()
		transport.err = sendErr
		// The documented no-reply API ignores even an unbuffered caller channel.
		call := conn.Send(testMethodCall(FlagNoReplyExpected), make(chan *Call))
		if !errors.Is(call.Err, sendErr) {
			t.Fatalf("got %v, want %v", call.Err, sendErr)
		}
		if completed := awaitSendCompletion(t, call); completed != call {
			t.Fatal("completion differs from returned call")
		}
		select {
		case _, open := <-call.Done:
			if open {
				t.Fatal("no-reply completion channel is open")
			}
		case <-time.After(time.Second):
			t.Fatal("no-reply completion channel was not closed")
		}
		assertNoPendingSerials(t, conn)
	}
}

func TestSendClosedConnectionRetiresSerial(t *testing.T) {
	for _, flags := range []Flags{0, FlagNoReplyExpected} {
		conn, transport := sendTestConnection()
		conn.outHandler.close()
		call := conn.Send(testMethodCall(flags), nil)
		if call = awaitSendCompletion(t, call); call.Err != ErrClosed {
			t.Fatalf("flags %v: %v", flags, call.Err)
		}
		if transport.message != nil {
			t.Fatal("closed connection sent a message")
		}
		assertNoPendingSerials(t, conn)
	}
}

func TestObjectInteractiveAuthorizationFlag(t *testing.T) {
	conn, transport := sendTestConnection()
	flags := FlagNoReplyExpected | FlagNoAutoStart | FlagAllowInteractiveAuthorization
	call := conn.Object("org.example.Service", "/").Call("org.example.Service.Ping", flags)
	if call.Err != nil || transport.message.Flags != flags {
		t.Fatalf("flags %v, want %v; %v", transport.message.Flags, flags, call.Err)
	}
}

func TestSerialZeroRemainsReserved(t *testing.T) {
	gen := newSerialGenerator()
	gen.RetireSerial(0)
	gen.nextSerial = ^uint32(0)
	if got := gen.GetSerial(); got != ^uint32(0) {
		t.Fatal(got)
	}
	if got := gen.GetSerial(); got != 1 {
		t.Fatalf("serial after wrap: %d", got)
	}
}
