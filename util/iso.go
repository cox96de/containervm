package util

import "github.com/pkg/errors"

// GenISO generates an iso file with the given files and label.
func GenISO(workDir string, output string, files []string, label string) error {
	cmds := []string{"-output", output, "-volid", label, "-joliet", "-rock"}
	output, err := RunWithWorkDir(workDir, "genisoimage", append(cmds, files...)...)
	if err != nil {
		return errors.WithMessagef(err, "failed to gen iso with output: %s", output)
	}
	return nil
}
