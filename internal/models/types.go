package models

import "time"

// ─────────────────────────────────────────────
// DID / Identity
// ─────────────────────────────────────────────

type DIDMethod string

const (
	DIDMethodWeb  DIDMethod = "web"
	DIDMethodKey  DIDMethod = "key"
	DIDMethodEthr DIDMethod = "ethr"
	DIDMethodEBSI DIDMethod = "ebsi"
)

type DIDDocument struct {
	Context            []string          `json:"@context"`
	ID                 string            `json:"id"`
	Method             DIDMethod         `json:"method"`
	VerificationMethod []VerificationKey `json:"verificationMethod"`
	Authentication     []string          `json:"authentication"`
	Created            time.Time         `json:"created"`
}

type VerificationKey struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Controller   string `json:"controller"`
	PublicKeyHex string `json:"publicKeyMultibase"`
}

type VerifiableCredential struct {
	Context           []string               `json:"@context"`
	ID                string                 `json:"id"`
	Type              []string               `json:"type"`
	Issuer            string                 `json:"issuer"`
	IssuanceDate      time.Time              `json:"issuanceDate"`
	ExpirationDate    time.Time              `json:"expirationDate"`
	CredentialSubject map[string]interface{} `json:"credentialSubject"`
	Proof             VCProof                `json:"proof"`
}

type VCProof struct {
	Type               string    `json:"type"`
	Created            time.Time `json:"created"`
	ProofPurpose       string    `json:"proofPurpose"`
	VerificationMethod string    `json:"verificationMethod"`
	JWSSignature       string    `json:"jws"`
}

// ─────────────────────────────────────────────
// Wallet
// ─────────────────────────────────────────────

type WalletType string

const (
	WalletWWWallet WalletType = "wwwallet"
	WalletExternal WalletType = "external"
	WalletTestnet  WalletType = "testnet"
	WalletStellar  WalletType = "stellar"
)

type Network string

const (
	NetworkStellarTestnet Network = "stellar-testnet"
	NetworkStellarMainnet Network = "stellar-mainnet"
	NetworkEVMSepolia     Network = "evm-sepolia"
	NetworkEVMMainnet     Network = "evm-mainnet"
)

type Account struct {
	Address    string     `json:"address"`
	PublicKey  string     `json:"publicKey"`
	PrivateKey string     `json:"privateKey,omitempty"`
	Network    Network    `json:"network"`
	Type       WalletType `json:"type"`
	Balance    string     `json:"balance"`
	DID        string     `json:"did,omitempty"`
	Funded     bool       `json:"funded"`
	CreatedAt  time.Time  `json:"createdAt"`
}

// WalletEntry represents a wallet in the registry (without private key)
type WalletEntry struct {
	Address   string     `json:"address"`
	Name      string     `json:"name,omitempty"`
	Type      WalletType `json:"type"`
	Network   Network    `json:"network"`
	Balance   string     `json:"balance"`
	Funded    bool       `json:"funded"`
	CreatedAt time.Time  `json:"createdAt"`
}

// WalletRegistry tracks all wallets and which one is active
type WalletRegistry struct {
	Wallets      []WalletEntry `json:"wallets"`
	ActiveWallet string        `json:"activeWallet"`
}

type PasskeyCredential struct {
	CredentialID string    `json:"credentialId"`
	PublicKey    string    `json:"publicKey"`
	Algorithm    string    `json:"algorithm"`
	Origin       string    `json:"origin"`
	CreatedAt    time.Time `json:"createdAt"`
}

// WebAuthnChallenge represents a WebAuthn credential creation challenge
type WebAuthnChallenge struct {
	Challenge   string    `json:"challenge"`
	UserID      string    `json:"userId"`
	UserName    string    `json:"userName"`
	DisplayName string    `json:"displayName"`
	RPName      string    `json:"rpName"`
	RPID        string    `json:"rpId"`
	Timestamp   time.Time `json:"timestamp"`
	Signature   string    `json:"signature"`
	CallbackURL string    `json:"callbackUrl"`
}

// ─────────────────────────────────────────────
// Assets — SAC / SEP-41
// ─────────────────────────────────────────────

type AssetType string

const (
	AssetFungible    AssetType = "fungible"
	AssetNonFungible AssetType = "non-fungible"
)

type Asset struct {
	ID           string                 `json:"id"`
	ContractID   string                 `json:"contractId"`
	Type         AssetType              `json:"type"`
	Name         string                 `json:"name"`
	Symbol       string                 `json:"symbol"`
	Decimals     int                    `json:"decimals"`
	TotalSupply  string                 `json:"totalSupply"`
	Issuer       string                 `json:"issuer"`
	Network      Network                `json:"network"`
	Standard     string                 `json:"standard"` // SEP-41
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	CarbonOffset *CarbonCredit          `json:"carbonOffset,omitempty"`
	Score        *OAScore               `json:"score,omitempty"`
	CreatedAt    time.Time              `json:"createdAt"`
	TxHash       string                 `json:"txHash"`
}

// ─────────────────────────────────────────────
// Payments
// ─────────────────────────────────────────────

type PaymentRail string

const (
	RailX402   PaymentRail = "x402"
	RailTempo  PaymentRail = "tempo"
	RailDirect PaymentRail = "direct"
	RailZK     PaymentRail = "zk"
	RailSwap   PaymentRail = "swap"
)

type PaymentStatus string

const (
	PaymentPending   PaymentStatus = "pending"
	PaymentConfirmed PaymentStatus = "confirmed"
	PaymentFailed    PaymentStatus = "failed"
)

type Payment struct {
	ID          string        `json:"id"`
	From        string        `json:"from"`
	To          string        `json:"to"`
	Amount      string        `json:"amount"`
	Asset       string        `json:"asset"`
	Rail        PaymentRail   `json:"rail"`
	Status      PaymentStatus `json:"status"`
	Network     Network       `json:"network"`
	Memo        string        `json:"memo,omitempty"`
	FXRate      string        `json:"fxRate,omitempty"`
	Fee         string        `json:"fee"`
	TxHash      string        `json:"txHash"`
	LedgerSeq   int64         `json:"ledgerSeq,omitempty"`
	CreatedAt   time.Time     `json:"createdAt"`
	ConfirmedAt *time.Time    `json:"confirmedAt,omitempty"`
}

// ─────────────────────────────────────────────
// Integrations
// ─────────────────────────────────────────────

type OAScore struct {
	Provider  string    `json:"provider"` // OA
	Score     int       `json:"score"`    // 0–1000
	Grade     string    `json:"grade"`    // AAA, AA, A, BBB...
	RiskLevel string    `json:"riskLevel"`
	UpdatedAt time.Time `json:"updatedAt"`
	URL       string    `json:"url"` // OA rating page URL
}

type CarbonCredit struct {
	Provider  string     `json:"provider"` // StellarCarbon
	TokenID   string     `json:"tokenId"`
	Amount    float64    `json:"amount"` // tonnes CO2e
	Vintage   int        `json:"vintage"`
	Standard  string     `json:"standard"` // VCS, Gold Standard
	Retired   bool       `json:"retired"`
	RetiredAt *time.Time `json:"retiredAt,omitempty"`
	TxHash    string     `json:"txHash"`
}

