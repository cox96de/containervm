package util

import (
	"github.com/kdomanski/iso9660"
	"github.com/pkg/errors"
	"os"
	"path/filepath"
)

// GenISO generates an iso file with the given files and label.
func GenISO(workDir string, output string, files []string, label string) error {
	writer, err := iso9660.NewWriter()
	if err != nil {
		return errors.WithMessage(err, "failed to create iso writer")
	}
	defer writer.Cleanup()
	for _, file := range files {
		f, err := os.Open(filepath.Join(workDir, file))
		if err != nil {
			return errors.WithMessagef(err, "failed to open file %s", file)
		}
		if err = writer.AddFile(f, file); err != nil {
			return errors.WithMessagef(err, "failed to add file %s to iso", file)
		}
		_ = f.Close()
	}
	writeFile, err := os.OpenFile(filepath.Join(workDir, output), os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return errors.WithMessage(err, "failed to open iso file for writing")
	}
	return writer.WriteTo(writeFile, label)
}
