package util

import (
	"os/exec"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type Option func(*exec.Cmd)

// Run starts a command and waits.
// The output includes its stdout & stderr. err is not nil when an error occurred or the exit code is not 0.
func Run(command string, args ...string) (output string, err error) {
	log.Infof("run: %s %+v", command, strings.Join(args, " "))
	cmd := exec.Command(command, args...)
	ouptut, err := cmd.CombinedOutput()
	if err != nil {
		return string(ouptut), errors.WithMessage(err, "failed to run command")
	}
	if exitCode := cmd.ProcessState.ExitCode(); exitCode != 0 {
		return string(ouptut), errors.Errorf("command exited with code %d", exitCode)
	}
	return string(ouptut), nil
}
