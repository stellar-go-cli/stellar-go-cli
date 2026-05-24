# Orchestrated Agreement Contract - Security Architecture

## Overview

The **Orchestrated Agreement Smart Contract** encodes the MozartPay 5-layer flow logic on Stellar Soroban. This document outlines the security architecture, threat model, and implementation guidelines.

## Architecture Layers

```
┌─────────────────────────────────────────────────────────────┐
│  Layer 5: Reporting & Audit                                 │
│  - On-chain audit trail                                     │
│  - ISO 20022 reference hashes                               │
│  - Immutable transaction records                            │
├─────────────────────────────────────────────────────────────┤
│  Layer 4: Integrations (OA Score, StellarCarbon, x402)     │
│  - External oracle data validation                          │
│  - Carbon credit retirement verification                    │
│  - Payment rail authorization                               │
├─────────────────────────────────────────────────────────────┤
│  Layer 3: Assets & Funding                                  │
│  - SAC/SEP-41 token compliance                              │
│  - Collateral locking mechanisms                            │
│  - Multi-signature requirements                             │
├─────────────────────────────────────────────────────────────┤
│  Layer 2: Wallet & Authentication                           │
│  - WebAuthn/Passkey verification                            │
│  - Address ownership proof                                  │
│  - Secondary signer management                              │
├─────────────────────────────────────────────────────────────┤
│  Layer 1: Identity & Attestation                            │
│  - DID verification (did:web, did:key, did:ethr, did:ebsi)  │
│  - Verifiable Credential validation                         │
│  - eIDAS 2.0 compliance hooks                               │
└─────────────────────────────────────────────────────────────┘
```

## Security Model

### Threat Actors

| Actor | Capability | Risk Level |
|-------|-----------|------------|
| External Attacker | No contract knowledge, network access | Medium |
| Malicious Initiator | Creates agreements, may try to exploit | High |
| Compromised Counterparty | Limited access, bilateral agreements | Medium |
| Admin/Owner | Full contract control | Critical |

### Key Security Properties

1. **Authorization**: All state-changing operations require cryptographic proof
2. **Atomicity**: Layer completion is atomic - partial states cannot be exploited
3. **Immutability**: Once executed, agreements cannot be altered
4. **Auditability**: All actions emit events for off-chain monitoring
5. **Pause Safety**: Emergency pause mechanism for vulnerability response

## Layer-by-Layer Security Analysis

### Layer 1: Identity & Attestation

**Purpose**: Establish trusted identity for agreement participants

**Security Controls**:
- `Address.require_auth()` for every identity attestation
- DID method validation (web/key/ethr/ebsi)
- VC issuance verification (off-chain oracle recommended)
- Attestation hash immutability

**Potential Vulnerabilities**:
- False DID attestation (mitigation: oracle verification)
- Replay attacks (mitigation: unique agreement IDs via PRNG)
- Timestamp manipulation (mitigation: ledger timestamp usage)

**Recommended Implementation**:
```rust
pub fn attest_identity(
    e: &Env,
    agreement_id: BytesN<32>,
    did: String,
    did_method: Symbol,
    vc_type: Symbol,
    attestation_hash: BytesN<32>,
) {
    let mut agreement = Self::get_agreement(e, agreement_id.clone());
    agreement.initiator.require_auth(); // Critical: auth check
    
    // Validate DID format
    assert!(
        did.len() > 0 && did.len() < 100, // Reasonable bounds
        "Invalid DID length"
    );
    
    // Verify attestation hash is non-zero
    let zero_hash = BytesN::from_array(e, &[0; 32]);
    assert!(attestation_hash != zero_hash, "Invalid attestation hash");
    
    // ... rest of implementation
}
```

### Layer 2: Wallet & Authentication

**Purpose**: Link blockchain addresses to verified identities

**Security Controls**:
- Passkey/WebAuthn status tracking
- Address ownership verification (recommended: signature challenge)
- Secondary signer whitelisting

