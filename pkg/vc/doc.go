// Package vc creates W3C Decentralized Identifiers (DIDs) and issues and
// verifies W3C Verifiable Credentials.
//
// Supported DID methods are did:web, did:key, did:ethr, and did:ebsi.
// did:ethr and did:ebsi identifiers embed the public key and are locally
// derived — they are not registered on any external registry.
//
// Credentials issued with an Ed25519 key carry Ed25519Signature2020 proofs;
// ECDSA (P-256) issuers produce ES256 JsonWebSignature2020 detached-JWS
// proofs. Verification resolves did:key, did:ebsi and public-key-form
// did:ethr identifiers locally, and fetches did:web documents over HTTPS.
// Unless an external issuer is configured, credentials are self-issued by
// the generated DID.
package vc
