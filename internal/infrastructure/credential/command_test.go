package credential

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/GiulioSavini/secret-rotator/internal/domain"
)

// writeScript creates an executable shell script and returns its path.
func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700))
	return path
}

// targetWith builds a command target around a spec.
func targetWith(spec domain.CommandSpec) domain.Target {
	return domain.Target{
		Host:       "minio",
		Port:       9000,
		AdminUser:  "admin",
		TargetUser: "app",
		Database:   "",
		Command:    spec,
	}
}

func TestApplyRunsTheConfiguredProgram(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "applied")
	script := writeScript(t, dir, "apply.sh", `printf '%s' "$ROTATOR_NEW_SECRET" > `+marker+`
`)

	err := CommandRotator{}.Apply(context.Background(),
		targetWith(domain.CommandSpec{Apply: []string{script}}),
		domain.NewCredential("brand-new-value"))
	require.NoError(t, err)

	body, readErr := os.ReadFile(marker)
	require.NoError(t, readErr)
	assert.Equal(t, "brand-new-value", string(body))
}

func TestTheCredentialNeverReachesArgv(t *testing.T) {
	// /proc/<pid>/cmdline is world-readable, so a credential in argv is
	// visible to every user on the host. This test is the guard for that.
	dir := t.TempDir()
	capture := filepath.Join(dir, "argv")
	script := writeScript(t, dir, "apply.sh", `printf '%s' "$*" > `+capture+`
`)

	err := CommandRotator{}.Apply(context.Background(),
		targetWith(domain.CommandSpec{Apply: []string{script, "--role", "app"}}),
		domain.NewCredential("super-secret-value"))
	require.NoError(t, err)

	argv, readErr := os.ReadFile(capture)
	require.NoError(t, readErr)
	assert.Equal(t, "--role app", string(argv))
	assert.NotContains(t, string(argv), "super-secret-value")
}

func TestTheChildDoesNotInheritRotatorsEnvironment(t *testing.T) {
	// The master passphrase lives in rotator's environment. Handing it to
	// every configured script would make it readable by anything the script
	// runs, so the child's environment is built from scratch.
	t.Setenv("ROTATOR_MASTER_KEY", "the-master-passphrase")
	t.Setenv("SOME_UNRELATED_TOKEN", "unrelated-value")

	dir := t.TempDir()
	capture := filepath.Join(dir, "env")
	script := writeScript(t, dir, "apply.sh", `env > `+capture+`
`)

	err := CommandRotator{}.Apply(context.Background(),
		targetWith(domain.CommandSpec{Apply: []string{script}}),
		domain.NewCredential("new-value"))
	require.NoError(t, err)

	env, readErr := os.ReadFile(capture)
	require.NoError(t, readErr)

	assert.NotContains(t, string(env), "the-master-passphrase")
	assert.NotContains(t, string(env), "unrelated-value")
	// The context a script does need is there.
	assert.Contains(t, string(env), "ROTATOR_NEW_SECRET=new-value")
	assert.Contains(t, string(env), "ROTATOR_OPERATION=apply")
	assert.Contains(t, string(env), "ROTATOR_HOST=minio")
	assert.Contains(t, string(env), "ROTATOR_TARGET_USER=app")
}

func TestPassEnvForwardsOnlyWhatIsNamed(t *testing.T) {
	t.Setenv("AWS_REGION", "eu-south-1")
	t.Setenv("NOT_FORWARDED", "nope")

	dir := t.TempDir()
	capture := filepath.Join(dir, "env")
	script := writeScript(t, dir, "apply.sh", `env > `+capture+`
`)

	err := CommandRotator{}.Apply(context.Background(),
		targetWith(domain.CommandSpec{Apply: []string{script}, PassEnv: []string{"AWS_REGION"}}),
		domain.NewCredential("new-value"))
	require.NoError(t, err)

	env, readErr := os.ReadFile(capture)
	require.NoError(t, readErr)
	assert.Contains(t, string(env), "AWS_REGION=eu-south-1")
	assert.NotContains(t, string(env), "nope")
}

