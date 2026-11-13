//go:build !linux && !darwin

package custodian

import "net"

// checkPeer is a no-op where peer credentials aren't available — the socket
// file's 0600 mode and the state dir's 0700 mode remain the boundary.
func checkPeer(conn net.Conn) error { return nil }
