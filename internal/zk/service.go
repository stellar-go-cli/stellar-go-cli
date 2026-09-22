package zk

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	mpCrypto "github.com/stellar-go-cli/stellar-go-cli/pkg/crypto"
)

// Service handles ZK proof generation and verification
type Service struct {
	circuitPath      string
	compiledPath     string
	provingKeyPath   string
	verifyingKeyPath string
}

// NewService creates a new ZK service
func NewService() *Service {
	baseDir := filepath.Join(os.Getenv("HOME"), ".mozartpay", "zk")
	return &Service{
		circuitPath:      filepath.Join(baseDir, "circuit.noir"),
		compiledPath:     filepath.Join(baseDir, "circuit.json"),
		provingKeyPath:   filepath.Join(baseDir, "proving_key.pk"),
		verifyingKeyPath: filepath.Join(baseDir, "verifying_key.vk"),
	}
}

// ProofData represents the generated ZK proof
type ProofData struct {
	Proof        string    `json:"proof"`
	PublicInputs []string  `json:"public_inputs"`
	ProofHash    string    `json:"proof_hash"`
	CircuitType  string    `json:"circuit_type"`
	GeneratedAt  time.Time `json:"generated_at"`
	GasUsed      uint64    `json:"gas_used"`
}

// PaymentInputs represents the inputs for ZK payment proof generation
type PaymentInputs struct {
	SenderPrivateKey string   `json:"sender_private_key"`
	ReceiverAddress  string   `json:"receiver_address"`
	Amount           string   `json:"amount"`
	Nonce            string   `json:"nonce"`
	NetworkID        string   `json:"network_id"`
	MaxAmount        string   `json:"max_amount"`
	PublicInputs     []string `json:"public_inputs"`
}

// GeneratePaymentProof generates a ZK proof for a payment transaction
func (s *Service) GeneratePaymentProof(inputs PaymentInputs) (*ProofData, error) {
	// Ensure circuit is compiled
	if err := s.ensureCircuitCompiled(); err != nil {
		return nil, fmt.Errorf("circuit compilation failed: %w", err)
	}

	// Prepare Prover.toml file
	proverInput := map[string]interface{}{
		"sender_private_key": inputs.SenderPrivateKey,
		"receiver_address":   inputs.ReceiverAddress,
		"amount":             inputs.Amount,
		"nonce":              inputs.Nonce,
		"signature_r":        "0", // Mock signature for now
		"signature_s":        "0", // Mock signature for now
		"sender_address":     s.derivePublicKey(inputs.SenderPrivateKey),
		"receiver_pubkey_x":  inputs.ReceiverAddress,
		"receiver_pubkey_y":  "0", // Mock Y coordinate
		"amount_hash":        s.hashAmount(inputs.Amount, inputs.Nonce, inputs.NetworkID),
		"nonce_hash":         s.hashNonce(inputs.Nonce, s.derivePublicKey(inputs.SenderPrivateKey)),
		"max_amount":         inputs.MaxAmount,
		"network_id":         inputs.NetworkID,
	}

	// Write Prover.toml
	proverPath := filepath.Join(filepath.Dir(s.circuitPath), "Prover.toml")
	if err := s.writeToml(proverPath, proverInput); err != nil {
		return nil, fmt.Errorf("failed to write prover input: %w", err)
	}

	// Generate proof using Noir
	start := time.Now()
	proofOutput, err := s.executeNoirCommand("prove", s.compiledPath, proverPath)
	if err != nil {
		return nil, fmt.Errorf("proof generation failed: %w", err)
	}
	proofDuration := time.Since(start)

	// Parse proof output
	proofData, err := s.parseProofOutput(proofOutput, proofDuration)
	if err != nil {
		return nil, fmt.Errorf("failed to parse proof output: %w", err)
	}

	// Cache the proof
	config.SaveState("zk_proof_latest", proofData)

	return proofData, nil
}

// VerifyProofLocally verifies a ZK proof locally before submission
func (s *Service) VerifyProofLocally(proofData *ProofData) (bool, error) {
	// Prepare Verifier.toml
	verifierInput := map[string]interface{}{
		"proof":         proofData.Proof,
		"public_inputs": proofData.PublicInputs,
	}

	verifierPath := filepath.Join(filepath.Dir(s.circuitPath), "Verifier.toml")
	if err := s.writeToml(verifierPath, verifierInput); err != nil {
		return false, fmt.Errorf("failed to write verifier input: %w", err)
	}

	// Verify proof using Noir
	output, err := s.executeNoirCommand("verify", s.compiledPath, verifierPath)
	if err != nil {
		return false, fmt.Errorf("verification failed: %w", err)
	}

	// Check if verification succeeded
	return strings.Contains(output, "Verification succeeded"), nil
}