func TestAFailingCommandReportsStatusAndOutput(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "apply.sh", `echo "the service said no" >&2
exit 3
`)

	err := CommandRotator{}.Apply(context.Background(),
		targetWith(domain.CommandSpec{Apply: []string{script}}),
		domain.NewCredential("new-value"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 3")
	assert.Contains(t, err.Error(), "the service said no")
}

func TestAHangingCommandIsKilled(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "apply.sh", "sleep 30\n")

	start := time.Now()
	err := CommandRotator{}.Apply(context.Background(),
		targetWith(domain.CommandSpec{Apply: []string{script}, Timeout: 300 * time.Millisecond}),
		domain.NewCredential("new-value"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
	assert.Less(t, time.Since(start), 5*time.Second, "the timeout must actually cut it off")
}

func TestAMissingProgramIsReportedClearly(t *testing.T) {
	err := CommandRotator{}.Apply(context.Background(),
		targetWith(domain.CommandSpec{Apply: []string{"/nonexistent/rotate.sh"}}),
		domain.NewCredential("new-value"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not run")
	assert.Contains(t, err.Error(), "/nonexistent/rotate.sh")
}

func TestVerifyIsSkippedWhenNotConfigured(t *testing.T) {
	err := CommandRotator{}.Verify(context.Background(),
		targetWith(domain.CommandSpec{Apply: []string{"/bin/true"}}),
		domain.NewCredential("new-value"))
	require.NoError(t, err, "a missing verify command is documented behaviour, not a failure")
}

func TestVerifyRunsItsOwnCommand(t *testing.T) {
	dir := t.TempDir()
	capture := filepath.Join(dir, "verify")
	script := writeScript(t, dir, "verify.sh", `printf '%s %s' "$ROTATOR_OPERATION" "$ROTATOR_NEW_SECRET" > `+capture+`
`)

	err := CommandRotator{}.Verify(context.Background(),
		targetWith(domain.CommandSpec{Apply: []string{"/bin/true"}, Verify: []string{script}}),
		domain.NewCredential("checked-value"))
	require.NoError(t, err)

	body, readErr := os.ReadFile(capture)
	require.NoError(t, readErr)
	assert.Equal(t, "verify checked-value", string(body))
}

func TestRestoreUsesTheDedicatedCommandWhenGiven(t *testing.T) {
	dir := t.TempDir()
	capture := filepath.Join(dir, "restore")
	script := writeScript(t, dir, "restore.sh",
		`printf '%s|%s|%s|%s' "$ROTATOR_OPERATION" "$ROTATOR_PREVIOUS_SECRET" "$ROTATOR_CURRENT_SECRET" "$ROTATOR_NEW_SECRET" > `+capture+`
`)

	err := CommandRotator{}.Restore(context.Background(),
		targetWith(domain.CommandSpec{Apply: []string{"/bin/false"}, Restore: []string{script}}),
		domain.NewCredential("old-value"), domain.NewCredential("current-value"))
	require.NoError(t, err)

	body, readErr := os.ReadFile(capture)
	require.NoError(t, readErr)
	// A dedicated restore command is told both values and is not handed
	// ROTATOR_NEW_SECRET, so it cannot confuse the direction.
	assert.Equal(t, "restore|old-value|current-value|", string(body))
}

func TestRestoreFallsBackToApplyingThePreviousValue(t *testing.T) {
	// With no restore command, undoing a rotation is applying the old value
	// again, which is correct whenever applying is idempotent.
	dir := t.TempDir()
	capture := filepath.Join(dir, "applied")
	script := writeScript(t, dir, "apply.sh",
		`printf '%s|%s' "$ROTATOR_OPERATION" "$ROTATOR_NEW_SECRET" > `+capture+`
`)

	err := CommandRotator{}.Restore(context.Background(),
		targetWith(domain.CommandSpec{Apply: []string{script}}),
		domain.NewCredential("old-value"), domain.NewCredential("current-value"))
	require.NoError(t, err)

	body, readErr := os.ReadFile(capture)
	require.NoError(t, readErr)
	assert.Equal(t, "restore|old-value", string(body))
}

func TestWorkingDirectoryIsHonoured(t *testing.T) {
	dir := t.TempDir()
	capture := filepath.Join(dir, "pwd")
	script := writeScript(t, dir, "apply.sh", `pwd > `+capture+`
`)

	err := CommandRotator{}.Apply(context.Background(),
		targetWith(domain.CommandSpec{Apply: []string{script}, WorkingDir: dir}),
		domain.NewCredential("new-value"))
	require.NoError(t, err)

	body, readErr := os.ReadFile(capture)
	require.NoError(t, readErr)
	// macOS reports /private/var for /var, so compare the resolved paths.
	wantResolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	assert.Equal(t, wantResolved, strings.TrimSpace(string(body)))
}

func TestLargeOutputIsTruncatedInTheError(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "apply.sh", `i=0
while [ $i -lt 500 ]; do echo "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"; i=$((i+1)); done
exit 1
`)

	err := CommandRotator{}.Apply(context.Background(),
		targetWith(domain.CommandSpec{Apply: []string{script}}),
		domain.NewCredential("new-value"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "output truncated")
	assert.Less(t, len(err.Error()), 3000, "a chatty script must not fill the audit trail")
}

func TestRegistryResolvesTheCommandKind(t *testing.T) {
	rotator, err := NewRegistry().For(domain.KindCommand)
	require.NoError(t, err)
	assert.Equal(t, domain.KindCommand, rotator.Kind())
}

func TestASuccessfulCommandThatLeavesAChildBehindStillReturnsAtOnce(t *testing.T) {
	// Regression test. With output on an in-process buffer, Go wires a pipe
	// and Wait blocks until it closes; a backgrounded grandchild holds the
	// write end open, so a script that exited 0 was reported as a timeout
	// after the full timeout plus the wait delay. Output now goes to a file
	// descriptor, so Wait returns as soon as the process does.
	dir := t.TempDir()
	script := writeScript(t, dir, "apply.sh", `sleep 30 &
echo started
exit 0
`)

	start := time.Now()
	err := CommandRotator{}.Apply(context.Background(),
		targetWith(domain.CommandSpec{Apply: []string{script}, Timeout: 500 * time.Millisecond}),
		domain.NewCredential("new-value"))

	require.NoError(t, err, "the command succeeded; a lingering child is not a failure")
	assert.Less(t, time.Since(start), 400*time.Millisecond,
		"it must not wait out the timeout for a command that already exited")
}

func TestStdinIsClosed(t *testing.T) {
	// A script that reads stdin would otherwise hang until the timeout.
	dir := t.TempDir()
	script := writeScript(t, dir, "apply.sh", `read line
echo "read: $line"
exit 0
`)

	start := time.Now()
	err := CommandRotator{}.Apply(context.Background(),
		targetWith(domain.CommandSpec{Apply: []string{script}, Timeout: 2 * time.Second}),
		domain.NewCredential("new-value"))

	// `read` hits EOF immediately and returns non-zero, which is a fast,
	// legible failure rather than a hang.
	assert.Less(t, time.Since(start), time.Second)
	_ = err
}
