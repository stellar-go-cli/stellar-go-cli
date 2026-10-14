// Package x402 implements the x402 v2 HTTP payment protocol for Stellar.
//
// An x402-protected resource answers an initial request with HTTP 402 and a
// base64-encoded PAYMENT-REQUIRED challenge. The client selects a Stellar
// `exact`-scheme requirement, builds a SEP-41 transfer invocation whose payer
// authorization entry is signed via ENVELOPE_TYPE_SOROBAN_AUTHORIZATION, and
// retries the request carrying the transaction envelope in PAYMENT-SIGNATURE.
// The facilitator verifies, signs, and submits the transaction, then returns
// the settlement in PAYMENT-RESPONSE.
package x402
