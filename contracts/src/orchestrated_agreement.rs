// SPDX-License-Identifier: MIT
// Compatible with OpenZeppelin Stellar Soroban Contracts ^0.6.0

#![no_std]

use soroban_sdk::{
    contract, contracterror, contractimpl, contracttype, 
    panic_with_error, Address, BytesN, Env, Vec, Symbol, Map, String, Bytes,
};
use stellar_access::ownable::{self as ownable};
use stellar_contract_utils::pausable::{self as pausable};
use stellar_macros::{only_owner, when_not_paused};

// ─────────────────────────────────────────────
// Custom Errors (OpenZeppelin Pattern)
// ─────────────────────────────────────────────

#[contracterror]
#[derive(Copy, Clone, Debug, Eq, PartialEq, PartialOrd, Ord)]
#[repr(u32)]
pub enum OrchestratedAgreementError {
    // Authorization errors (1xx)
    Unauthorized = 100,
    NotInitiator = 101,
    NotOwner = 102,
    NotCounterparty = 103,

    // State errors (2xx)
    ContractPaused = 200,
    AgreementNotFound = 201,
    InvalidStateTransition = 202,
    AgreementExpired = 203,
    AgreementNotExpired = 204,
    LayerNotComplete = 205,

    // Input validation errors (3xx)
    InvalidDID = 300,
    InvalidAddress = 301,
    InvalidAmount = 302,
    InvalidScore = 303,
    InvalidTimestamp = 304,
    ZeroValue = 305,
    InvalidHash = 306,

    // Agreement lifecycle errors (4xx)
    IdentityRequired = 400,
    WalletRequired = 401,
    AssetRequired = 402,
    AlreadyExecuted = 403,
    AlreadySettled = 404,
    DisputeActive = 405,

    // Integration errors (5xx)
    CarbonCreditsNegative = 500,
    OracleDataStale = 501,
}

// ─────────────────────────────────────────────
// Contract State & Types
// ─────────────────────────────────────────────

#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub enum AgreementState {
    Draft,      // Initial creation
    Active,     // Identity verified, wallet connected
    Funded,     // Collateral/asset deposited
    Executed,   // Payment/asset transfer complete
    Settled,    // All obligations fulfilled
    Disputed,   // Conflict resolution needed
    Expired,    // Past expiration timestamp
    Cancelled,  // Cancelled by initiator
}

#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub enum DIDMethod {
    Web,
    Key,
    Ethr,
    Ebsi,
}

#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct IdentityLayer {
    pub did: String,                    // did:web, did:key, did:ethr, did:ebsi
    pub did_method: DIDMethod,          // Enum for type safety
    pub vc_issued: bool,                // Verifiable credential issued
    pub vc_type: Symbol,                // national_id, business_license, etc.
    pub attested_at: u64,               // Timestamp of attestation
    pub attestation_hash: BytesN<32>,   // Hash of attestation record
    pub verifier_address: Option<Address>, // On-chain verifier if applicable
}

#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct WalletLayer {
    pub stellar_address: Address,       // Primary Stellar address
    pub secondary_addresses: Vec<Address>, // Additional signers
    pub passkey_enabled: bool,          // WebAuthn/FIDO2 enabled
    pub wallet_type: Symbol,            // wwwallet/external/testnet
    pub funded: bool,                   // Account has minimum balance
    pub required_signers: u32,          // Multi-sig threshold
}

#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AssetLayer {
    pub asset_code: Symbol,             // USDC, XLM, custom token
    pub contract_id: Option<BytesN<32>>, // SAC contract address
    pub amount: i128,                   // Amount in stroops/smallest unit
    pub locked_amount: i128,            // Amount locked as collateral
    pub asset_type: Symbol,             // fungible/non-fungible
    pub sep41_compliant: bool,          // SEP-41 standard compliance
    pub escrow_released: bool,          // Whether escrow is released
}

#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct IntegrationLayer {
    pub oa_score: Option<i32>,          // Orchestrated Agreement score (0-1000)
    pub oa_grade: Symbol,               // AAA, AA, A, BBB, etc.
    pub carbon_credits: i128,           // Tonnes of CO2 offset
    pub carbon_retired: bool,           // Credits retired on-chain
    pub x402_enabled: bool,             // HTTP 402 pay-per-use enabled
    pub x402_endpoint: Option<String>,  // x402 payment endpoint
    pub tempo_fx_rate: Option<i128>,    // FX rate if using Tempo
    pub tempo_quote_id: Option<BytesN<32>>,
    pub tempo_expires_at: Option<u64>,  // Quote expiration
}

#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ReportingLayer {
    pub audit_hash: BytesN<32>,         // Hash of full audit trail
    pub iso20022_ref: Option<String>, // ISO 20022 message reference
    pub tx_hash: Option<BytesN<32>>,    // Final settlement transaction
    pub report_generated: bool,       // Compliance report created
    pub dispute_resolution_hash: Option<BytesN<32>>, // Dispute outcome
}

