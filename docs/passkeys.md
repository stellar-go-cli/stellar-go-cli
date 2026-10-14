# Passkey-Protected Wallets

Stellar Go CLI can protect a wallet's seed behind a real WebAuthn passkey
(Touch ID, Windows Hello, security keys, or platform authenticators).
This is **passkey-gated key access**: the wallet remains a normal Ed25519
Stellar keypair — the passkey authenticates you and gates use of the
encrypted seed. It is *not* a keyless smart contract account.

## Security model

- Registration runs a real WebAuthn ceremony against a loopback-only
  relying party (`http://localhost:<ephemeral-port>`) using
  `github.com/go-webauthn/webauthn`. Attestation, challenge binding,
  origin, and RP-ID are verified server-side — there is no fallback or
  mock path.
- On registration the seed is re-encrypted with AES-256-GCM and the
  wallet file stores only `privateKeyEnc` — no plaintext seed.
- The encryption key lives in the OS keychain (`github.com/zalando/go-keyring`:
  macOS Keychain, Secret Service, or Windows Credential Manager).
- `wallet passkey unlock` runs an assertion ceremony with
  `userVerification: required`. On success a short-lived grant
  (default 15 minutes) is stored in the keychain; every signing path
  resolves the seed only through that grant.
- Signing while locked fails with a descriptive error. `wallet passkey lock`
  revokes the grant immediately.
- Passkey credential records (credential ID, COSE public key, sign
  counter, transports) live in `~/.stellar-go-cli/state/passkeys/<addr>.json`
  at `0600`. The sign counter is updated on each unlock for clone
  detection.

## Commands

```bash
# Protect the active wallet (or pass --address)
stellar-go-cli wallet passkey register
stellar-go-cli wallet passkey register --address GABC...

# Unlock for signing (default 15m; override with --ttl)
stellar-go-cli wallet passkey unlock --ttl 30m

# Lock immediately
stellar-go-cli wallet passkey lock

# Show protection state for all wallets / credentials for one
stellar-go-cli wallet passkey status
stellar-go-cli wallet passkey list

# Remove protection (requires an assertion; restores plaintext seed)
stellar-go-cli wallet passkey disable
```

All commands open a browser window for the ceremony. If the browser does
not open, the terminal prints a copy-pasteable link that is valid only
for that ceremony session.

## What happens to existing commands

Everything resolves seeds through `wallet.ResolveSeed`:

- Plaintext wallets behave exactly as before.
- Protected + unlocked wallets sign normally (payments, swaps, contract
  deploy/invoke, DID/VC ops, asset issuance, x402, MCP tools).
- Protected + locked wallets fail with
  `wallet passkey unlock` instructions — no silent fallback.

`wallet export --private` requires the wallet to be unlocked, and
`wallet remove` cleans up the passkey file and keychain entries.

## MCP

`wallet_passkey` exposes `register | unlock | lock | status | disable`.
Ceremony actions (`register`, `unlock`, `disable`) open a browser on the
machine running the CLI — they require human interaction and do not work
in a fully headless environment. `status`, `lock`, and subsequent
signing tools work headlessly once a grant is active.

## Platform requirements

- A browser supporting WebAuthn (Chrome, Safari, Edge, Firefox) on the
  same machine.
- An OS keychain backend: macOS Keychain, Secret Service (`gnome-keyring`
  / KWallet via dbus) on Linux, Windows Credential Manager.
- Linux headless servers without Secret Service will fail to store the
  key — register only on machines with a keychain.
