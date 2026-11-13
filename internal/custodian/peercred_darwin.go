//go:build darwin

package custodian

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// checkPeer verifies the connecting peer runs as the daemon's UID via
// LOCAL_PEERCRED. Cross-user peers are refused — the socket file mode is the
// first barrier; this is defense in depth.
func checkPeer(conn net.Conn) error {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("not a unix connection")
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return fmt.Errorf("peer fd: %w", err)
	}
	var cred *unix.Xucred
	var verr error
	if err := raw.Control(func(fd uintptr) {
		cred, verr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	}); err != nil {
		return fmt.Errorf("peer cred: %w", err)
	}
	if verr != nil {
		return fmt.Errorf("peer cred: %w", verr)
	}
	if int64(cred.Uid) != int64(os.Geteuid()) {
		return fmt.Errorf("peer uid %d != daemon uid %d", cred.Uid, os.Geteuid())
	}
	return nil
}
