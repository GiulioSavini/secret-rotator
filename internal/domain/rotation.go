package domain

import (
	"fmt"
	"time"
)

// Step is a stage of the rotation pipeline. The zero value is the state before
// anything has been touched.
type Step int

const (
	// StepPending means nothing has happened yet.
	StepPending Step = iota
	// StepRead means the current value has been read from the primary file.
	StepRead
	// StepGenerated means a new credential exists but nothing was applied.
	StepGenerated
	// StepApplied means the backing service now accepts the new credential.
	StepApplied
	// StepVerified means the new credential was proven to work.
	StepVerified
	// StepFilesUpdated means every .env file carries the new value.
	StepFilesUpdated
	// StepRestarted means the containers were restarted and are healthy.
	StepRestarted
	// StepDone means the outcome has been recorded.
	StepDone
)

// String returns the step name used in logs and audit entries.
func (s Step) String() string {
	switch s {
	case StepPending:
		return "pending"
	case StepRead:
		return "read"
	case StepGenerated:
		return "generated"
	case StepApplied:
		return "applied"
	case StepVerified:
		return "verified"
	case StepFilesUpdated:
		return "files_updated"
	case StepRestarted:
		return "restarted"
	case StepDone:
		return "done"
	default:
		return "unknown"
	}
}

// Outcome is the terminal state of a rotation.
type Outcome string

const (
	// OutcomeSuccess means the rotation completed.
	OutcomeSuccess Outcome = "success"
	// OutcomeFailed means it did not, and compensation ran.
	OutcomeFailed Outcome = "failed"
)

// Rotation tracks one attempt at rotating one secret.
//
// It is the write model for compensation: every mutating step records what it
// needs to undo itself, and CompensationPlan turns that journal into the list
// of actions to run. Deciding what to undo is a domain rule, so it lives here
// rather than in the code that performs the I/O.
type Rotation struct {
	secret *Secret

	step      Step
	startedAt time.Time

	previous Credential
	next     Credential

	// fileBackups holds the original bytes of every file written, keyed by
	// path. Recording each file individually is what makes compensation
	// correct for multi-file secrets.
	fileBackups map[FilePath][]byte
	// writtenFiles preserves write order so compensation can mirror it.
	writtenFiles []FilePath

	// restarted records the containers actually restarted, already resolved
	// to real container names.
	restarted []ContainerRef
}

// BeginRotation starts tracking a rotation of the given secret.
func BeginRotation(secret *Secret, startedAt time.Time) *Rotation {
	return &Rotation{
		secret:      secret,
		step:        StepPending,
		startedAt:   startedAt,
		fileBackups: make(map[FilePath][]byte),
	}
}

// Secret returns the secret being rotated.
func (r *Rotation) Secret() *Secret { return r.secret }

// Step returns how far the rotation got.
func (r *Rotation) Step() Step { return r.step }

// StartedAt returns when the attempt began.
func (r *Rotation) StartedAt() time.Time { return r.startedAt }

// Previous returns the credential in force before the rotation.
func (r *Rotation) Previous() Credential { return r.previous }

// Next returns the credential being rotated to, zero until generated.
func (r *Rotation) Next() Credential { return r.next }

// RecordCurrentValue notes the credential that was in force, and the untouched
// contents of the primary file.
func (r *Rotation) RecordCurrentValue(current Credential, primarySnapshot []byte) {
	r.previous = current
	r.fileBackups[r.secret.PrimaryEnvFile()] = primarySnapshot
	r.advance(StepRead)
}

// RecordGenerated notes the new credential, before it has been applied.
func (r *Rotation) RecordGenerated(next Credential) {
	r.next = next
	r.advance(StepGenerated)
}

// RecordApplied notes that the backing service accepted the new credential.
// From this point the service must be restored if anything later fails.
func (r *Rotation) RecordApplied() { r.advance(StepApplied) }

// RecordVerified notes that the new credential was proven to work.
func (r *Rotation) RecordVerified() { r.advance(StepVerified) }

// RecordFileWritten notes that a file now carries the new value, keeping its
// previous contents for compensation.
func (r *Rotation) RecordFileWritten(path FilePath, snapshot []byte) {
	if _, ok := r.fileBackups[path]; !ok {
		r.fileBackups[path] = snapshot
	}
	r.writtenFiles = append(r.writtenFiles, path)
	r.advance(StepFilesUpdated)
}