**Potential Vulnerabilities**:
- Address spoofing (mitigation: challenge-response)
- Unauthorized signer addition (mitigation: owner-only updates)

**Recommended Implementation**:
```rust
pub fn connect_wallet(
    e: &Env,
    agreement_id: BytesN<32>,
    stellar_address: Address,
    wallet_type: Symbol,
    passkey_enabled: bool,
) {
    let mut agreement = Self::get_agreement(e, agreement_id.clone());
    agreement.initiator.require_auth();
    
    // Verify address is not zero
    assert!(stellar_address != Address::from_string(&String::from_str(e, "")), 
            "Invalid address");
    
    // Optional: Verify address ownership via signature
    // This would require an additional signature parameter
    
    // ... rest of implementation
}
```

### Layer 3: Assets & Funding

**Purpose**: Manage token deposits and collateral

**Security Controls**:
- SEP-41 compliance verification
- Contract ID validation for SAC tokens
- Amount bounds checking
- Funding status tracking

**Potential Vulnerabilities**:
- Integer overflow (mitigation: i128 bounds)
- Invalid contract IDs (mitigation: format validation)
- Re-entrancy (mitigation: state updates before external calls)

**Critical Implementation Notes**:
- Use `checked_add`/`checked_sub` for arithmetic
- Validate contract_id format (CA... for Stellar)
- Emit events for all asset movements

### Layer 4: Integrations

**Purpose**: Connect external oracles and services

**Security Controls**:
- OA Score bounds (0-1000)
- Carbon credit verification hooks
- x402 payment request validation
- Tempo FX rate freshness checks

**Potential Vulnerabilities**:
- Oracle manipulation (mitigation: multi-source validation)
- Stale FX rates (mitigation: expiration timestamps)
- Invalid score injection (mitigation: range checks)

**Recommended Implementation**:
```rust
pub fn add_integrations(
    e: &Env,
    agreement_id: BytesN<32>,
    oa_score: Option<i32>,
    carbon_credits: i128,
    x402_enabled: bool,
    tempo_fx_rate: Option<i128>,
) {
    // Validate OA score bounds
    if let Some(score) = oa_score {
        assert!(score >= 0 && score <= 1000, "OA score out of bounds");
    }
    
    // Validate carbon credits non-negative
    assert!(carbon_credits >= 0, "Carbon credits must be positive");
    
    // ... rest of implementation
}
```

### Layer 5: Reporting & Execution

**Purpose**: Finalize agreements with audit trail

**Security Controls**:
- All-layer completion verification
- Expiration timestamp validation
- Audit hash generation (SHA-256)
- Irreversible state transitions

**Critical Implementation Notes**:
- Verify ALL previous layers are complete before execution
- Use ledger timestamp for time checks (not system time)
- Generate audit hash from immutable data

## Data Structures

### Storage Layout

```rust
#[contracttype]
pub enum DataKey {
    Owner,              // Admin address
    Paused,             // Emergency pause flag
    AgreementCount,     // Total agreements created
    Agreement(BytesN<32>), // Individual agreement storage
}
```

**Security Considerations**:
- Instance storage for owner/pause (global state)
- Persistent storage for agreements (user data)
- Separate keys prevent accidental overwrites

### Agreement State Machine

```
Draft → Active → Funded → Executed → Settled
  ↓       ↓        ↓         ↓
Error  Expired  Disputed  (immutable)
```

**Security Guarantees**:
- One-way state progression (no rollback)
- Expiration checks at critical transitions
- State validation on every update

## Authorization Patterns

### Role-Based Access Control

