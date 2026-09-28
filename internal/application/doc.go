// Package application holds the use cases: the sequences of domain operations
// and port calls that make up a unit of work a user can ask for.
//
// It depends on internal/domain and on nothing else in this module. It owns
// the ordering and the error handling of a rotation; it owns none of the rules
// about what a rotation is, which belong to the domain, and none of the
// mechanics of talking to Docker, SQL or the filesystem, which belong to the
// adapters.
package application
