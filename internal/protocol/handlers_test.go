package protocol

import (
	"errors"
	"testing"

	"github.com/steveyegge/gastown/internal/mail"
)

func TestHandlerRegistry_RegisterAndHandle(t *testing.T) {
	reg := NewHandlerRegistry()
	called := false

	reg.Register(TypeMergeReady, func(msg *mail.Message) error {
		called = true
		return nil
	})

	msg := &mail.Message{Subject: "MERGE_READY nux"}
	if err := reg.Handle(msg); err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
}

func TestHandlerRegistry_Handle_UnknownSubject(t *testing.T) {
	reg := NewHandlerRegistry()
	msg := &mail.Message{Subject: "not a protocol message"}
	err := reg.Handle(msg)
	if err == nil {
		t.Fatal("Handle() expected error for unknown subject")
	}
}

func TestHandlerRegistry_Handle_NoHandler(t *testing.T) {
	reg := NewHandlerRegistry()
	// TypeMergeReady is valid but no handler registered
	msg := &mail.Message{Subject: "MERGE_READY nux"}
	err := reg.Handle(msg)
	if err == nil {
		t.Fatal("Handle() expected error when no handler registered")
	}
}

func TestHandlerRegistry_Handle_HandlerError(t *testing.T) {
	reg := NewHandlerRegistry()
	expectedErr := errors.New("handler failed")

	reg.Register(TypeMerged, func(msg *mail.Message) error {
		return expectedErr
	})

	msg := &mail.Message{Subject: "MERGED nux"}
	err := reg.Handle(msg)
	if err != expectedErr {
		t.Errorf("Handle() error = %v, want %v", err, expectedErr)
	}
}

func TestHandlerRegistry_CanHandle(t *testing.T) {
	reg := NewHandlerRegistry()
	reg.Register(TypeMergeReady, func(msg *mail.Message) error { return nil })

	tests := []struct {
		name    string
		subject string
		want    bool
	}{
		{"registered type", "MERGE_READY nux", true},
		{"unregistered type", "MERGED nux", false},
		{"not protocol", "hello world", false},
		{"empty subject", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &mail.Message{Subject: tt.subject}
			if got := reg.CanHandle(msg); got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.subject, got, tt.want)
			}
		})
	}
}

func TestHandlerRegistry_ProcessProtocolMessage_Handled(t *testing.T) {
	reg := NewHandlerRegistry()
	handlerCalled := false
	reg.Register(TypeMergeReady, func(msg *mail.Message) error {
		handlerCalled = true
		return nil
	})

	msg := &mail.Message{Subject: "MERGE_READY nux"}
	isProto, err := reg.ProcessProtocolMessage(msg)
	if !isProto {
		t.Error("expected isProtocol=true")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !handlerCalled {
		t.Error("handler was not called")
	}
}

func TestHandlerRegistry_ProcessProtocolMessage_NoHandler(t *testing.T) {
	reg := NewHandlerRegistry()
	msg := &mail.Message{Subject: "MERGED nux"}
	isProto, err := reg.ProcessProtocolMessage(msg)
	if !isProto {
		t.Error("expected isProtocol=true")
	}
	if !errors.Is(err, ErrNoHandler) {
		t.Errorf("expected ErrNoHandler, got: %v", err)
	}
}

func TestHandlerRegistry_ProcessProtocolMessage_NotProtocol(t *testing.T) {
	reg := NewHandlerRegistry()
	msg := &mail.Message{Subject: "regular mail"}
	isProto, err := reg.ProcessProtocolMessage(msg)
	if isProto {
		t.Error("expected isProtocol=false")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHandlerRegistry_ProcessProtocolMessage_HandlerError(t *testing.T) {
	reg := NewHandlerRegistry()
	expectedErr := errors.New("boom")
	reg.Register(TypeMergeFailed, func(msg *mail.Message) error {
		return expectedErr
	})
	msg := &mail.Message{Subject: "MERGE_FAILED nux"}
	isProto, err := reg.ProcessProtocolMessage(msg)
	if !isProto {
		t.Error("expected isProtocol=true")
	}
	if err != expectedErr {
		t.Errorf("error = %v, want %v", err, expectedErr)
	}
}

func TestHandlerRegistry_OverwriteHandler(t *testing.T) {
	reg := NewHandlerRegistry()
	callCount := 0

	reg.Register(TypeMerged, func(msg *mail.Message) error {
		callCount = 1
		return nil
	})
	reg.Register(TypeMerged, func(msg *mail.Message) error {
		callCount = 2
		return nil
	})

	msg := &mail.Message{Subject: "MERGED nux"}
	if err := reg.Handle(msg); err != nil {
		t.Fatal(err)
	}
	if callCount != 2 {
		t.Errorf("expected second handler to be called, got callCount=%d", callCount)
	}
}

func TestNewHandlerRegistry_Empty(t *testing.T) {
	reg := NewHandlerRegistry()
	if reg == nil {
		t.Fatal("NewHandlerRegistry() returned nil")
	}
	msg := &mail.Message{Subject: "MERGED nux"}
	if reg.CanHandle(msg) {
		t.Error("empty registry should not handle anything")
	}
}

func TestHandlerRegistry_MultipleTypes(t *testing.T) {
	reg := NewHandlerRegistry()
	var calledTypes []MessageType

	for _, mt := range []MessageType{TypeMergeReady, TypeMerged, TypeMergeFailed, TypeReworkRequest} {
		mt := mt // capture
		reg.Register(mt, func(msg *mail.Message) error {
			calledTypes = append(calledTypes, mt)
			return nil
		})
	}

	subjects := []string{
		"MERGE_READY alpha",
		"MERGED beta",
		"MERGE_FAILED gamma",
		"REWORK_REQUEST delta",
	}

	for _, subj := range subjects {
		msg := &mail.Message{Subject: subj}
		if err := reg.Handle(msg); err != nil {
			t.Errorf("Handle(%q) error: %v", subj, err)
		}
	}

	if len(calledTypes) != 4 {
		t.Errorf("expected 4 handler calls, got %d", len(calledTypes))
	}
}
