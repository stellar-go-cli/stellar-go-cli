// Package soroban is a pure-Go client for the Soroban RPC API: it uploads
// WASM bytecode, deploys contract instances, invokes contract methods, runs
// read-only simulations, and queries contract events — no external stellar
// CLI required.
//
// Transactions are always simulated before submission so resource footprints
// and auth entries are assembled correctly.
package soroban
