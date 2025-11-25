package main

import "errors"

var (
	ErrEmptyTaskName   = errors.New("task name cannot be empty")
	ErrTaskNameTooLong = errors.New("task name too long")
	ErrInvalidTaskName = errors.New("task name contains invalid characters")
)

