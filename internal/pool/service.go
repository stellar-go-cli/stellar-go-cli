package pool

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/models"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/network"
	"github.com/stellar/go/protocols/horizon"
)

// Service provides liquidity pool operations via Horizon
type Service struct {
	client     *horizonclient.Client
	passphrase string
	network    models.Network
}

// NewService creates a pool service for the specified network
func NewService(net models.Network) *Service {
	var client *horizonclient.Client
	var passphrase string

	switch net {
	case models.NetworkStellarMainnet:
		client = horizonclient.DefaultPublicNetClient
		passphrase = network.PublicNetworkPassphrase
	default:
		client = horizonclient.DefaultTestNetClient
		passphrase = network.TestNetworkPassphrase
	}

	return &Service{
		client:     client,
		passphrase: passphrase,
		network:    net,
	}
}

// ListPools fetches liquidity pools from Horizon
func (s *Service) ListPools(limit uint) ([]models.LiquidityPool, error) {
	if limit == 0 || limit > 200 {
		limit = 20
	}

	req := horizonclient.LiquidityPoolsRequest{
		Limit: limit,
	}

	poolsPage, err := s.client.LiquidityPools(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch liquidity pools: %w", err)
	}

	return s.parsePools(poolsPage.Embedded.Records), nil
}

// GetPool fetches a single liquidity pool by ID
func (s *Service) GetPool(poolID string) (*models.LiquidityPool, error) {
	req := horizonclient.LiquidityPoolRequest{
		LiquidityPoolID: poolID,
	}

	pool, err := s.client.LiquidityPoolDetail(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pool %s: %w", poolID, err)
	}

	return s.parsePool(&pool), nil
}

// GetPoolsForAsset fetches pools containing a specific asset
func (s *Service) GetPoolsForAsset(assetCode string, limit uint) ([]models.LiquidityPool, error) {
	if limit == 0 || limit > 200 {
		limit = 20
	}

	// Build asset filter
	var reserves []string
	if assetCode == "XLM" || assetCode == "native" {
		reserves = append(reserves, "native")
	} else {
		reserves = append(reserves, assetCode)
	}

	req := horizonclient.LiquidityPoolsRequest{
		Limit:    limit,
		Reserves: reserves,
	}

	poolsPage, err := s.client.LiquidityPools(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pools for asset %s: %w", assetCode, err)
	}

	return s.parsePools(poolsPage.Embedded.Records), nil
}

// CalculatePrice computes exchange rates from pool reserves
func (s *Service) CalculatePrice(pool *models.LiquidityPool) *models.PoolPrice {
	if len(pool.Reserves) != 2 {
		return nil
	}

	reserveA, _ := strconv.ParseFloat(pool.Reserves[0].Amount, 64)
	reserveB, _ := strconv.ParseFloat(pool.Reserves[1].Amount, 64)

	if reserveA == 0 || reserveB == 0 {
		return nil
	}

	return &models.PoolPrice{
		AssetA:    s.formatAsset(pool.Reserves[0].Asset),
		AssetB:    s.formatAsset(pool.Reserves[1].Asset),
		PriceAtoB: reserveB / reserveA,
		PriceBtoA: reserveA / reserveB,
		ReserveA:  reserveA,
		ReserveB:  reserveB,
	}
}

// parsePools converts Horizon records to model types
func (s *Service) parsePools(records []horizon.LiquidityPool) []models.LiquidityPool {
	pools := make([]models.LiquidityPool, len(records))
	for i, r := range records {
		pools[i] = *s.parsePool(&r)
	}
	return pools
}

// parsePool converts a single Horizon record to model type
func (s *Service) parsePool(r *horizon.LiquidityPool) *models.LiquidityPool {
	reserves := make([]models.PoolReserve, len(r.Reserves))
	for i, res := range r.Reserves {
		reserves[i] = models.PoolReserve{
			Asset:  res.Asset,
			Amount: res.Amount,
		}
	}

	// Convert time to string
	var lastModifiedTime string
	if r.LastModifiedTime != nil {
		lastModifiedTime = r.LastModifiedTime.Format(time.RFC3339)
	}

	return &models.LiquidityPool{
		ID:               r.ID,
		PagingToken:      r.PagingToken(),
		Type:             models.PoolType(r.Type),
		FeeBP:            int32(r.FeeBP),
		TotalShares:      r.TotalShares,
		Reserves:         reserves,
		LastModified:     int64(r.LastModifiedLedger),
		LastModifiedTime: lastModifiedTime,
	}
}

// formatAsset converts Horizon asset format to readable format
func (s *Service) formatAsset(asset string) string {
	if asset == "native" {
		return "XLM"
	}
	// If asset contains issuer, show just the code
	if strings.Contains(asset, ":") {
		parts := strings.Split(asset, ":")
		if len(parts) >= 1 {
			return parts[0]
		}
	}
	return asset
}

// GetNetwork returns the network this service is configured for
func (s *Service) GetNetwork() models.Network {
	return s.network
}

// ExplorerURL returns the Stellar Expert URL for a pool
func (s *Service) ExplorerURL(poolID string) string {
	if s.network == models.NetworkStellarMainnet {
		return fmt.Sprintf("https://stellar.expert/explorer/public/liquidity-pool/%s", poolID)
	}
	return fmt.Sprintf("https://stellar.expert/explorer/testnet/liquidity-pool/%s", poolID)
}