// RecordRestarted notes the containers that were restarted.
func (r *Rotation) RecordRestarted(containers []ContainerRef) {
	r.restarted = append([]ContainerRef(nil), containers...)
	r.advance(StepRestarted)
}

// RecordDone marks the rotation complete.
func (r *Rotation) RecordDone() { r.advance(StepDone) }

// advance moves the pipeline forward, never backwards.
func (r *Rotation) advance(to Step) {
	if to > r.step {
		r.step = to
	}
}

// CompensationAction is one undo operation in a compensation plan.
type CompensationAction struct {
	// Type says what to undo.
	Type CompensationType
	// Path is set for RestoreFile.
	Path FilePath
	// Snapshot is the original file content for RestoreFile.
	Snapshot []byte
	// Containers is set for RestartContainers.
	Containers []ContainerRef
}

// CompensationType enumerates the undo operations.
type CompensationType string

const (
	// CompensationRestoreFile rewrites an .env file with its original bytes.
	CompensationRestoreFile CompensationType = "restore_file"
	// CompensationRestartContainers restarts containers so they pick the
	// restored values back up.
	CompensationRestartContainers CompensationType = "restart_containers"
	// CompensationRestoreCredential puts the old password back on the service.
	CompensationRestoreCredential CompensationType = "restore_credential"
)

// CompensationPlan returns the actions that undo whatever this rotation
// managed to do, in the order they must run.
//
// Files come back before containers restart, so a restart picks up the
// restored values; the backing service is restored last, because the service
// keeps working under the new credential until then and an early restore would
// leave running containers holding a password that no longer works.
//
// Every file that was written is restored, not just the first one.
func (r *Rotation) CompensationPlan() []CompensationAction {
	if r.step < StepGenerated {
		// Nothing was applied anywhere, so there is nothing to undo.
		return nil
	}

	var plan []CompensationAction

	for _, path := range r.writtenFiles {
		snapshot, ok := r.fileBackups[path]
		if !ok {
			continue
		}
		plan = append(plan, CompensationAction{
			Type:     CompensationRestoreFile,
			Path:     path,
			Snapshot: snapshot,
		})
	}

	if r.step >= StepRestarted && len(r.restarted) > 0 {
		plan = append(plan, CompensationAction{
			Type:       CompensationRestartContainers,
			Containers: append([]ContainerRef(nil), r.restarted...),
		})
	}

	if r.step >= StepApplied && r.secret.Kind().HasBackingService() {
		plan = append(plan, CompensationAction{Type: CompensationRestoreCredential})
	}

	return plan
}

// Record is the audit entry describing how a rotation ended.
type Record struct {
	SecretName SecretName
	Kind       Kind
	StartedAt  time.Time
	FinishedAt time.Time
	Outcome    Outcome
	LastStep   Step
	// Previous is the credential that was replaced. Whether it is persisted
	// in cleartext is the audit adapter's decision, made explicitly through
	// Credential.Expose.
	Previous Credential
	Details  string
}

// Succeeded builds the audit record for a completed rotation.
func (r *Rotation) Succeeded(finishedAt time.Time) Record {
	return Record{
		SecretName: r.secret.Name(),
		Kind:       r.secret.Kind(),
		StartedAt:  r.startedAt,
		FinishedAt: finishedAt,
		Outcome:    OutcomeSuccess,
		LastStep:   r.step,
		Previous:   r.previous,
	}
}

// Failed builds the audit record for a rotation that did not complete.
func (r *Rotation) Failed(finishedAt time.Time, cause error) Record {
	details := ""
	if cause != nil {
		details = cause.Error()
	}
	return Record{
		SecretName: r.secret.Name(),
		Kind:       r.secret.Kind(),
		StartedAt:  r.startedAt,
		FinishedAt: finishedAt,
		Outcome:    OutcomeFailed,
		LastStep:   r.step,
		Previous:   r.previous,
		Details:    details,
	}
}

// StateDescription summarises where a failed rotation left the system, for an
// operator who now has to decide what to do by hand.
func (r *Rotation) StateDescription(compensationErrs []error) string {
	if len(compensationErrs) == 0 {
		return fmt.Sprintf("rotation of %s failed at step %s and was fully rolled back",
			r.secret.Name(), r.step)
	}
	return fmt.Sprintf(
		"rotation of %s failed at step %s and rollback did not fully succeed (%d of the undo actions failed); "+
			"inspect the backing service and %s before rotating again",
		r.secret.Name(), r.step, len(compensationErrs), r.secret.PrimaryEnvFile())
}
