/*
Common Types
============

Shared types used across pattern implementations.
*/

package patterns

// Job represents a unit of work
type Job struct {
	ID   int
	Data interface{}
}

// Result represents the output of a job
type Result struct {
	JobID int
	Value interface{}
	Error error
}
