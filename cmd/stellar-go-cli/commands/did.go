package commands

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/did"
	"github.com/stellar-go-cli/stellar-go-cli/internal/models"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
	mpCrypto "github.com/stellar-go-cli/stellar-go-cli/pkg/crypto"
)

// proof type constants for CLI flags
const (
	proofTypeEd25519 = "ed25519"
	proofTypeJWS     = "jws"
)

func newDIDCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "did",
		Short: "Manage decentralized identifiers and verifiable credentials",
		Long:  "Create DIDs using web/key/ethr/ebsi methods. Issue and verify W3C Verifiable Credentials.",
		cfg:   cfg,
	}

	// Sub-commands
	cmd.addSub(newDIDCreateCmd(cfg))
	cmd.addSub(newDIDAttestCmd(cfg))
	cmd.addSub(newDIDVerifyCmd(cfg))
	cmd.addSub(newDIDShowCmd(cfg))

	cmd.Run = func(c *Command, args []string) error {
		c.printHelp()
		return nil
	}

	return cmd
}

// ─── did create ───────────────────────────────

func newDIDCreateCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	method := fs.String("method", "key", "DID method: web | key | ethr | ebsi")
	domain := fs.String("domain", "mozartpay.com", "Domain for did:web (used with --method web)")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "create",
		Short: "Create a new DID document",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Create DID")

			m := models.DIDMethod(*method)
			spin := ui.NewSpinner(fmt.Sprintf("Generating did:%s document...", m))
			spin.Start()
			time.Sleep(600 * time.Millisecond)

			svc, err := did.NewService()
			if err != nil {
				spin.Stop(false, "Key generation failed")
				return err
			}

			var doc *models.DIDDocument
			if m == models.DIDMethodWeb {
				doc, err = svc.CreateDID(m, *domain)
			} else {
				doc, err = svc.CreateDID(m)
			}
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}

			spin.Stop(true, "DID document created")

			if *output == "json" {
				fmt.Println(did.PrettyPrint(doc))
				return nil
			}

			ui.SectionLabel("DID Document")
			ui.KV("DID", doc.ID)
			ui.KV("Method", string(doc.Method))
			ui.KV("Key Type", doc.VerificationMethod[0].Type)
			ui.KV("Public Key", safeTrunc(doc.VerificationMethod[0].PublicKeyHex, 32)+"...")
			ui.KV("Created", doc.Created.Format(time.RFC3339))

			// Save to state
			if err := config.SaveState("did_"+string(m), doc); err == nil {
				ui.Info("Saved to ~/.mozartpay/state/did_" + string(m) + ".json")
			}

			// Update config with active DID
			cfg.ActiveDID = doc.ID
			cfg.DIDMethod = string(m)
			config.Save(cfg)

			return nil
		},
	}
}

// ─── did attest ───────────────────────────────

func newDIDAttestCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("attest", flag.ContinueOnError)
	method := fs.String("method", "ebsi", "DID method for attestation")
	vcType := fs.String("vc", "national-id", "VC type: national-id | kyc | accreditation")
	name := fs.String("name", "", "Holder full name")
	country := fs.String("country", "AT", "ISO country code")
	birthYear := fs.String("birth-year", "", "Holder birth year (optional, omitted if empty)")
	level := fs.String("level", "KYC_LEVEL_2", "KYC level for the credential")
	output := fs.String("output", "pretty", "Output format: pretty | json")
	useWalletKey := fs.Bool("use-wallet-key", false, "Sign VC with the active wallet's key instead of an ephemeral key")
	proofType := fs.String("proof-type", proofTypeEd25519, "Proof type: ed25519 | jws")

	return &Command{
		Name:  "attest",
		Short: "Issue a Verifiable Credential (VC) via national ID / KYC",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("DID Attestation")

			if *name == "" {
				*name = ui.Prompt("Holder name:")
			}

			m := models.DIDMethod(*method)

			spin := ui.NewSpinner("Creating DID and issuing VC...")
			spin.Start()
			time.Sleep(800 * time.Millisecond)

			var svc *did.Service
			if *useWalletKey {
				ws := wallet.NewService()
				acc, werr := ws.GetActiveWallet()
				if werr != nil || acc.PrivateKey == "" {
					spin.Stop(false, "No active wallet with private key found")
					return fmt.Errorf("no active wallet: %w", werr)
				}
				if *proofType == proofTypeJWS {
					key, kerr := mpCrypto.DeriveKeyFromSeed(acc.PrivateKey)
					if kerr != nil {
						spin.Stop(false, "Key derivation failed")
						return kerr
					}
					svc = did.NewServiceWithECDSAKey(key)
				} else {
					s, serr := did.NewServiceWithStellarSeed(acc.PrivateKey)
					if serr != nil {
						spin.Stop(false, "Stellar seed error: "+serr.Error())
						return serr
					}
					svc = s
				}
				ui.Info(fmt.Sprintf("VC signed with wallet key (%s...)", acc.Address[:8]))
			} else {
				if *proofType == proofTypeJWS {
					eckey, eerr := mpCrypto.GenerateKeyPair()
					if eerr != nil {
						spin.Stop(false, "Key generation failed")
						return eerr
					}
					svc = did.NewServiceWithECDSAKey(eckey)
				} else {
					var err error
					svc, err = did.NewService()
					if err != nil {
						spin.Stop(false, "Failed")
						return err
					}
				}
			}

			doc, err := svc.CreateDID(m)
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}

			vcTypeName := vcTypeToName(*vcType)
			vc, err := svc.IssueNationalIDVC(doc.ID, doc.ID, *name, *country, "", *birthYear, *level)
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}
			vc.Type = append(vc.Type[:1], vcTypeName)

			spin.Stop(true, "Attestation complete")

			config.SaveState("vc_latest", vc)
			config.SaveState("did_"+string(m), doc)
			cfg.ActiveDID = doc.ID
			cfg.DIDMethod = string(m)
			config.Save(cfg)

			if *output == "json" {
				fmt.Println(did.PrettyPrint(vc))
				return nil
			}

			ui.SectionLabel("Verifiable Credential")
			ui.KV("VC ID", vc.ID)
			ui.KV("Type", vcTypeName)
			ui.KV("Issuer DID", safeTrunc(vc.Issuer, 40)+"...")
			ui.KV("Subject", *name)
			ui.KV("Country", *country)
			ui.KV("Level", *level)
			ui.KV("Issued", vc.IssuanceDate.Format(time.RFC3339))
			ui.KV("Expires", vc.ExpirationDate.Format("2006-01-02"))
			ui.KV("Proof Type", vc.Proof.Type)
			if vc.Proof.ProofValue != "" {
				ui.KV("Proof Value (trunc)", safeTrunc(vc.Proof.ProofValue, 24)+"...")
			}
			if vc.Proof.JWSSignature != "" {
				ui.KV("JWS (trunc)", safeTrunc(vc.Proof.JWSSignature, 24)+"...")
			}
			if *useWalletKey {
				ui.KVColor("Signed By", "wallet key", ui.BrightGreen)
			} else {
				ui.KV("Signed By", "ephemeral key")
			}

			ui.Info("Saved to ~/.mozartpay/state/vc_latest.json")
			ui.KVColor("Active DID", safeTrunc(doc.ID, 50)+"...", ui.Teal)

			return nil
		},
	}
}

// ─── did verify ───────────────────────────────

func newDIDVerifyCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	vcFile := fs.String("vc-file", "", "Path to VC JSON file (optional, uses saved state if omitted)")

	return &Command{
		Name:  "verify",
		Short: "Verify a Verifiable Credential",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Verify VC")

			var vc models.VerifiableCredential
			if *vcFile != "" {
				data, err := os.ReadFile(*vcFile)
				if err != nil {
					return fmt.Errorf("read VC file: %w", err)
				}
				if err := json.Unmarshal(data, &vc); err != nil {
					return fmt.Errorf("parse VC file: %w", err)
				}
			} else if err := config.LoadState("vc_latest", &vc); err != nil {
				ui.Warn("No saved VC found. Run 'mozartpay did attest' first.")
				return nil
			}

			spin := ui.NewSpinner("Verifying credential...")
			spin.Start()
			time.Sleep(500 * time.Millisecond)

			svc, _ := did.NewService()
			valid, err := svc.Verify(&vc)
			if err != nil {
				spin.Stop(false, "Verification failed: "+err.Error())
				return fmt.Errorf("verification failed: %w", err)
			}
			spin.Stop(valid, map[bool]string{true: "Credential is VALID", false: "Credential is INVALID"}[valid])

			ui.SectionLabel("Verification Result")
			ui.KV("VC ID", vc.ID)
			ui.KV("Issuer", safeTrunc(vc.Issuer, 40)+"...")
			ui.KV("Proof Type", vc.Proof.Type)
			ui.KV("Valid", fmt.Sprintf("%v", valid))
			ui.KV("Expires", vc.ExpirationDate.Format("2006-01-02"))
			ui.KV("Status", map[bool]string{true: "✓ VERIFIED", false: "✗ INVALID"}[valid])

			if !valid {
				return fmt.Errorf("credential verification failed")
			}
			return nil
		},
	}
}

// ─── did show ────────────────────────────────

func newDIDShowCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "show",
		Short: "Show supported DID methods",
		Run: func(c *Command, args []string) error {
			ui.Header("DID Methods")
			fmt.Println()

			methods := []struct {
				method  string
				spec    string
				usecase string
			}{
				{"did:web", "W3C DID Core", "Web-hosted DIDs for organizations (TLS-anchored)"},
				{"did:key", "W3C DID Core", "Self-sovereign, portable, no registry required"},
				{"did:ethr", "ERC-1056", "Ethereum-based DID, smart contract anchoring"},
				{"did:ebsi", "EBSI v3", "EU Blockchain Services Infrastructure (eIDAS 2.0)"},
			}

			t := ui.NewTable("Method", "Spec", "Use Case")
			for _, m := range methods {
				t.AddRow(ui.Teal_(m.method), m.spec, m.usecase)
			}
			t.Print()

			if cfg.ActiveDID != "" {
				ui.SectionLabel("Active DID")
				ui.KV("DID", cfg.ActiveDID)
				ui.KV("Method", cfg.DIDMethod)
			}

			return nil
		},
	}
}

// ─────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────

func safeTrunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func vcTypeToName(t string) string {
	switch t {
	case "national-id":
		return "NationalIdentityCredential"
	case "kyc":
		return "KYCCredential"
	case "accreditation":
		return "AccreditationCredential"
	default:
		return "VerifiableCredential"
	}
}
