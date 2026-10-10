package credential

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/GiulioSavini/secret-rotator/internal/domain"
)

// CommandRotator rotates a credential by running programs named in the
// configuration.
//
// Security is the whole design here, because this adapter runs code a
// configuration file chose:
//
//   - argv only. exec.CommandContext is given a program and arguments, never
//     a string handed to a shell, so there is nothing to inject into.
//   - no credential in argv. /proc/<pid>/cmdline is world-readable and `ps`
//     shows it to every user on the host, so values travel in the child's
//     environment, which only the same uid and root can read.
//   - no inherited environment. rotator's own environment holds
//     ROTATOR_MASTER_KEY; handing that to every hook script would make the
//     passphrase readable by anything the script runs. The child gets a
//     minimal environment plus whatever command.pass_env names explicitly.
//   - a timeout on every invocation that covers the whole process tree, so a
//     script that hangs, or that leaves a child behind, cannot wedge the
//     daemon.
type CommandRotator struct{}

// Kind returns domain.KindCommand.
func (CommandRotator) Kind() domain.Kind { return domain.KindCommand }

// Apply runs the apply command with the new credential.
func (r CommandRotator) Apply(ctx context.Context, target domain.Target, next domain.Credential) error {
	return r.run(ctx, target, invocation{
		operation: "apply",
		argv:      target.Command.Apply,
		values: map[string]domain.Credential{
			domain.EnvCommandNewSecret: next,
		},
	})
}

// Verify runs the verify command, if one is configured.
func (r CommandRotator) Verify(ctx context.Context, target domain.Target, c domain.Credential) error {
	if len(target.Command.Verify) == 0 {
		// Reported as a warning when the configuration is loaded; skipping is
		// the documented behaviour, not a silent failure.
		return nil
	}
	return r.run(ctx, target, invocation{
		operation: "verify",
		argv:      target.Command.Verify,
		values: map[string]domain.Credential{
			domain.EnvCommandNewSecret: c,
		},
	})
}

// Restore puts the previous credential back.
//
// With no dedicated restore command, the apply command runs again with the
// previous value, which is correct whenever applying is idempotent. The
// previous value is therefore exposed as the new secret as well, so one script
// can serve both directions without caring which way it is going.
func (r CommandRotator) Restore(ctx context.Context, target domain.Target, previous, current domain.Credential) error {
	argv, dedicated := target.Command.RestoreCommand()

	values := map[string]domain.Credential{
		domain.EnvCommandPreviousSecret: previous,
		domain.EnvCommandCurrentSecret:  current,
	}
	if !dedicated {
		values[domain.EnvCommandNewSecret] = previous
	}

	return r.run(ctx, target, invocation{operation: "restore", argv: argv, values: values})
}

// invocation is one command run.
type invocation struct {
	operation string
	argv      []string
	// values are the credentials to expose, keyed by environment variable.
	values map[string]domain.Credential
}

// run executes one invocation.
func (r CommandRotator) run(ctx context.Context, target domain.Target, inv invocation) error {
	if len(inv.argv) == 0 {
		return fmt.Errorf("command: no %s command configured", inv.operation)
	}

	runCtx, cancel := context.WithTimeout(ctx, target.Command.EffectiveTimeout())
	defer cancel()

	// Output goes to a temp file rather than an in-process buffer on purpose.
	// When Stdout is an io.Writer, Go wires a pipe and a copy goroutine, and
	// Wait blocks until that pipe closes -- which a grandchild holding the
	// write end keeps open. A script that backgrounded a child and exited
	// successfully was reported as a timeout. Handing the child a real file
	// descriptor removes the pipe, and with it the dependency on anything the
	// script leaves behind.
	sink, err := os.CreateTemp("", "rotator-cmd-*")
	if err != nil {
		return fmt.Errorf("command: could not capture output: %w", err)
	}
	sinkPath := sink.Name()
	defer func() {
		sink.Close()
		os.Remove(sinkPath)
	}()

	cmd := exec.CommandContext(runCtx, inv.argv[0], inv.argv[1:]...)
	cmd.Dir = target.Command.WorkingDir
	cmd.Env = buildEnv(target, inv)
	// No stdin: a script that waits for input would otherwise hang until the
	// timeout. On Unix a nil Stdin is /dev/null.
	cmd.Stdin = nil
	cmd.Stdout = sink
	cmd.Stderr = sink
	boundProcess(cmd)

	runErr := cmd.Run()
	if runErr == nil {
		return nil
	}

	detail := summarise(readSink(sinkPath))

	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("command: %s command timed out after %s%s",
			inv.operation, target.Command.EffectiveTimeout(), detail)
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return fmt.Errorf("command: %s command %q exited with status %d%s",
			inv.operation, inv.argv[0], exitErr.ExitCode(), detail)
	}

	return fmt.Errorf("command: could not run the %s command %q: %w",
		inv.operation, inv.argv[0], runErr)
}

// readSink reads back a bounded amount of a command's captured output.
func readSink(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	// One extra byte, so summarise can tell a full read from a truncated one.
	buf := make([]byte, maxCommandOutput+1)
	n, _ := io.ReadFull(f, buf)
	return string(buf[:n])
}

// buildEnv assembles the child's environment from scratch.
//
// Starting from nothing rather than os.Environ() is the point: it guarantees
// the master passphrase, and anything else in rotator's environment, is not
// handed to a configured script by accident.
func buildEnv(target domain.Target, inv invocation) []string {
	env := []string{
		// PATH and HOME are the minimum a script needs to find its tools and
		// its own configuration.
		"PATH=" + pathOrDefault(),
		"HOME=" + os.Getenv("HOME"),
		domain.EnvCommandOperation + "=" + inv.operation,
		domain.EnvCommandHost + "=" + target.Host,
		domain.EnvCommandUsername + "=" + target.AdminUser,
		domain.EnvCommandTargetUser + "=" + target.EffectiveTargetUser(),
		domain.EnvCommandDatabase + "=" + target.Database,
	}

	if port := target.EffectivePort(domain.KindCommand); port > 0 {
		env = append(env, domain.EnvCommandPort+"="+strconv.Itoa(port))
	}

	for name, credential := range inv.values {
		// Expose is called only here: this is the boundary where a credential
		// has to become a plain string.
		env = append(env, name+"="+credential.Expose())
	}

	// Explicitly allowed passthrough, for scripts that need a token or a
	// region from the ambient environment.
	for _, name := range target.Command.PassEnv {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}

	return env
}

// pathOrDefault returns rotator's PATH, or a sane default when it has none,
// which happens in a distroless container.
func pathOrDefault() string {
	if p := os.Getenv("PATH"); p != "" {
		return p
	}
	return "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
}

// processWaitDelay is how long Wait may keep draining a cancelled command's
// pipes before abandoning them. It only matters when the command left a child
// holding those pipes open.
const processWaitDelay = 2 * time.Second

// maxCommandOutput bounds how much of a command's output reaches an error.
const maxCommandOutput = 2000

// summarise renders captured output for an error message, bounded in size.
//
// Whatever a script prints ends up here, so a script that echoes the new
// credential puts it in an error message and from there into the audit trail.
// That is the script's choice to make; it is documented rather than silently
// stripped, because guessing which substring is a secret would be worse.
func summarise(out string) string {
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) > maxCommandOutput {
		trimmed = trimmed[:maxCommandOutput] + "... (output truncated)"
	}
	return ": " + trimmed
}

var _ domain.CredentialRotator = CommandRotator{}
