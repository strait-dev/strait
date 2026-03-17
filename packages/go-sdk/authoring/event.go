package authoring

// EventDefinition represents a named event with optional validation.
type EventDefinition struct {
	Key      string
	Validate func(input any) (any, error)
}

// Parse validates and returns the input using the validator, or returns as-is.
func (e *EventDefinition) Parse(input any) (any, error) {
	if e.Validate != nil {
		return e.Validate(input)
	}
	return input, nil
}

// DefineEvent creates a new event definition.
func DefineEvent(key string, validate func(any) (any, error)) *EventDefinition {
	return &EventDefinition{Key: key, Validate: validate}
}
