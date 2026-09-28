// Package credential holds the adapters that change a credential on a backing
// service: one per domain.Kind, plus the password generator and the registry
// that resolves a kind to its rotator.
//
// Each adapter implements domain.CredentialRotator and receives a fully
// resolved domain.Target, so none of them parses configuration, applies
// defaults or generates passwords -- those rules live in the domain.
package credential
