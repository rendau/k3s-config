package sshx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTarget_args(t *testing.T) {
	tg := Target{Host: "1.2.3.4", User: "user"}
	assert.Equal(t, []string{
		"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=15",
		"-p", "22", "user@1.2.3.4",
	}, tg.args())

	tg = Target{Host: "h", Port: 2222, User: "root", Key: "/k/id", ExtraOpts: "-o LogLevel=ERROR -J bastion"}
	assert.Equal(t, []string{
		"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=15",
		"-p", "2222", "-i", "/k/id", "-o", "IdentitiesOnly=yes", "-o", "LogLevel=ERROR", "-J", "bastion", "root@h",
	}, tg.args())
	assert.Equal(t, "root@h:2222", tg.String())
}
