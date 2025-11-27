package main

import "testing"

func TestErrorTypes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "ErrEmptyTaskName",
			err:  ErrEmptyTaskName,
			want: "task name cannot be empty",
		},
		{
			name: "ErrTaskNameTooLong",
			err:  ErrTaskNameTooLong,
			want: "task name too long",
		},
		{
			name: "ErrInvalidTaskName",
			err:  ErrInvalidTaskName,
			want: "task name contains invalid characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.want {
				t.Errorf("%s.Error() = %q; want %q", tt.name, tt.err.Error(), tt.want)
			}
		})
	}
}

func TestErrorIsComparable(t *testing.T) {
	// Test that errors can be compared
	if ErrEmptyTaskName != ErrEmptyTaskName {
		t.Error("ErrEmptyTaskName != ErrEmptyTaskName; want equal")
	}
	
	if ErrEmptyTaskName == ErrTaskNameTooLong {
		t.Error("ErrEmptyTaskName == ErrTaskNameTooLong; want not equal")
	}
	
	if ErrTaskNameTooLong != ErrTaskNameTooLong {
		t.Error("ErrTaskNameTooLong != ErrTaskNameTooLong; want equal")
	}
	
	if ErrInvalidTaskName != ErrInvalidTaskName {
		t.Error("ErrInvalidTaskName != ErrInvalidTaskName; want equal")
	}
}

func TestErrorWrapping(t *testing.T) {
	// Test that errors can be wrapped with fmt.Errorf
	err := ErrEmptyTaskName
	wrapped := wrapError("test context", err)
	
	if wrapped == nil {
		t.Error("wrapError() = nil; want non-nil")
	}
	
	// Verify the wrapped error contains the original error message
	if !containsString(wrapped.Error(), err.Error()) {
		t.Errorf("wrapError() = %q; want error containing %q", wrapped.Error(), err.Error())
	}
}

// Helper function to wrap errors (simulating fmt.Errorf usage)
func wrapError(msg string, err error) error {
	return &wrappedError{msg: msg, err: err}
}

type wrappedError struct {
	msg string
	err error
}

func (e *wrappedError) Error() string {
	return e.msg + ": " + e.err.Error()
}

func (e *wrappedError) Unwrap() error {
	return e.err
}