#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct OrchestratedAgreement {
    pub id: BytesN<32>,                 // Unique agreement ID
    pub state: AgreementState,
    pub initiator: Address,             // Agreement creator
    pub counterparty: Option<Address>,  // Other party (if bilateral)
    pub created_at: u64,
    pub updated_at: u64,
    pub expires_at: Option<u64>,
    pub dispute_window_end: Option<u64>, // Time window for disputes
    pub identity: Option<IdentityLayer>,
    pub wallet: Option<WalletLayer>,
    pub asset: Option<AssetLayer>,
    pub integration: Option<IntegrationLayer>,
    pub reporting: Option<ReportingLayer>,
    pub metadata: Map<Symbol, String>, // Additional key-value data
    pub version: u32,                  // Contract version for upgrades
}

// Storage keys - added Owner back since OpenZeppelin module doesn't expose getter
#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub enum DataKey {
    Owner,
    AgreementCount,
    Agreement(BytesN<32>),
    InitiatorAgreements(Address), // List of agreements by initiator
}

// ─────────────────────────────────────────────
// Events
// ─────────────────────────────────────────────

pub fn emit_agreement_created(
    e: &Env,
    agreement_id: &BytesN<32>,
    initiator: &Address,
    counterparty: &Option<Address>,
    timestamp: u64,
) {
    e.events().publish(
        (Symbol::new(e, "agreement_created"), agreement_id.clone()),
        (initiator.clone(), counterparty.clone(), timestamp),
    );
}

pub fn emit_identity_attested(
    e: &Env,
    agreement_id: &BytesN<32>,
    did_method: &DIDMethod,
    timestamp: u64,
) {
    e.events().publish(
        (Symbol::new(e, "identity_attested"), agreement_id.clone()),
        (did_method.clone(), timestamp),
    );
}

pub fn emit_wallet_connected(
    e: &Env,
    agreement_id: &BytesN<32>,
    stellar_address: &Address,
    wallet_type: &Symbol,
) {
    e.events().publish(
        (Symbol::new(e, "wallet_connected"), agreement_id.clone()),
        (stellar_address.clone(), wallet_type.clone()),
    );
}

pub fn emit_asset_funded(
    e: &Env,
    agreement_id: &BytesN<32>,
    asset_code: &Symbol,
    amount: i128,
    locked: i128,
) {
    e.events().publish(
        (Symbol::new(e, "asset_funded"), agreement_id.clone()),
        (asset_code.clone(), amount, locked),
    );
}

pub fn emit_integration_added(
    e: &Env,
    agreement_id: &BytesN<32>,
    oa_score: i32,
    carbon_credits: i128,
) {
    e.events().publish(
        (Symbol::new(e, "integration_added"), agreement_id.clone()),
        (oa_score, carbon_credits),
    );
}

pub fn emit_carbon_retired(
    e: &Env,
    agreement_id: &BytesN<32>,
    timestamp: u64,
) {
    e.events().publish(
        (Symbol::new(e, "carbon_retired"), agreement_id.clone()),
        timestamp,
    );
}

pub fn emit_agreement_executed(
    e: &Env,
    agreement_id: &BytesN<32>,
    tx_hash: &BytesN<32>,
    timestamp: u64,
) {
    e.events().publish(
        (Symbol::new(e, "agreement_executed"), agreement_id.clone()),
        (tx_hash.clone(), timestamp),
    );
}

pub fn emit_agreement_settled(
    e: &Env,
    agreement_id: &BytesN<32>,
    timestamp: u64,
) {
    e.events().publish(
        (Symbol::new(e, "agreement_settled"), agreement_id.clone()),
        timestamp,
    );
}

pub fn emit_agreement_disputed(
    e: &Env,
    agreement_id: &BytesN<32>,
    dispute_hash: &BytesN<32>,
    timestamp: u64,
) {
    e.events().publish(
        (Symbol::new(e, "agreement_disputed"), agreement_id.clone()),
        (dispute_hash.clone(), timestamp),
    );
}

pub fn emit_agreement_cancelled(
    e: &Env,
    agreement_id: &BytesN<32>,
    timestamp: u64,
) {
    e.events().publish(
        (Symbol::new(e, "agreement_cancelled"), agreement_id.clone()),
        timestamp,
    );
}

pub fn emit_state_transition(
    e: &Env,
    agreement_id: &BytesN<32>,
    old_state: &AgreementState,
    new_state: &AgreementState,
) {
    e.events().publish(
        (Symbol::new(e, "state_transition"), agreement_id.clone()),
        (old_state.clone(), new_state.clone()),
    );
}

// ─────────────────────────────────────────────
// Contract Implementation
// ─────────────────────────────────────────────

#[contract]
pub struct OrchestratedAgreementContract;

#[contractimpl]
impl OrchestratedAgreementContract {
    // ─── Constructor & Admin ──────────────────

    /// Constructor - initializes with owner using OpenZeppelin ownable module
    pub fn __constructor(e: &Env, owner: Address) {
        ownable::set_owner(e, &owner);
        e.storage().instance().set(&DataKey::Owner, &owner);
        e.storage().instance().set(&DataKey::AgreementCount, &0u64);
    }

    /// Transfer ownership to new address (OpenZeppelin only_owner macro)
    #[only_owner]
    pub fn transfer_ownership(e: &Env, new_owner: Address) {
        let current_owner: Address = e.storage().instance().get(&DataKey::Owner).expect("Owner not set");
        if new_owner == current_owner {
            panic_with_error!(e, OrchestratedAgreementError::Unauthorized);
        }
        ownable::set_owner(e, &new_owner);
        e.storage().instance().set(&DataKey::Owner, &new_owner);
    }

    /// Get current owner
    pub fn owner(e: &Env) -> Address {
        e.storage().instance().get(&DataKey::Owner).expect("Owner not set")
    }

    // ─── Pause Mechanism (OpenZeppelin Pausable) ──────────────────────

    /// Pause contract operations (owner only)
    #[only_owner]
    pub fn pause(e: &Env) {
        pausable::pause(e);
    }

    /// Unpause contract operations (owner only)
    #[only_owner]
    pub fn unpause(e: &Env) {
        pausable::unpause(e);
    }

    /// Check if contract is paused (OpenZeppelin Pausable)
    pub fn is_paused(e: &Env) -> bool {
        pausable::paused(e)
    }

    // ─── Agreement Lifecycle ──────────────────

    /// Create a new orchestrated agreement (Layer 0: Draft)
    /// 
    /// # Arguments
    /// * `initiator` - The agreement creator (must authenticate)
    /// * `counterparty` - Optional other party for bilateral agreements
    /// * `expires_at` - Optional expiration timestamp
    /// * `dispute_window` - Seconds after execution for dispute filing
    #[when_not_paused]
    pub fn create_agreement(
        e: &Env,
        initiator: Address,
        counterparty: Option<Address>,
        expires_at: Option<u64>,
        dispute_window: u32,
    ) -> BytesN<32> {
        initiator.require_auth();

        // Validate expiration is in the future
        let now = e.ledger().timestamp();
        if let Some(exp) = expires_at {
            if exp <= now {
                panic_with_error!(e, OrchestratedAgreementError::InvalidTimestamp);
            }
        }

        // Validate dispute window
        if dispute_window == 0 {
            panic_with_error!(e, OrchestratedAgreementError::InvalidTimestamp);
        }

        // Generate unique agreement ID
        let count: u64 = e.storage().instance().get(&DataKey::AgreementCount).unwrap_or(0);
        let id = e.prng().gen::<BytesN<32>>();

        // Calculate dispute window end if expiration is set
        let dispute_window_end = expires_at.map(|exp| exp + dispute_window as u64);

        let agreement = OrchestratedAgreement {
            id: id.clone(),
            state: AgreementState::Draft,
            initiator: initiator.clone(),
            counterparty: counterparty.clone(),
            created_at: now,
            updated_at: now,
            expires_at,
            dispute_window_end,
            identity: None,
            wallet: None,
            asset: None,
            integration: None,
            reporting: None,
            metadata: Map::new(e),
            version: 1,
        };

        // Store agreement
        let key = DataKey::Agreement(id.clone());
        e.storage().persistent().set(&key, &agreement);
        e.storage().instance().set(&DataKey::AgreementCount, &(count + 1));

        // Index by initiator
        let initiator_key = DataKey::InitiatorAgreements(initiator.clone());
        let mut agreements: Vec<BytesN<32>> = e.storage().persistent().get(&initiator_key).unwrap_or(Vec::new(e));
        agreements.push_back(id.clone());
        e.storage().persistent().set(&initiator_key, &agreements);

        // Emit event
        emit_agreement_created(e, &id, &initiator, &counterparty, now);

        id
    }

    /// Layer 1: Identity - Attest DID and VC
    /// 
    /// # Arguments
    /// * `agreement_id` - The agreement to update
    /// * `did` - Decentralized Identifier string
    /// * `did_method` - Method used (Web, Key, Ethr, Ebsi)
    /// * `vc_type` - Type of verifiable credential
    /// * `attestation_hash` - Hash of the attestation record
    /// * `verifier` - Optional on-chain verifier address
    pub fn attest_identity(
        e: &Env,
        agreement_id: BytesN<32>,
        did: String,
        did_method: DIDMethod,
        vc_type: Symbol,
        attestation_hash: BytesN<32>,
        verifier: Option<Address>,
    ) {
        let mut agreement = Self::get_agreement(e, agreement_id.clone());
        
        // Require auth from the initiator
        agreement.initiator.require_auth();

        // Check pause state
        Self::require_not_paused(e);

        // Validate agreement state
        if agreement.state != AgreementState::Draft && agreement.state != AgreementState::Active {
            panic_with_error!(e, OrchestratedAgreementError::InvalidStateTransition);
        }

        // Check expiration
        Self::require_not_expired(e, &agreement);

        // Validate DID length
        let did_len = did.len();
        if did_len == 0 || did_len > 200 {
            panic_with_error!(e, OrchestratedAgreementError::InvalidDID);
        }

        // Verify attestation hash is non-zero
        let zero_hash = BytesN::from_array(e, &[0; 32]);
        if attestation_hash == zero_hash {
            panic_with_error!(e, OrchestratedAgreementError::InvalidHash);
        }

        let old_state = agreement.state.clone();

        let identity = IdentityLayer {
            did,
            did_method: did_method.clone(),
            vc_issued: true,
            vc_type,
            attested_at: e.ledger().timestamp(),
            attestation_hash,
            verifier_address: verifier,
        };

        agreement.identity = Some(identity);
        agreement.state = AgreementState::Active;
        agreement.updated_at = e.ledger().timestamp();

        Self::update_agreement(e, agreement_id.clone(), &agreement);

        emit_state_transition(e, &agreement_id, &old_state, &agreement.state);
        emit_identity_attested(e, &agreement_id, &did_method, e.ledger().timestamp());
    }

    /// Layer 2: Wallet - Connect and verify wallet
    /// 
    /// # Arguments
    /// * `agreement_id` - The agreement to update
    /// * `stellar_address` - Primary Stellar address
    /// * `wallet_type` - Type of wallet (wwwallet/external/testnet)
    /// * `passkey_enabled` - Whether WebAuthn/FIDO2 is enabled
    /// * `required_signers` - Multi-sig threshold (1 for single-sig)
    pub fn connect_wallet(
        e: &Env,
        agreement_id: BytesN<32>,
        stellar_address: Address,
        wallet_type: Symbol,
        passkey_enabled: bool,
        required_signers: u32,
    ) {
        let mut agreement = Self::get_agreement(e, agreement_id.clone());
        agreement.initiator.require_auth();

        Self::require_not_paused(e);
        Self::require_not_expired(e, &agreement);

        // Validate state
        if agreement.state == AgreementState::Executed 
            || agreement.state == AgreementState::Settled 
            || agreement.state == AgreementState::Cancelled {
            panic_with_error!(e, OrchestratedAgreementError::InvalidStateTransition);
        }

        // Validate required signers
        if required_signers == 0 {
            panic_with_error!(e, OrchestratedAgreementError::ZeroValue);
        }

        let wallet = WalletLayer {
            stellar_address: stellar_address.clone(),
            secondary_addresses: Vec::new(e),
            passkey_enabled,
            wallet_type: wallet_type.clone(),
            funded: false,
            required_signers,
        };

        agreement.wallet = Some(wallet);
        agreement.updated_at = e.ledger().timestamp();

        Self::update_agreement(e, agreement_id.clone(), &agreement);

        emit_wallet_connected(e, &agreement_id, &stellar_address, &wallet_type);
    }

    /// Add secondary signer to wallet
    pub fn add_secondary_signer(
        e: &Env,
        agreement_id: BytesN<32>,
        signer: Address,
    ) {
        let mut agreement = Self::get_agreement(e, agreement_id.clone());
        agreement.initiator.require_auth();

        Self::require_not_paused(e);

        if let Some(mut wallet) = agreement.wallet.clone() {
            // Check for duplicates
            for existing in wallet.secondary_addresses.iter() {
                if existing == signer {
                    panic_with_error!(e, OrchestratedAgreementError::Unauthorized);
                }
            }
            wallet.secondary_addresses.push_back(signer);
            agreement.wallet = Some(wallet);
            agreement.updated_at = e.ledger().timestamp();
            Self::update_agreement(e, agreement_id.clone(), &agreement);
        } else {
            panic_with_error!(e, OrchestratedAgreementError::WalletRequired);
        }
    }

    /// Layer 3: Fund wallet / Asset setup
    /// 
    /// # Arguments
    /// * `agreement_id` - The agreement to update
    /// * `asset_code` - Asset identifier (USDC, XLM, etc.)
    /// * `amount` - Total amount
    /// * `locked_amount` - Amount to lock as collateral
    /// * `asset_type` - fungible or non-fungible
    /// * `contract_id` - SAC contract address for tokens
    pub fn fund_and_set_asset(
        e: &Env,
        agreement_id: BytesN<32>,
        asset_code: Symbol,
        amount: i128,
        locked_amount: i128,
        asset_type: Symbol,
        contract_id: Option<BytesN<32>>,
    ) {
        let mut agreement = Self::get_agreement(e, agreement_id.clone());
        agreement.initiator.require_auth();

        Self::require_not_paused(e);
        Self::require_not_expired(e, &agreement);

        // Validate amounts
        if amount < 0 {
            panic_with_error!(e, OrchestratedAgreementError::InvalidAmount);
        }
        if locked_amount < 0 {
            panic_with_error!(e, OrchestratedAgreementError::InvalidAmount);
        }
        if locked_amount > amount {
            panic_with_error!(e, OrchestratedAgreementError::InvalidAmount);
        }

        // Validate state
        if agreement.state != AgreementState::Active {
            panic_with_error!(e, OrchestratedAgreementError::InvalidStateTransition);
        }

        let old_state = agreement.state.clone();

        let asset = AssetLayer {
            asset_code: asset_code.clone(),
            contract_id,
            amount,
            locked_amount,
            asset_type,
            sep41_compliant: true,
            escrow_released: false,
        };

        // Mark wallet as funded
        if let Some(mut wallet) = agreement.wallet.clone() {
            wallet.funded = true;
            agreement.wallet = Some(wallet);
        }

        agreement.asset = Some(asset);
        agreement.state = AgreementState::Funded;
        agreement.updated_at = e.ledger().timestamp();

        Self::update_agreement(e, agreement_id.clone(), &agreement);

        emit_state_transition(e, &agreement_id, &old_state, &agreement.state);
        emit_asset_funded(e, &agreement_id, &asset_code, amount, locked_amount);
    }

    /// Release escrow (after successful execution)
    pub fn release_escrow(e: &Env, agreement_id: BytesN<32>) {
        let mut agreement = Self::get_agreement(e, agreement_id.clone());
        agreement.initiator.require_auth();

        if agreement.state != AgreementState::Settled {
            panic_with_error!(e, OrchestratedAgreementError::InvalidStateTransition);
        }

        if let Some(mut asset) = agreement.asset.clone() {
            asset.escrow_released = true;
            agreement.asset = Some(asset);
            agreement.updated_at = e.ledger().timestamp();
            Self::update_agreement(e, agreement_id.clone(), &agreement);
        }
    }

    /// Layer 4: Integrations - Add OA score, carbon credits, etc.
    /// 
    /// # Arguments
    /// * `agreement_id` - The agreement to update
    /// * `oa_score` - Optional OA score (0-1000)
    /// * `carbon_credits` - Tonnes of CO2 to offset
    /// * `x402_enabled` - Enable HTTP 402 payments
    /// * `x402_endpoint` - Payment endpoint URL
    /// * `tempo_fx_rate` - Optional FX rate
    /// * `tempo_quote_id` - Optional Tempo quote ID
    /// * `tempo_expires_at` - Quote expiration timestamp
    pub fn add_integrations(
        e: &Env,
        agreement_id: BytesN<32>,
        oa_score: Option<i32>,
        carbon_credits: i128,
        x402_enabled: bool,
        x402_endpoint: Option<String>,
        tempo_fx_rate: Option<i128>,
        tempo_quote_id: Option<BytesN<32>>,
        tempo_expires_at: Option<u64>,
    ) {
        let mut agreement = Self::get_agreement(e, agreement_id.clone());
        agreement.initiator.require_auth();

        Self::require_not_paused(e);
        Self::require_not_expired(e, &agreement);

        // Validate OA score bounds
        if let Some(score) = oa_score {
            if score < 0 || score > 1000 {
                panic_with_error!(e, OrchestratedAgreementError::InvalidScore);
            }
        }

        // Validate carbon credits non-negative
        if carbon_credits < 0 {
            panic_with_error!(e, OrchestratedAgreementError::CarbonCreditsNegative);
        }

        // Validate Tempo expiration is in the future if provided
        if let Some(exp) = tempo_expires_at {
            if exp <= e.ledger().timestamp() {
                panic_with_error!(e, OrchestratedAgreementError::InvalidTimestamp);
            }
        }

        // Determine grade from score
        let oa_grade = Self::score_to_grade(e, oa_score);

        let integration = IntegrationLayer {
            oa_score,
            oa_grade,
            carbon_credits,
            carbon_retired: false,
            x402_enabled,
            x402_endpoint,
            tempo_fx_rate,
            tempo_quote_id,
            tempo_expires_at,
        };

        agreement.integration = Some(integration);
        agreement.updated_at = e.ledger().timestamp();

        Self::update_agreement(e, agreement_id.clone(), &agreement);

        emit_integration_added(
            e,
            &agreement_id,
            oa_score.unwrap_or(0),
            carbon_credits,
        );
    }

    /// Retire carbon credits (separate action)
    pub fn retire_carbon(e: &Env, agreement_id: BytesN<32>) {
        let mut agreement = Self::get_agreement(e, agreement_id.clone());
        agreement.initiator.require_auth();

        Self::require_not_paused(e);

        if let Some(mut integration) = agreement.integration.clone() {
            if integration.carbon_retired {
                panic_with_error!(e, OrchestratedAgreementError::InvalidStateTransition);
            }
            integration.carbon_retired = true;
            agreement.integration = Some(integration);
            agreement.updated_at = e.ledger().timestamp();
            Self::update_agreement(e, agreement_id.clone(), &agreement);
        } else {
            panic_with_error!(e, OrchestratedAgreementError::LayerNotComplete);
        }

        emit_carbon_retired(e, &agreement_id, e.ledger().timestamp());
    }

    /// Layer 5: Execute the agreement (complete flow)
    /// 
    /// # Arguments
    /// * `agreement_id` - The agreement to execute
    /// * `final_tx_hash` - Hash of the final settlement transaction
    pub fn execute_agreement(
        e: &Env,
        agreement_id: BytesN<32>,
        final_tx_hash: BytesN<32>,
    ) {
        let mut agreement = Self::get_agreement(e, agreement_id.clone());
        agreement.initiator.require_auth();

        Self::require_not_paused(e);
        Self::require_not_expired(e, &agreement);

        // Validate all layers are complete
        if agreement.identity.is_none() {
            panic_with_error!(e, OrchestratedAgreementError::IdentityRequired);
        }
        if agreement.wallet.is_none() {
            panic_with_error!(e, OrchestratedAgreementError::WalletRequired);
        }
        if agreement.asset.is_none() {
            panic_with_error!(e, OrchestratedAgreementError::AssetRequired);
        }

        // Validate state
        if agreement.state != AgreementState::Funded {
            panic_with_error!(e, OrchestratedAgreementError::InvalidStateTransition);
        }

        let old_state = agreement.state.clone();
        let timestamp = e.ledger().timestamp();

        // Generate audit hash
        let mut audit_data = Bytes::from_array(e, &agreement_id.to_array());
        let ts_bytes = timestamp.to_be_bytes();
        for i in 0..8u32 {
            audit_data.push_back(ts_bytes[i as usize]);
        }
        let tx_array = final_tx_hash.to_array();
        for i in 0..32u32 {
            audit_data.push_back(tx_array[i as usize]);
        }
        let audit_hash = e.crypto().sha256(&audit_data);

        let reporting = ReportingLayer {
            audit_hash: audit_hash.into(),
            iso20022_ref: None,
            tx_hash: Some(final_tx_hash.clone()),
            report_generated: true,
            dispute_resolution_hash: None,
        };

        agreement.reporting = Some(reporting);
        agreement.state = AgreementState::Executed;
        agreement.updated_at = timestamp;

        Self::update_agreement(e, agreement_id.clone(), &agreement);

        emit_state_transition(e, &agreement_id, &old_state, &agreement.state);
        emit_agreement_executed(e, &agreement_id, &final_tx_hash, timestamp);
    }

    /// Finalize/Settle the agreement
    pub fn settle_agreement(e: &Env, agreement_id: BytesN<32>) {
        let mut agreement = Self::get_agreement(e, agreement_id.clone());
        agreement.initiator.require_auth();

        Self::require_not_paused(e);

        if agreement.state != AgreementState::Executed {
            panic_with_error!(e, OrchestratedAgreementError::InvalidStateTransition);
        }

        // Check dispute window has passed
        if let Some(dispute_end) = agreement.dispute_window_end {
            if e.ledger().timestamp() < dispute_end {
                panic_with_error!(e, OrchestratedAgreementError::DisputeActive);
            }
        }

        let old_state = agreement.state.clone();
        agreement.state = AgreementState::Settled;
        agreement.updated_at = e.ledger().timestamp();

        Self::update_agreement(e, agreement_id.clone(), &agreement);

        emit_state_transition(e, &agreement_id, &old_state, &agreement.state);
        emit_agreement_settled(e, &agreement_id, e.ledger().timestamp());
    }

    /// Cancel agreement (only in Draft or Active state, initiator only)
    pub fn cancel_agreement(e: &Env, agreement_id: BytesN<32>) {
        let mut agreement = Self::get_agreement(e, agreement_id.clone());
        agreement.initiator.require_auth();

        Self::require_not_paused(e);

        // Can only cancel in early states
        if agreement.state != AgreementState::Draft 
            && agreement.state != AgreementState::Active {
            panic_with_error!(e, OrchestratedAgreementError::InvalidStateTransition);
        }

        let old_state = agreement.state.clone();
        agreement.state = AgreementState::Cancelled;
        agreement.updated_at = e.ledger().timestamp();

        Self::update_agreement(e, agreement_id.clone(), &agreement);

        emit_state_transition(e, &agreement_id, &old_state, &agreement.state);
        emit_agreement_cancelled(e, &agreement_id, e.ledger().timestamp());
    }