// ensureCircuitCompiled ensures the Noir circuit is compiled
func (s *Service) ensureCircuitCompiled() error {
	// Check if already compiled
	if _, err := os.Stat(s.compiledPath); err == nil {
		return nil
	}

	// Create directory
	if err := os.MkdirAll(filepath.Dir(s.circuitPath), 0755); err != nil {
		return fmt.Errorf("failed to create ZK directory: %w", err)
	}

	// Copy circuit file if it doesn't exist
	if _, err := os.Stat(s.circuitPath); os.IsNotExist(err) {
		// Create a basic circuit file for demonstration
		basicCircuit := `
// Basic ZK payment circuit
fn main(
    private sender_private_key: Field,
    private receiver_address: Field,
    private amount: Field,
    private nonce: Field,
    public sender_address: Field,
    public receiver_pubkey_x: Field,
    public amount_hash: Field,
    public nonce_hash: Field,
    public max_amount: Field,
    public network_id: Field,
) {
    constrain amount > 0;
    constrain amount <= max_amount;
    constrain network_id == 1;
}
`
		if err := ioutil.WriteFile(s.circuitPath, []byte(basicCircuit), 0644); err != nil {
			return fmt.Errorf("failed to create circuit file: %w", err)
		}
	}

	// Compile circuit
	_, err := s.executeNoirCommand("compile", s.circuitPath)
	return err
}

// executeNoirCommand executes a Noir CLI command
func (s *Service) executeNoirCommand(cmd string, args ...string) (string, error) {
	// For demonstration, we'll simulate Noir execution
	// In real implementation, this would call the actual Noir binary

	switch cmd {
	case "compile":
		// Simulate compilation
		time.Sleep(2 * time.Second)
		// Create mock compiled file
		mockCompiled := `{"name": "circuit", "hash": "mock_hash"}`
		err := ioutil.WriteFile(s.compiledPath, []byte(mockCompiled), 0644)
		return "Compilation completed", err

	case "prove":
		// Simulate proof generation
		time.Sleep(8 * time.Second)
		// Generate mock proof
		proof := mpCrypto.RandomHex(128)
		publicInputs := []string{"mock_input1", "mock_input2"}
		proofHash := mpCrypto.Hash256([]byte(proof + strings.Join(publicInputs, "")))

		return fmt.Sprintf(`{
			"proof": "%s",
			"public_inputs": [%s],
			"proof_hash": "%s"
		}`, proof, `"`+strings.Join(publicInputs, `","`)+`"`, proofHash), nil

	case "verify":
		// Simulate verification
		time.Sleep(2 * time.Second)
		return "Verification succeeded", nil

	default:
		return "", fmt.Errorf("unknown Noir command: %s", cmd)
	}
}

// parseProofOutput parses the proof output from Noir
func (s *Service) parseProofOutput(output string, duration time.Duration) (*ProofData, error) {
	// For demonstration, parse mock output
	proof := mpCrypto.RandomHex(128)
	publicInputs := []string{"mock_sender", "mock_receiver", "mock_amount"}
	proofHash := mpCrypto.Hash256([]byte(proof + strings.Join(publicInputs, "")))

	return &ProofData{
		Proof:        proof,
		PublicInputs: publicInputs,
		ProofHash:    proofHash,
		CircuitType:  "noir",
		GeneratedAt:  time.Now().UTC(),
		GasUsed:      2500000, // Estimated gas for ZK verification
	}, nil
}

// Helper functions
func (s *Service) derivePublicKey(privateKey string) string {
	// Simplified public key derivation
	// In real implementation, use Ed25519 key derivation
	return "G" + mpCrypto.RandomHex(56)
}

func (s *Service) hashAmount(amount, nonce, networkID string) string {
	hash := sha256.Sum256([]byte(amount + nonce + networkID))
	return hex.EncodeToString(hash[:])
}

func (s *Service) hashNonce(nonce, senderAddress string) string {
	hash := sha256.Sum256([]byte(nonce + senderAddress))
	return hex.EncodeToString(hash[:])
}

func (s *Service) writeToml(path string, data map[string]interface{}) error {
	var lines []string
	for key, value := range data {
		lines = append(lines, fmt.Sprintf("%s = %v", key, value))
	}
	return ioutil.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// GetCircuitHash returns the compiled circuit hash
func (s *Service) GetCircuitHash() (string, error) {
	if _, err := os.Stat(s.compiledPath); err != nil {
		return "", fmt.Errorf("circuit not compiled")
	}

	// For demonstration, return mock hash
	return mpCrypto.Hash256([]byte("mock_circuit")), nil
}

// EstimateProofGas estimates the gas cost for proof verification
func (s *Service) EstimateProofGas(proofData *ProofData) uint64 {
	// Base gas cost for ZK verification
	baseGas := uint64(2000000)

	// Additional gas based on proof size
	proofSize := len(proofData.Proof) / 2 // Hex to bytes
	sizeGas := uint64(proofSize) * 10

	// Additional gas based on public inputs count
	inputsGas := uint64(len(proofData.PublicInputs)) * 50000

	return baseGas + sizeGas + inputsGas
}
