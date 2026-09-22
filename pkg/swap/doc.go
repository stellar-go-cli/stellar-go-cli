// Package swap quotes and executes asset swaps on the Stellar decentralized
// exchange using path payments.
//
// A Service is bound to a Stellar network (testnet or mainnet) and queries
// Horizon for path-payment quotes. ExecuteSwap signs and submits the
// resulting path payment with the active wallet's keypair.
package swap
