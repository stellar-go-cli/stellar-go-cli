// Package webauthn implements a relying party for WebAuthn ceremonies over a
// loopback HTTP server. It is used to gate wallet key access behind real
// passkey (FIDO2) assertions.
package webauthn

import (
	"github.com/go-webauthn/webauthn/webauthn"
)

// User adapts a wallet to the webauthn.User interface.
type User struct {
	Address     string
	Name        string
	Credentials []webauthn.Credential
}

func (u *User) WebAuthnID() []byte                         { return []byte(u.Address) }
func (u *User) WebAuthnName() string                       { return u.Name }
func (u *User) WebAuthnDisplayName() string                { return u.Name }
func (u *User) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }
