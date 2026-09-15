# Tansu Integration

Tansu is a decentralized project governance and versioning layer for open source projects, built on Stellar/Soroban. It provides on-chain project registration, commit hash tracking, DAO governance with public/anonymous voting, badge-based membership, and supply-chain evidence (SBOM/CVE/Attestation) — all backend-less, interacting directly with Stellar RPC.

- **Website**: https://tansu.dev
- **Docs**: https://tansu.dev/docs/intro
- **GitHub**: https://github.com/Consulting-Manao/tansu
- **Radicle**: https://radicle.network/nodes/radicle.consulting-manao.com/rad:zssaAF91kxuquZmZCV2SiK2FNX6s

---

## Contract IDs

| Network  | Contract ID                                                          | dApp                       |
|----------|----------------------------------------------------------------------|----------------------------|
| Testnet  | `CBXKUSLQPVF35FYURR5C42BPYA5UOVDXX2ELKIM2CAJMCI6HXG2BHGZA`          | https://testnet.tansu.dev  |
| Mainnet  | `CDXINK2T3P46M4LWK35FVIXXHJ2XHAS4FOVCGVPJ63YV5OVTM24IY5BI`          | https://app.tansu.dev      |

---

## Architecture Overview

Tansu is a single Soroban contract implementing four traits:

| Trait              | File                        | Responsibility                                                        |
|--------------------|-----------------------------|-----------------------------------------------------------------------|
| `TansuTrait`       | `contract_tansu.rs`         | Admin config, pause/unpause, multi-admin upgrade flow                  |
| `VersioningTrait`  | `contract_versioning.rs`    | Project registration, commit tracking, evidence, attestations          |
| `MembershipTrait`  | `contract_membership.rs`   | Member registration, badges, voting weight calculation                |
| `DaoTrait`         | `contract_dao.rs`           | Proposals, public/anonymous voting, execution, outcome contracts      |

### Key Properties

- **Backend-less**: No REST API. All reads/writes go through Soroban RPC directly.
- **IPFS for off-chain content**: Proposal descriptions, member profiles, project metadata (`tansu.toml`), and evidence artifacts are stored on IPFS; only CIDs are stored on-chain.
- **Collateral-based spam prevention**: 5 XLM for project registration, 5 XLM for proposal creation (refunded on execute, forfeited on revoke).
- **Supermajority governance**: `approve > (abstain + reject)` to pass; abstain votes count against both sides.
- **Anonymous voting**: BLS12-381 Pedersen commitment scheme; tallies revealed at execution time.
- **Event-driven**: All state changes emit events (no indexer needed for basic reads; full evidence history recoverable from `EvidenceSet` events).

### Key Constants

| Constant                          | Value                        |
|-----------------------------------|------------------------------|
| `REGISTER_COLLATERAL`             | 5 XLM (50,000,000 stroops)   |
| `PROPOSAL_COLLATERAL`             | 5 XLM (50,000,000 stroops)   |
| `MIN_VOTING_PERIOD`               | 24 hours                     |
| `MAX_VOTING_PERIOD`               | 30 days                     |
| `TIMELOCK_DELAY`                  | 24 hours                     |
| `DEFAULT_FINALITY_THRESHOLD`      | 66%                          |
| `MIN_FINALITY_THRESHOLD`          | 50%                          |
| `ATTESTATION_REVOCATION_WINDOW`   | 24 hours                     |
| `MAX_PROJECTS_PER_PAGE`           | 10                           |
| `MAX_PROPOSALS_PER_PAGE`          | 9                            |
| `MAX_PAGES`                       | 1000 (9,000 proposals max)  |
| `MAX_VOTES_PER_PROPOSAL`          | 40                           |
| `MAX_ATTESTATIONS`                | 25                           |
| `MAX_MAINTAINERS`                 | 25                           |
| `MAX_EVIDENCE`                    | 10 per (project, commit, kind) |

### Badge Weights

| Badge      | Weight    |
|------------|-----------|
| Developer  | 10,000,000 |
| Triage     | 5,000,000  |
| Community  | 1,000,000  |
| Verified   | 500,000    |
| Default    | 1          |

Multiple badges sum for total voting weight. Three voting weight modes exist:
- **Badge-based** (default): `get_max_weight` returns badge sum
- **Token-based**: `create_proposal` with `token_contract`, weight capped by token balance
- **NQG**: Admin-configured for SCF Public Goods project, uses Neural Quorum Governance scores

---

## Contract Function Map

### Read-Only Functions (Soroban `SimulateOnly`)

These functions require no authentication and no transaction submission. MozartPay calls them via `internal/soroban/` with a dummy keypair.

#### VersioningTrait

| Function                    | Parameters                                                      | Returns                    |
|-----------------------------|-----------------------------------------------------------------|----------------------------|
| `get_projects`              | `page: u32`                                                    | `Vec<Project>`             |
| `get_project`               | `project_key: Bytes`                                           | `Project`                  |
| `get_commit`                | `project_key: Bytes`                                           | `String`                   |
| `get_evidence`              | `project_key: Bytes, commit_hash: String, kind: EvidenceKind`  | `Vec<Evidence>`            |
| `get_sub_projects`          | `project_key: Bytes`                                           | `Vec<Bytes>`               |
| `get_attestation_threshold` | `project_key: Bytes`                                           | `u32`                      |
| `get_attestation_finality`  | `project_key: Bytes, commit_hash: String, target: AttestationTarget` | `FinalityStatus`   |
| `get_attestations`          | `project_key: Bytes, commit_hash: String, target: AttestationTarget` | `Vec<Attestation>` |

#### MembershipTrait

| Function          | Parameters                                  | Returns  |
|-------------------|---------------------------------------------|----------|
| `get_member`      | `member_address: Address`                   | `Member` |
| `get_badges`      | `key: Bytes`                                | `Badges` |
| `get_max_weight`  | `project_key: Bytes, member_address: Address` | `u32`  |

#### DaoTrait

| Function                     | Parameters                                  | Returns                  |
|------------------------------|---------------------------------------------|--------------------------|
| `get_dao`                    | `project_key: Bytes, page: u32`            | `Dao`                    |
| `get_proposal`               | `project_key: Bytes, proposal_id: u32`     | `Proposal`               |
| `get_conflict_of_interest`   | `project_key: Bytes, proposal_id: u32`     | `Vec<Address>`           |
| `get_anonymous_voting_config`| `project_key: Bytes`                       | `AnonymousVoteConfig`    |

#### TansuTrait

| Function            | Parameters | Returns        |
|---------------------|------------|----------------|
| `get_admins_config` | none       | `AdminsConfig` |

### Write Functions (Soroban `Invoke`)

These functions require wallet signing and transaction submission. MozartPay uses the active wallet's private key via `internal/soroban/` `Invoke()`.

#### VersioningTrait

| Function                   | Parameters                                                                                                         | Auth         | Collateral | Returns  |
|----------------------------|--------------------------------------------------------------------------------------------------------------------|--------------|------------|----------|
| `register`                 | `maintainer: Address, name: String, maintainers: Vec<Address>, url: String, ipfs: String, min_voting_period: Option<u64>, execute_delay: Option<u64>, attestation_threshold: Option<u32>` | Maintainer | 5 XLM     | `Bytes` (project key) |
| `update_config`            | `maintainer: Address, key: Bytes, maintainers: Vec<Address>, url: String, ipfs: String, min_voting_period: Option<u64>, execute_delay: Option<u64>, attestation_threshold: Option<u32>` | Maintainer | —          | —        |
| `commit`                   | `maintainer: Address, project_key: Bytes, hash: String`                                                          | Maintainer   | —          | —        |
| `set_evidence`             | `maintainer: Address, project_key: Bytes, commit_hash: String, kind: EvidenceKind, cid: String`                 | Maintainer   | —          | —        |
| `set_sub_projects`         | `maintainer: Address, project_key: Bytes, sub_projects: Vec<Bytes>`                                              | Maintainer   | —          | —        |
| `set_attestation_threshold`| `maintainer: Address, project_key: Bytes, attestation_threshold: Option<u32>`                                    | Maintainer   | —          | —        |
| `attest`                   | `attester: Address, project_key: Bytes, commit_hash: String, target: AttestationTarget, note: Option<String>`   | Maintainer   | —          | —        |
| `revoke_attestation`      | `attester: Address, project_key: Bytes, commit_hash: String, target: AttestationTarget`                        | Maintainer   | —          | —        |

#### MembershipTrait

| Function         | Parameters                                                                                                         | Auth         |
|------------------|--------------------------------------------------------------------------------------------------------------------|--------------|
| `add_member`     | `member_address: Address, meta: String, git_identity: Option<String>, git_pubkey: Option<BytesN<32>>, git_sig: Option<BytesN<64>>` | Member      |
| `update_member`  | `member_address: Address, meta: String, git_identity: Option<String>, git_pubkey: Option<BytesN<32>>, git_sig: Option<BytesN<64>>` | Member      |
| `set_badges`     | `maintainer: Address, key: Bytes, member: Address, badges: Vec<Badge>`                                            | Maintainer   |

#### DaoTrait

| Function                     | Parameters                                                                                                         | Auth         | Collateral | Returns            |
|------------------------------|--------------------------------------------------------------------------------------------------------------------|--------------|------------|--------------------|
| `create_proposal`            | `proposer: Address, project_key: Bytes, title: String, ipfs: String, voting_ends_at: u64, public_voting: bool, token_contract: Option<Address>, outcome_contracts: Option<Vec<OutcomeContract>>` | Proposer | 5 XLM | `u32` (proposal ID) |
| `vote`                       | `voter: Address, project_key: Bytes, proposal_id: u32, vote: Vote`                                              | Voter        | —          | —                  |
| `execute`                    | `maintainer: Address, project_key: Bytes, proposal_id: u32, tallies: Option<Vec<u128>>, seeds: Option<Vec<u128>>` | Maintainer | —          | `ProposalStatus`   |
| `revoke_proposal`            | `maintainer: Address, project_key: Bytes, proposal_id: u32`                                                     | Maintainer/Admin | —     | —                  |
| `remove_vote`               | `maintainer: Address, project_key: Bytes, proposal_id: u32, voter: Address`                                     | Maintainer   | —          | —                  |
| `add_conflict_of_interest`  | `maintainer: Address, project_key: Bytes, proposal_id: u32, addresses: Vec<Address>`                            | Maintainer   | —          | —                  |
| `remove_conflict_of_interest`| `maintainer: Address, project_key: Bytes, proposal_id: u32, addresses: Vec<Address>`                           | Maintainer   | —          | —                  |
| `anonymous_voting_setup`    | `maintainer: Address, project_key: Bytes, public_key: String`                                                   | Maintainer   | —          | —                  |

#### TansuTrait

| Function                  | Parameters                                              | Auth  |
|---------------------------|---------------------------------------------------------|-------|
| `pause`                   | `admin: Address, paused: bool`                         | Admin |
| `set_collateral_contract` | `admin: Address, collateral_contract: ContractRef`     | Admin |
| `set_nqg_contract`        | `admin: Address, nqg_contract: ContractRef, project: String` | Admin |

---

## Data Types

### Core Types

```rust
struct Project {
    name: String,              // max 15 lowercase chars
    config: Config,            // { url: String, ipfs: String }
    maintainers: Vec<Address>,
    sub_projects: Option<Vec<Bytes>>,
}

struct Member {
    projects: Vec<ProjectBadges>,
    meta: String,               // IPFS CID to profile folder
    git_identity: Option<String>,
    git_pubkey: Option<BytesN<32>>,
}

struct ProjectBadges {
    project: Bytes,
    badges: Vec<Badge>,
}

struct Badges {
    developer: Vec<Address>,
    triage: Vec<Address>,
    community: Vec<Address>,
    verified: Vec<Address>,
}

enum Badge {
    Developer = 10_000_000,
    Triage = 5_000_000,
    Community = 1_000_000,
    Verified = 500_000,
    Default = 1,
}
```

### Governance Types

```rust
struct Proposal {
    id: u32,
    title: String,
    proposer: Address,
    ipfs: String,
    vote_data: VoteData,
    status: ProposalStatus,
    outcome_contracts: Option<Vec<OutcomeContract>>,
}

enum ProposalStatus { Active, Approved, Rejected, Cancelled, Malicious }

struct VoteData {
    voting_ends_at: u64,
    public_voting: bool,
    token_contract: Option<Address>,
    votes: Vec<Vote>,
}

enum Vote {
    PublicVote(PublicVote),
    AnonymousVote(AnonymousVote),
}

struct PublicVote {
    address: Address,
    weight: u32,
    vote_choice: VoteChoice,
}

enum VoteChoice { Approve, Reject, Abstain }

struct AnonymousVote {
    address: Address,
    weight: u32,
    encrypted_seeds: Vec<String>,
    encrypted_votes: Vec<String>,
    commitments: Vec<BytesN<96>>,
}

struct OutcomeContract {
    address: Address,
    execute_fn: Symbol,
    args: Vec<Val>,
}

struct Dao {
    proposals: Vec<Proposal>,
}
```

### Evidence & Attestation Types

```rust
enum EvidenceKind { Sbom, Cve, Attestation }

struct Evidence {
    cid: String,
    created_at: u64,
}

enum AttestationTarget {
    Commit,
    Evidence(EvidenceKind, String),
}

struct Attestation {
    attester: Address,
    weight: u32,
    created_at: u64,
    note: Option<String>,
}

struct FinalityStatus {
    attested: u32,
    total: u32,
    is_final: bool,
    finalized_at: Option<u64>,
}
```

### Admin Types

```rust
struct AdminsConfig {
    threshold: u32,
    admins: Vec<Address>,
}

struct ContractRef {
    address: Address,
    wasm_hash: Option<BytesN<32>>,
}
```

---

## Project Key Derivation

Project keys are `keccak256(project_name)` → `Bytes`. The name must be:
- Max 15 characters
- Lowercase letters only
- Unique on-chain

MozartPay must compute this client-side to query specific projects by name.

---

## Planned Integration Phases

### Phase 1: Read-Only Integration (No Tansu coordination needed)

**Goal**: MozartPay CLI can query Tansu contract state via Soroban RPC simulation calls.

**Scope**:
- Add `TansuConfig` to `internal/config/config.go` (contract ID, network, enabled flag)
- Create `internal/integrations/tansu.go` client wrapping `internal/soroban/` `SimulateOnly()` calls
- Add CLI commands under `cmd/mozartpay/commands/tansu.go`
- Expose MCP tools in `internal/mcp/tools.go` for AI assistant access

**CLI Commands**:
```
mozartpay tansu list --page <n>
mozartpay tansu show --key <hex>
mozartpay tansu commit --key <hex>
mozartpay tansu evidence --key <hex> --commit <hash> --kind sbom|cve|attestation
mozartpay tansu proposals --key <hex> --page <n>
mozartpay tansu proposal --key <hex> --id <n>
mozartpay tansu member --address <G...>
mozartpay tansu badges --key <hex>
mozartpay tansu weight --key <hex> --address <G...>
mozartpay tansu finality --key <hex> --commit <hash>
mozartpay tansu attestations --key <hex> --commit <hash>
mozartpay tansu sub-projects --key <hex>
mozartpay tansu threshold --key <hex>
mozartpay tansu coi --key <hex> --proposal <n>
mozartpay tansu admins
```

**MCP Tools**:
- `tansu_list_projects` — List projects (paginated)
- `tansu_get_project` — Get project details
- `tansu_get_proposals` — List proposals for a project
- `tansu_get_proposal` — Get single proposal details
- `tansu_get_member` — Get member info and badges
- `tansu_get_commit` — Get latest commit hash
- `tansu_get_evidence` — Get evidence history
- `tansu_get_attestation_finality` — Check attestation finality status

**Implementation files**:
- `internal/config/config.go` — Add `TansuConfig` struct to `IntegrationConfig`
- `internal/integrations/tansu.go` — Tansu client with `SimulateOnly()` wrappers
- `internal/models/types.go` — Go structs mirroring Tansu Rust types
- `cmd/mozartpay/commands/tansu.go` — CLI command definitions
- `internal/mcp/tools.go` — MCP tool definitions
- `internal/mcp/handlers.go` — MCP tool handlers

**Risk**: Zero. Read-only simulation calls cannot modify contract state.

### Phase 2: Write Operations (Requires Tansu coordination)

**Goal**: MozartPay CLI can register projects, create proposals, vote, and commit hashes on Tansu.

**Scope**:
- Extend `internal/integrations/tansu.go` with `Invoke()` wrappers
- Handle 5 XLM collateral transfers (ensure wallet balance, sign transactions)
- Add CLI commands for write operations

**CLI Commands**:
```
mozartpay tansu register --name <name> --maintainers <addr1,addr2> --url <url> --ipfs <cid>
mozartpay tansu commit-set --key <hex> --hash <sha>
mozartpay tansu set-evidence --key <hex> --commit <hash> --kind sbom --cid <ipfs-cid>
mozartpay tansu create-proposal --key <hex> --title <title> --ipfs <cid> --voting-ends <unix> --public
mozartpay tansu vote --key <hex> --proposal <id> --choice approve|reject|abstain --weight <n>
mozartpay tansu execute --key <hex> --proposal <id>
mozartpay tansu add-member --address <G...> --meta <cid>
mozartpay tansu set-badges --key <hex> --member <addr> --badges Developer,Community
mozartpay tansu attest --key <hex> --commit <hash> --note <cid>
```

**Risk**: Low. Write operations are standard Soroban invocations. Collateral transfers go through the native XLM SAC, so MozartPay just needs sufficient wallet balance and proper signing.

**Coordination needed**:
- Confirm testnet contract ID remains stable
- Understand any planned contract upgrades that might change function signatures
- Align on outcome contract integration (Tansu proposals can invoke external contracts on execution — MozartPay OAs could be outcome contracts)

### Phase 3: Deep Integration (Future)

**Goal**: Tansu governance data enhances MozartPay's Orchestrated Agreements platform.

**Potential integrations**:
- **OA Score enhancement**: Use Tansu project attestation finality and evidence records as trust signals in MozartPay's OA scoring algorithm
- **DID/VC bridge**: Tansu member badges and git identity bindings could become Verifiable Credentials in MozartPay's DID layer
- **Payment integration**: Tansu's donation flow could route through MozartPay's payment rails (x402 micropayments, direct payments) with ISO 20022 compliance reporting
- **Outcome contracts**: MozartPay OAs could be registered as Tansu proposal outcome contracts — when a governance proposal passes, it automatically executes an OA payment
- **Compliance reporting**: Tansu project governance events could be included in MozartPay's ISO 20022 pacs.008 reports for funded projects

**Risk**: Medium. Requires contract-to-contract interaction design and alignment with Tansu's roadmap.

---

## tansu.toml (Project Metadata)

Tansu projects use a TOML file following the SEP-1 (Stellar Info File) specification, stored on IPFS:

```toml
VERSION = "2.0.0"
ACCOUNTS = [ "GA...", "GB...", "GC..." ]

[DOCUMENTATION]
ORG_DBA = "My Awesome Project"
ORG_NAME = "Organization Name"
ORG_URL = "https://www.domain.com"
ORG_LOGO = "https://www.domain.com/awesomelogo.png"
ORG_DESCRIPTION = "Description of project"
ORG_GITHUB = "orgcode"

[[PRINCIPALS]]
github = "Handle maintainer GA..."

[[PRINCIPALS]]
github = "Handle maintainer GB..."

[[PRINCIPALS]]
github = "Handle maintainer GC..."
```

The `ORG_DBA` field allows a display name different from the on-chain name (max 15 lowercase chars). Radicle identities are also supported.

---

## Events

All state changes emit Soroban events. These can be observed directly from chain without an indexer for basic reads.

| Event                     | Trait              | Trigger                              |
|---------------------------|--------------------|--------------------------------------|
| `ProjectRegistered`       | VersioningTrait    | New project created                  |
| `ProjectConfigUpdated`    | VersioningTrait    | Project config/maintainers changed   |
| `Commit`                  | VersioningTrait    | New commit hash set                  |
| `EvidenceSet`             | VersioningTrait    | Evidence CID recorded                |
| `SubProjectsUpdated`      | VersioningTrait    | Sub-projects added/changed           |
| `Attested`                | VersioningTrait    | Attestation recorded                  |
| `AttestationRevoked`      | VersioningTrait    | Attestation revoked                   |
| `MemberAdded`              | MembershipTrait   | New member registered / updated      |
| `BadgesUpdated`           | MembershipTrait   | Badges assigned/updated              |
| `AnonymousVotingSetup`    | DaoTrait           | Anonymous voting configured          |
| `ProposalCreated`         | DaoTrait           | New proposal submitted               |
| `VoteCast`                | DaoTrait           | Vote recorded                         |
| `VoteRemoved`             | DaoTrait           | Vote removed by maintainer           |
| `ProposalExecuted`        | DaoTrait           | Proposal finalized                   |
| `ConflictOfInterestUpdated`| DaoTrait          | COI list modified                     |
| `ContractPaused`          | TansuTrait         | Contract paused/unpaused             |
| `ContractUpdated`         | TansuTrait         | Collateral/NQG contract changed      |
| `UpgradeProposed`         | TansuTrait         | Upgrade proposed                     |
| `UpgradeApproved`         | TansuTrait         | Upgrade approved                     |
| `UpgradeStatus`           | TansuTrait         | Upgrade executed/cancelled           |

---

## References

- [Tansu Documentation](https://tansu.dev/docs/intro)
- [Tansu GitHub](https://github.com/Consulting-Manao/tansu)
- [dApps and Contracts](https://tansu.dev/docs/developers/dapps_and_contracts)
- [Architecture](https://tansu.dev/docs/developers/architecture)
- [On-Chain Guide](https://tansu.dev/docs/developers/on_chain)
- [Membership & Badges](https://tansu.dev/docs/developers/membership)
- [Governance & Proposals](https://tansu.dev/docs/developers/governance)
- [Commit Evidence](https://tansu.dev/docs/developers/evidence)
- [Tansu Info File](https://tansu.dev/docs/developers/project_information_file)
- [User Flows](https://tansu.dev/docs/developers/user_flows)
