package authoring

import (
	"errors"
	"testing"
)

func TestDefineEvent_Key(t *testing.T) {
	event := DefineEvent("payment.completed", nil)
	if event.Key != "payment.completed" {
		t.Errorf("expected key 'payment.completed', got %q", event.Key)
	}
}

func TestDefineEvent_NilValidator(t *testing.T) {
	event := DefineEvent("test.event", nil)
	if event.Validate != nil {
		t.Error("expected nil validator")
	}
}

func TestDefineEvent_WithValidator(t *testing.T) {
	validator := func(input any) (any, error) {
		return input, nil
	}
	event := DefineEvent("validated.event", validator)
	if event.Validate == nil {
		t.Error("expected non-nil validator")
	}
}

func TestEventDefinition_Parse_WithoutValidator(t *testing.T) {
	event := DefineEvent("test.event", nil)
	input := map[string]any{"amount": 100}

	result, err := event.Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatal("expected map result")
	}
	if m["amount"] != 100 {
		t.Error("expected amount=100")
	}
}

func TestEventDefinition_Parse_WithValidator_Success(t *testing.T) {
	validator := func(input any) (any, error) {
		m, ok := input.(map[string]any)
		if !ok {
			return nil, errors.New("invalid input")
		}
		m["validated"] = true
		return m, nil
	}
	event := DefineEvent("validated.event", validator)

	result, err := event.Parse(map[string]any{"data": "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["validated"] != true {
		t.Error("expected validated=true")
	}
	if m["data"] != "test" {
		t.Error("expected data=test preserved")
	}
}

func TestEventDefinition_Parse_WithValidator_Error(t *testing.T) {
	validator := func(input any) (any, error) {
		return nil, errors.New("validation failed")
	}
	event := DefineEvent("failing.event", validator)

	_, err := event.Parse("invalid")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "validation failed" {
		t.Errorf("expected 'validation failed', got %q", err.Error())
	}
}

func TestEventDefinition_Parse_NilInput(t *testing.T) {
	event := DefineEvent("nil.event", nil)
	result, err := event.Parse(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for nil input")
	}
}

func TestEventDefinition_Parse_StringInput(t *testing.T) {
	event := DefineEvent("string.event", nil)
	result, err := event.Parse("hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello" {
		t.Errorf("expected 'hello', got %v", result)
	}
}

func TestEventDefinition_Parse_TransformValidator(t *testing.T) {
	validator := func(input any) (any, error) {
		s, ok := input.(string)
		if !ok {
			return nil, errors.New("expected string")
		}
		return map[string]any{"original": s, "length": len(s)}, nil
	}
	event := DefineEvent("transform.event", validator)

	result, err := event.Parse("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["original"] != "test" {
		t.Error("expected original=test")
	}
	if m["length"] != 4 {
		t.Error("expected length=4")
	}
}

func TestDefineEvent_MultipleEvents(t *testing.T) {
	e1 := DefineEvent("event.one", nil)
	e2 := DefineEvent("event.two", nil)

	if e1.Key == e2.Key {
		t.Error("expected different keys")
	}
	if e1.Key != "event.one" {
		t.Errorf("expected 'event.one', got %q", e1.Key)
	}
	if e2.Key != "event.two" {
		t.Errorf("expected 'event.two', got %q", e2.Key)
	}
}
