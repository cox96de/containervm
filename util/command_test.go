package util

import (
	"gotest.tools/v3/assert"
	"testing"
)

func TestRun(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		_, err := Run("ls", "-alh")
		assert.NilError(t, err)
	})
	t.Run("exit_1", func(t *testing.T) {
		_, err := Run("bash", "-c", "exit 1")
		assert.ErrorContains(t, err, "failed to run command")
	})
}