    /// Raise a dispute (only in Executed state, during dispute window)
    pub fn raise_dispute(
        e: &Env,
        agreement_id: BytesN<32>,
        dispute_hash: BytesN<32>,
    ) {
        let mut agreement = Self::get_agreement(e, agreement_id.clone());
        
        // Either party can dispute
        let _caller = e.current_contract_address(); // Will be replaced with actual caller logic
        
        // For now, only initiator can dispute
        agreement.initiator.require_auth();

        Self::require_not_paused(e);

        if agreement.state != AgreementState::Executed {
            panic_with_error!(e, OrchestratedAgreementError::InvalidStateTransition);
        }

        // Check still within dispute window
        if let Some(dispute_end) = agreement.dispute_window_end {
            if e.ledger().timestamp() > dispute_end {
                panic_with_error!(e, OrchestratedAgreementError::AgreementExpired);
            }
        }

        let old_state = agreement.state.clone();
        agreement.state = AgreementState::Disputed;
        agreement.updated_at = e.ledger().timestamp();

        // Update reporting with dispute info
        if let Some(mut reporting) = agreement.reporting.clone() {
            reporting.dispute_resolution_hash = Some(dispute_hash.clone());
            agreement.reporting = Some(reporting);
        }

        Self::update_agreement(e, agreement_id.clone(), &agreement);

        emit_state_transition(e, &agreement_id, &old_state, &agreement.state);
        emit_agreement_disputed(e, &agreement_id, &dispute_hash, e.ledger().timestamp());
    }

    /// Resolve a dispute (owner only, for now)
    #[only_owner]
    pub fn resolve_dispute(
        e: &Env,
        agreement_id: BytesN<32>,
        resolution_hash: BytesN<32>,
        settle: bool, // If true, settle; if false, return to Executed
    ) {
        Self::require_not_paused(e);

        let mut agreement = Self::get_agreement(e, agreement_id.clone());

        if agreement.state != AgreementState::Disputed {
            panic_with_error!(e, OrchestratedAgreementError::InvalidStateTransition);
        }

        let old_state = agreement.state.clone();

        // Update reporting with resolution
        if let Some(mut reporting) = agreement.reporting.clone() {
            reporting.dispute_resolution_hash = Some(resolution_hash);
            agreement.reporting = Some(reporting);
        }

        if settle {
            agreement.state = AgreementState::Settled;
            emit_agreement_settled(e, &agreement_id, e.ledger().timestamp());
        } else {
            agreement.state = AgreementState::Executed;
        }

        agreement.updated_at = e.ledger().timestamp();

        Self::update_agreement(e, agreement_id.clone(), &agreement);
        emit_state_transition(e, &agreement_id, &old_state, &agreement.state);
    }

    // ─── Query Functions ──────────────────────

    /// Get full agreement details
    pub fn get_agreement(e: &Env, agreement_id: BytesN<32>) -> OrchestratedAgreement {
        let key = DataKey::Agreement(agreement_id);
        e.storage().persistent().get(&key)
            .expect("Agreement not found")
    }

    /// Check if agreement exists
    pub fn agreement_exists(e: &Env, agreement_id: BytesN<32>) -> bool {
        let key = DataKey::Agreement(agreement_id);
        e.storage().persistent().has(&key)
    }

    /// Get agreement state
    pub fn get_agreement_state(e: &Env, agreement_id: BytesN<32>) -> AgreementState {
        let agreement = Self::get_agreement(e, agreement_id);
        agreement.state
    }

    /// Get layer completion status
    pub fn get_layer_status(e: &Env, agreement_id: BytesN<32>) -> Map<Symbol, bool> {
        let agreement = Self::get_agreement(e, agreement_id);
        let mut status = Map::new(e);
        
        status.set(Symbol::new(e, "identity"), agreement.identity.is_some());
        status.set(Symbol::new(e, "wallet"), agreement.wallet.is_some());
        status.set(Symbol::new(e, "asset"), agreement.asset.is_some());
        status.set(Symbol::new(e, "integration"), agreement.integration.is_some());
        status.set(Symbol::new(e, "reporting"), agreement.reporting.is_some());
        
        status
    }

    /// Get total agreement count
    pub fn get_agreement_count(e: &Env) -> u64 {
        e.storage().instance().get(&DataKey::AgreementCount).unwrap_or(0)
    }

    /// Get agreements by initiator
    pub fn get_initiator_agreements(e: &Env, initiator: Address) -> Vec<BytesN<32>> {
        let key = DataKey::InitiatorAgreements(initiator);
        e.storage().persistent().get(&key).unwrap_or(Vec::new(e))
    }