type X402Request struct {
	ResourceURL string    `json:"resourceUrl"`
	Price       float64   `json:"price"`
	Asset       string    `json:"asset"`
	Payer       string    `json:"payer"`
	Payee       string    `json:"payee"`
	Nonce       string    `json:"nonce"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

type TempoFXQuote struct {
	SourceCurrency string    `json:"sourceCurrency"`
	TargetCurrency string    `json:"targetCurrency"`
	Rate           float64   `json:"rate"`
	Fee            float64   `json:"fee"`
	EstimatedTime  string    `json:"estimatedTime"`
	QuoteID        string    `json:"quoteId"`
	ValidUntil     time.Time `json:"validUntil"`
}

// ─────────────────────────────────────────────
// Reporting
// ─────────────────────────────────────────────

type TransactionReport struct {
	ReportID     string                `json:"reportId"`
	GeneratedAt  time.Time             `json:"generatedAt"`
	Payment      *Payment              `json:"payment,omitempty"`
	Asset        *Asset                `json:"asset,omitempty"`
	DID          string                `json:"did"`
	VCAttached   bool                  `json:"vcAttached"`
	VC           *VerifiableCredential `json:"vc,omitempty"`
	CarbonOffset *CarbonCredit         `json:"carbonOffset,omitempty"`
	Score        *OAScore              `json:"score,omitempty"`
	ISO20022     *ISO20022Message      `json:"iso20022,omitempty"`
	AuditHash    string                `json:"auditHash"`
}

type ISO20022Message struct {
	MessageID       string              `json:"msgId"`
	CreatedAt       time.Time           `json:"creDtTm"`
	InitiatingParty string              `json:"initgPty"`
	PaymentInfo     ISO20022PaymentInfo `json:"pmtInf"`
}

type ISO20022PaymentInfo struct {
	PaymentID    string `json:"pmtInfId"`
	Method       string `json:"pmtMtd"`
	Amount       string `json:"instdAmt"`
	Currency     string `json:"ccy"`
	CreditorName string `json:"cdtrNm"`
	DebtorName   string `json:"dbtrNm"`
	EndToEndID   string `json:"endToEndId"`
}

// ─────────────────────────────────────────────
// ZK Proof Payment Rail
// ─────────────────────────────────────────────

type ZKProofRequest struct {
	ResourceURL    string    `json:"resourceUrl"`
	Amount         float64   `json:"amount"`
	Asset          string    `json:"asset"`
	Payer          string    `json:"payer"`
	Payee          string    `json:"payee"`
	Nonce          string    `json:"nonce"`
	ExpiresAt      time.Time `json:"expiresAt"`
	PrivacyLevel   string    `json:"privacyLevel"` // "full" | "selective"
	ComplianceHash string    `json:"complianceHash,omitempty"`
}

type ZKProofVerification struct {
	ProofID          string        `json:"proofId"`
	CircuitType      string        `json:"circuitType"` // "noir" | "risc0"
	Verified         bool          `json:"verified"`
	VerificationTime time.Duration `json:"verificationTime"`
	GasUsed          uint64        `json:"gasUsed"`
	OnChainRef       string        `json:"onChainRef"`
	CreatedAt        time.Time     `json:"createdAt"`
}

// ZKSwapRequest represents a ZK proof request for swap operations
type ZKSwapRequest struct {
	ResourceURL    string    `json:"resourceUrl"`
	Amount         float64   `json:"amount"`
	SourceAsset    string    `json:"sourceAsset"`
	DestAsset      string    `json:"destAsset"`
	Payer          string    `json:"payer"`
	Destination    string    `json:"destination"`
	Nonce          string    `json:"nonce"`
	ExpiresAt      time.Time `json:"expiresAt"`
	PrivacyLevel   string    `json:"privacyLevel"` // "full" | "selective"
	ComplianceHash string    `json:"complianceHash,omitempty"`
	SwapType       SwapType  `json:"swapType"`
}

// ZKSwapProof represents the generated ZK proof for a swap
type ZKSwapProof struct {
	ProofHash    string    `json:"proofHash"`
	CircuitType  string    `json:"circuitType"` // "noir" | "risc0"
	PublicInputs []string  `json:"publicInputs"`
	ProofData    []byte    `json:"proofData"`
	GeneratedAt  time.Time `json:"generatedAt"`
	GasUsed      uint64    `json:"gasUsed"`
	SourceAsset  string    `json:"sourceAsset"`
	DestAsset    string    `json:"destAsset"`
	Amount       string    `json:"amount"`
	Destination  string    `json:"destination"`
}

// ZKSwapVerification represents the verification result for a ZK swap
type ZKSwapVerification struct {
	ProofID          string        `json:"proofId"`
	CircuitType      string        `json:"circuitType"`
	Verified         bool          `json:"verified"`
	VerificationTime time.Duration `json:"verificationTime"`
	GasUsed          uint64        `json:"gasUsed"`
	OnChainRef       string        `json:"onChainRef"`
	TxHash           string        `json:"txHash"`
	CreatedAt        time.Time     `json:"createdAt"`
}

// ─────────────────────────────────────────────
// Swap — Path Payments
// ─────────────────────────────────────────────

type SwapType string

const (
	SwapStrictSend    SwapType = "strict-send"
	SwapStrictReceive SwapType = "strict-receive"
)

type PathAsset struct {
	Code   string `json:"code"`
	Issuer string `json:"issuer"`
}

type SwapPath struct {
	Path         []PathAsset `json:"path"`         // Assets in path (code + issuer)
	SourceAmount string      `json:"sourceAmount"` // Amount of source asset
	DestAmount   string      `json:"destAmount"`   // Amount of dest asset
	Price        float64     `json:"price"`        // Exchange rate
}

type SwapQuote struct {
	QuoteID        string     `json:"quoteId"`
	SourceAsset    string     `json:"sourceAsset"`
	DestAsset      string     `json:"destAsset"`
	SwapType       SwapType   `json:"swapType"`
	Amount         string     `json:"amount"`         // Requested amount (send or receive)
	ExpectedAmount string     `json:"expectedAmount"` // Expected amount (receive or send)
	PriceImpact    float64    `json:"priceImpact"`    // Percentage
	NetworkFee     string     `json:"networkFee"`     // XLM fee
	Paths          []SwapPath `json:"paths"`
	ValidUntil     time.Time  `json:"validUntil"`
	CreatedAt      time.Time  `json:"createdAt"`
}

// IsValid checks if the quote has not expired
func (q *SwapQuote) IsValid() bool {
	return time.Now().UTC().Before(q.ValidUntil)
}

// Age returns the age of the quote in seconds
func (q *SwapQuote) Age() float64 {
	return time.Since(q.CreatedAt).Seconds()
}

type SwapRequest struct {
	SourceAsset string   `json:"sourceAsset"`
	DestAsset   string   `json:"destAsset"`
	Amount      string   `json:"amount"`
	SwapType    SwapType `json:"swapType"`
	MaxSlippage float64  `json:"maxSlippage"` // Percentage (e.g., 1.0 = 1%)
	Destination string   `json:"destination"` // Optional: defaults to self
	Memo        string   `json:"memo,omitempty"`
}

// SwapRoundTripResult summarizes a paper XLM→USDC→XLM path round trip and estimated net (after two base fees).
type SwapRoundTripResult struct {
	AmountXLM           string           `json:"amountXlm"`
	BaseAsset           string           `json:"baseAsset"`
	CounterAsset        string           `json:"counterAsset"`
	LegA                *SwapQuote       `json:"legA"`
	LegB                *SwapQuote       `json:"legB"`
	USDCIntermediate    string           `json:"usdcIntermediate"`
	XLMReturned         string           `json:"xlmReturned"`
	CounterIntermediate string           `json:"counterIntermediate"`
	BaseReturned        string           `json:"baseReturned"`
	FeeReserveXLM       string           `json:"feeReserveXlm"`
	EstimatedNetXLM     float64          `json:"estimatedNetXlm"`
	SnapshotBefore      *AccountSnapshot `json:"snapshotBefore,omitempty"`
	SnapshotAfterLegA   *AccountSnapshot `json:"snapshotAfterLegA,omitempty"` // Captures intermediates
	SnapshotAfter       *AccountSnapshot `json:"snapshotAfter,omitempty"`
}

// AccountSnapshot captures all asset balances at a point in time
type AccountSnapshot struct {
	Timestamp time.Time      `json:"timestamp"`
	XLM       string         `json:"xlm"`
	Assets    []AssetBalance `json:"assets,omitempty"`
}

// AssetBalance represents a single asset holding
type AssetBalance struct {
	Code    string `json:"code"`
	Issuer  string `json:"issuer,omitempty"`
	Balance string `json:"balance"`
}

// Liquidity Pool Types
// ─────────────────────────────────────────────

type PoolType string

const (
	PoolTypeConstantProduct PoolType = "constant_product"
)

// PoolReserve represents a single asset in a liquidity pool
type PoolReserve struct {
	Asset  string `json:"asset"`  // Asset code or "XLM" or "CODE:ISSUER"
	Amount string `json:"amount"` // Amount held in pool
}

// LiquidityPool represents a Stellar AMM liquidity pool
type LiquidityPool struct {
	ID               string        `json:"id"`               // Pool ID (hex)
	PagingToken      string        `json:"pagingToken"`      // Pagination token
	Type             PoolType      `json:"type"`             // Pool type (constant_product)
	FeeBP            int32         `json:"feeBp"`            // Fee in basis points (30 = 0.3%)
	TotalShares      string        `json:"totalShares"`      // Total pool shares issued
	Reserves         []PoolReserve `json:"reserves"`         // Asset reserves
	LastModified     int64         `json:"lastModified"`     // Ledger sequence of last modification
	LastModifiedTime string        `json:"lastModifiedTime"` // ISO 8601 timestamp
}

// PoolPrice represents calculated price info for a pool
type PoolPrice struct {
	AssetA       string  `json:"assetA"`
	AssetB       string  `json:"assetB"`
	PriceAtoB    float64 `json:"priceAtoB"` // How much of B per 1 A
	PriceBtoA    float64 `json:"priceBtoA"` // How much of A per 1 B
	ReserveA     float64 `json:"reserveA"`
	ReserveB     float64 `json:"reserveB"`
	LiquidityUSD float64 `json:"liquidityUsd,omitempty"`
}
