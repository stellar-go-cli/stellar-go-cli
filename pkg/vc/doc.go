// Package vc creates W3C Decentralized Identifiers (DIDs) and issues and
// verifies W3C Verifiable Credentials.
//
// Supported DID methods are did:web, did:key, did:ethr, and did:ebsi.
// Credentials are signed with Ed25519Signature2020 proofs (or JWS when an
// ECDSA key is supplied). Unless an external issuer is configured,
// credentials are self-issued by the generated DID.
package vc