| Function | Initiator | Counterparty | Owner |
|----------|-----------|--------------|-------|
| create_agreement | ✓ | ✗ | ✗ |
| attest_identity | ✓ | ✗ | ✗ |
| connect_wallet | ✓ | ✗ | ✗ |
| fund_and_set_asset | ✓ | ✗ | ✗ |
| add_integrations | ✓ | ✗ | ✗ |
| execute_agreement | ✓ | ✗ | ✗ |
| settle_agreement | ✓ | ✗ | ✗ |
| retire_carbon | ✓ | ✗ | ✗ |
| pause | ✗ | ✗ | ✓ |
| unpause | ✗ | ✗ | ✓ |

### Critical Authorization Checks

1. **Always use `Address.require_auth()`** - Never `env.require_auth()`
2. **Verify agreement ownership** - Caller must be initiator
3. **Check pause state** - Reject operations when paused
4. **Validate expiration** - Reject expired agreements

## Emergency Procedures

### Pause Mechanism

**Purpose**: Halt operations during security incidents

**Usage**:
- Owner can pause/unpause contract
- Query functions remain accessible
- State-changing operations revert when paused

**Implementation**:
```rust
pub fn pause(e: &Env) {
    let owner: Address = e.storage().instance().get(&DataKey::Owner).unwrap();
    owner.require_auth();
    e.storage().instance().set(&DataKey::Paused, &true);
}
```

### Recovery Scenarios

1. **Contract Vulnerability Discovered**:
   - Owner pauses contract immediately
   - Analyze impact via event logs
   - Deploy fixed contract
   - Migrate state (if applicable)

2. **Oracle Compromise**:
   - Pause integration functions
   - Verify existing agreement integrity
   - Switch to backup oracle sources

3. **Key Compromise**:
   - Owner rotation mechanism (recommended addition)
   - Multi-signature upgrade path

## Testing Security

### Unit Test Requirements

1. **Authorization Tests**:
   - Verify all functions reject unauthorized callers
   - Test boundary conditions (expired, paused states)

2. **State Transition Tests**:
   - Verify one-way state progression
   - Test invalid transitions

3. **Input Validation Tests**:
   - Bounds checking (scores, amounts, timestamps)
   - Format validation (DIDs, addresses, hashes)

4. **Re-entrancy Tests**:
   - Attempt recursive calls
   - Verify state consistency

### Integration Test Requirements

1. **End-to-End Flow**:
   - Create → Attest → Wallet → Fund → Integrate → Execute → Settle
   - Verify all layers complete correctly

2. **Event Emission**:
   - Verify all state changes emit events
   - Check event parameter correctness

3. **Cross-Contract Interactions**:
   - SAC token transfers
   - Oracle callbacks

## Audit Checklist

### Pre-Deployment Verification

- [ ] All `require_auth()` calls use `Address` not `Env`
- [ ] No unchecked arithmetic operations
- [ ] All event emissions include agreement identifier
- [ ] Pause mechanism tested and functional
- [ ] State machine prevents invalid transitions
- [ ] No storage key collisions
- [ ] All external inputs validated
- [ ] No use of `panic!` for expected errors (use proper error handling)

### Post-Deployment Monitoring

- [ ] Event log monitoring for suspicious patterns
- [ ] Pause mechanism access tested
- [ ] Gas usage tracking for DoS prevention
- [ ] Oracle data freshness monitoring

## Implementation Recommendations

1. **Use checked arithmetic**: `checked_add`, `checked_sub`, `checked_mul`
2. **Validate all inputs**: Length bounds, format, range
3. **Emit comprehensive events**: Every state change should be observable
4. **Test edge cases**: Zero values, maximum values, boundary conditions
5. **Document assumptions**: What is trusted vs. verified on-chain
6. **Plan for upgrades**: Consider proxy patterns for future enhancements

## References

- [Stellar Soroban Security Best Practices](https://soroban.stellar.org/docs/learn/security)
- [OpenZeppelin Contracts for Stellar](https://github.com/OpenZeppelin/stellar-contracts)
- [W3C DID Core Specification](https://www.w3.org/TR/did-core/)
- [SEP-41 Token Interface](https://github.com/stellar/stellar-protocol/blob/master/ecosystem/sep-0041.md)
