package util

import (
	"gotest.tools/v3/assert"
	"net"
	"os"
	"testing"
)

func TestGetDefaultNIC(t *testing.T) {
	t.Logf("%+v", os.Args)
	defaultNIC, err := GetDefaultNIC()
	assert.NilError(t, err)
	t.Logf("%+v", defaultNIC)
}

func TestGetRandomMAC(t *testing.T) {
	mac := GetRandomMAC()
	hw, err := net.ParseMAC(mac.String())
	assert.NilError(t, err)
	assert.DeepEqual(t, mac, hw)
}