    /// Get agreement summary for UI display
    pub fn get_agreement_summary(
        e: &Env,
        agreement_id: BytesN<32>,
    ) -> Map<Symbol, Symbol> {
        let agreement = Self::get_agreement(e, agreement_id);
        let mut summary = Map::new(e);

        // Map state to string representation
        let state_str = match agreement.state {
            AgreementState::Draft => Symbol::new(e, "Draft"),
            AgreementState::Active => Symbol::new(e, "Active"),
            AgreementState::Funded => Symbol::new(e, "Funded"),
            AgreementState::Executed => Symbol::new(e, "Executed"),
            AgreementState::Settled => Symbol::new(e, "Settled"),
            AgreementState::Disputed => Symbol::new(e, "Disputed"),
            AgreementState::Expired => Symbol::new(e, "Expired"),
            AgreementState::Cancelled => Symbol::new(e, "Cancelled"),
        };

        summary.set(Symbol::new(e, "state"), state_str);
        summary.set(Symbol::new(e, "initiator"), Symbol::new(e, "set"));
        
        if agreement.counterparty.is_some() {
            summary.set(Symbol::new(e, "counterparty"), Symbol::new(e, "set"));
        } else {
            summary.set(Symbol::new(e, "counterparty"), Symbol::new(e, "none"));
        }

        summary
    }

    /// Check if agreement is expired
    pub fn is_expired(e: &Env, agreement_id: BytesN<32>) -> bool {
        let agreement = Self::get_agreement(e, agreement_id);
        
        if let Some(expires) = agreement.expires_at {
            e.ledger().timestamp() > expires
        } else {
            false
        }
    }

    /// Get time remaining until expiration (0 if expired)
    pub fn time_until_expiry(e: &Env, agreement_id: BytesN<32>) -> u64 {
        let agreement = Self::get_agreement(e, agreement_id);
        
        if let Some(expires) = agreement.expires_at {
            let now = e.ledger().timestamp();
            if now >= expires {
                0
            } else {
                expires - now
            }
        } else {
            u64::MAX // No expiration
        }
    }

    // ─── Metadata Functions ─────────────────────

    /// Add metadata to agreement
    pub fn set_metadata(
        e: &Env,
        agreement_id: BytesN<32>,
        key: Symbol,
        value: String,
    ) {
        let mut agreement = Self::get_agreement(e, agreement_id.clone());
        agreement.initiator.require_auth();

        // Only allow metadata updates in certain states
        if agreement.state == AgreementState::Settled 
            || agreement.state == AgreementState::Cancelled {
            panic_with_error!(e, OrchestratedAgreementError::InvalidStateTransition);
        }

        agreement.metadata.set(key, value);
        agreement.updated_at = e.ledger().timestamp();

        Self::update_agreement(e, agreement_id, &agreement);
    }

    /// Get metadata value
    pub fn get_metadata(
        e: &Env,
        agreement_id: BytesN<32>,
        key: Symbol,
    ) -> Option<String> {
        let agreement = Self::get_agreement(e, agreement_id);
        agreement.metadata.get(key)
    }

    // ─── Private Helpers ────────────────────────

    fn update_agreement(
        e: &Env,
        agreement_id: BytesN<32>,
        agreement: &OrchestratedAgreement,
    ) {
        let key = DataKey::Agreement(agreement_id);
        e.storage().persistent().set(&key, agreement);
    }

    fn require_not_paused(e: &Env) {
        if pausable::paused(e) {
            panic_with_error!(e, OrchestratedAgreementError::ContractPaused);
        }
    }

    fn require_not_expired(e: &Env, agreement: &OrchestratedAgreement) {
        if let Some(expires) = agreement.expires_at {
            if e.ledger().timestamp() > expires {
                panic_with_error!(e, OrchestratedAgreementError::AgreementExpired);
            }
        }
    }

    fn score_to_grade(e: &Env, score: Option<i32>) -> Symbol {
        match score {
            None => Symbol::new(e, "N/A"),
            Some(s) => {
                if s >= 950 { Symbol::new(e, "AAA+") }
                else if s >= 900 { Symbol::new(e, "AAA") }
                else if s >= 850 { Symbol::new(e, "AA+") }
                else if s >= 800 { Symbol::new(e, "AA") }
                else if s >= 750 { Symbol::new(e, "A+") }
                else if s >= 700 { Symbol::new(e, "A") }
                else if s >= 650 { Symbol::new(e, "BBB+") }
                else if s >= 600 { Symbol::new(e, "BBB") }
                else if s >= 550 { Symbol::new(e, "BB+") }
                else if s >= 500 { Symbol::new(e, "BB") }
                else if s >= 450 { Symbol::new(e, "B+") }
                else if s >= 400 { Symbol::new(e, "B") }
                else if s >= 300 { Symbol::new(e, "CCC") }
                else if s >= 200 { Symbol::new(e, "CC") }
                else if s >= 100 { Symbol::new(e, "C") }
                else { Symbol::new(e, "D") }
            }
        }
    }
}
