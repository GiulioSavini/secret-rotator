// Package domain holds the rotation model: what a secret is, what rotating one
// means, and what has to be undone when a rotation fails part-way.
//
// It is the innermost layer. It performs no I/O, knows nothing about YAML,
// Docker, SQL or the filesystem, and imports only the standard library. The
// capabilities it needs from the outside world are declared here as interfaces
// (see ports.go) and implemented by the adapters under internal/infrastructure,
// so every dependency points inwards. internal/domain/arch_test.go enforces
// that rule rather than leaving it to review.
package domain
