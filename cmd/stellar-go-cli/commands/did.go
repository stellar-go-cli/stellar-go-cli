package commands

import (
	"crypto/ecdsa"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
	mpCrypto "github.com/stellar-go-cli/stellar-go-cli/pkg/crypto"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/vc"
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
		Long: "Create DIDs using web/key/ethr/ebsi methods. Issue and verify W3C Verifiable Credentials " +
			"(Ed25519Signature2020 for Ed25519 issuers, ES256 JsonWebSignature2020 for ECDSA issuers). " +
			"did:ethr and did:ebsi identifiers embed the public key and are locally derived — nothing is " +
			"registered on Ethereum or EBSI.",
		cfg: cfg,
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
	domain := fs.String("domain", "example.com", "Domain for did:web (used with --method web)")
	keyType := fs.String("key-type", "ed25519", "Key type: ed25519 | ecdsa (ethr/ebsi require ecdsa)")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "create",
		Short: "Create a new DID document",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Create DID")

			m := models.DIDMethod(*method)
			if m == models.DIDMethodEthr || m == models.DIDMethodEBSI {
				*keyType = "ecdsa" // ethr/ebsi identifiers carry an ECDSA key
			}
			spin := ui.NewSpinner(fmt.Sprintf("Generating did:%s document...", m))
			spin.Start()
			time.Sleep(600 * time.Millisecond)

			var svc *vc.Service
			var err error
			if *keyType == "ecdsa" {
				var key *ecdsa.PrivateKey
				key, err = mpCrypto.GenerateKeyPair()
				if err == nil {
					svc = vc.NewServiceWithECDSAKey(key)
				}
			} else {
				svc, err = vc.NewService()
			}
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
				fmt.Println(vc.PrettyPrint(doc))
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
				ui.Info("Saved to ~/.stellar-go-cli/state/did_" + string(m) + ".json")
			}

			// Update config with active DID
			cfg.ActiveDID = doc.ID
			cfg.DIDMethod = string(m)
			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			return nil
		},
	}
}

// ─── did attest ───────────────────────────────

func newDIDAttestCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("attest", flag.ContinueOnError)
	method := fs.String("method", "key", "DID method for the issuer identity: key | web (ed25519) | ethr | ebsi (ECDSA, forces --proof-type jws)")
	domain := fs.String("domain", "example.com", "Domain for did:web issuer (used with --method web)")
	vcType := fs.String("vc", "national-id", "VC type: national-id | kyc | accreditation")
	subjectDID := fs.String("subject-did", "", "DID of the credential subject (default: same as issuer — self-issued)")
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
			if m == models.DIDMethodEthr || m == models.DIDMethodEBSI {
				*proofType = proofTypeJWS // ethr/ebsi issuers sign with an ECDSA key
			}

			if m == models.DIDMethodWeb {
				ui.Warn("A did:web issuer is verifiable only if the DID document is hosted at the domain")
			}

			spin := ui.NewSpinner("Creating DID and issuing VC...")
			spin.Start()
			time.Sleep(800 * time.Millisecond)

			var svc *vc.Service
			var protectedIssuer, protectedAccount string
			if *useWalletKey {
				ws := wallet.NewService()
				acc, werr := ws.GetActiveWallet()
				if werr != nil {
					spin.Stop(false, "No active wallet found")
					return fmt.Errorf("no active wallet: %w", werr)
				}
				if acc.PasskeyProtected {
					// Protected wallet: VC signing happens inside the custodian —
					// the seed never enters this process. The daemon derives the
					// issuer DID from the method + proof type (ed25519 and ECDSA
					// issuers both supported).
					if m == models.DIDMethodWeb {
						spin.Stop(false, "did:web needs a hosted document")
						return fmt.Errorf("did:web is not a wallet-derived issuer")
					}
					protectedAccount = acc.Address
					protectedIssuer = "method:" + string(m) + ":" + *proofType
					ui.Info(fmt.Sprintf("VC signed by custodian for protected wallet (%s...)", acc.Address[:8]))
				} else {
					seed, serr := wallet.ResolveSeed(acc)
					if serr != nil {
						spin.Stop(false, "No usable key in active wallet")
						return serr
					}
					if *proofType == proofTypeJWS {
						key, kerr := mpCrypto.DeriveKeyFromSeed(seed)
						if kerr != nil {
							spin.Stop(false, "Key derivation failed")
							return kerr
						}
						svc = vc.NewServiceWithECDSAKey(key)
					} else {
						s, serr := vc.NewServiceWithStellarSeed(seed)
						if serr != nil {
							spin.Stop(false, "Stellar seed error: "+serr.Error())
							return serr
						}
						svc = s
					}
					ui.Info(fmt.Sprintf("VC signed with wallet key (%s...)", acc.Address[:8]))
				}
			} else {
				if *proofType == proofTypeJWS {
					eckey, eerr := mpCrypto.GenerateKeyPair()
					if eerr != nil {
						spin.Stop(false, "Key generation failed")
						return eerr
					}
					svc = vc.NewServiceWithECDSAKey(eckey)
				} else {
					var err error
					svc, err = vc.NewService()
					if err != nil {
						spin.Stop(false, "Failed")
						return err
					}
				}
			}

			vcTypeName := vcTypeToName(*vcType)
			holderDID := *subjectDID
			var doc *models.DIDDocument
			var cred *models.VerifiableCredential
			var err error
			if protectedIssuer != "" {
				// Unsigned subject + type only — the custodian builds and signs
				// the VC internally with the wallet's derived key.
				if *level == "" {
					*level = "KYC_LEVEL_2"
				}
				subject := map[string]interface{}{
					"id":       protectedIssuer,
					"name":     *name,
					"country":  *country,
					"verified": true,
					"level":    *level,
				}
				if *birthYear != "" {
					subject["birthYear"] = *birthYear
				}
				if *subjectDID != "" {
					subject["id"] = *subjectDID
				}
				holderDID = fmt.Sprint(subject["id"])
				unsigned, merr := json.Marshal(map[string]interface{}{
					"type":              []string{"VerifiableCredential", vcTypeToName(*vcType)},
					"credentialSubject": subject,
				})
				if merr != nil {
					spin.Stop(false, "Serialize credential failed")
					return merr
				}
				rw, rerr := custodianRemote()
				if rerr != nil {
					spin.Stop(false, "Custodian unavailable")
					return rerr
				}
				sres, serr := rw.SignCredential(protectedAccount, "", string(m), *proofType, string(unsigned))
				if serr != nil {
					spin.Stop(false, "Custodian credential signing failed")
					return serr
				}
				signedVC, _ := sres["signed_vc"].(string)
				issuerDID, _ := sres["issuer_did"].(string)
				if issuerDID == "" {
					issuerDID = cred.Issuer
				}
				if jerr := json.Unmarshal([]byte(signedVC), &cred); jerr != nil {
					spin.Stop(false, "Invalid credential from custodian")
					return fmt.Errorf("invalid signed credential: %w", jerr)
				}
				if issuerDID != "" {
					cred.Issuer = issuerDID
				}
				doc = &models.DIDDocument{ID: issuerDID, Method: m}
			} else {
				var derr error
				doc, derr = svc.CreateDID(m, *domain)
				if derr != nil {
					spin.Stop(false, derr.Error())
					return derr
				}

				if holderDID == "" {
					holderDID = doc.ID
				}
				cred, err = svc.IssueNationalIDVC(doc.ID, holderDID, *name, *country, "", *birthYear, *level, vcTypeName)
				if err != nil {
					spin.Stop(false, err.Error())
					return err
				}
			}

			spin.Stop(true, "Attestation complete")

			config.SaveState("vc_latest", cred)     //nolint:errcheck // best-effort state cache
			config.SaveState("did_"+string(m), doc) //nolint:errcheck // best-effort state cache
			cfg.ActiveDID = doc.ID
			cfg.DIDMethod = string(m)
			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			if *output == "json" {
				fmt.Println(vc.PrettyPrint(cred))
				return nil
			}

			ui.SectionLabel("Verifiable Credential")
			ui.KV("VC ID", cred.ID)
			ui.KV("Type", vcTypeName)
			ui.KV("Issuer DID", safeTrunc(cred.Issuer, 40)+"...")
			ui.KV("Subject DID", safeTrunc(holderDID, 40)+"...")
			ui.KV("Self-issued", fmt.Sprintf("%v", holderDID == cred.Issuer))
			ui.KV("Subject", *name)
			ui.KV("Country", *country)
			ui.KV("Level", *level)
			ui.KV("Issued", cred.IssuanceDate.Format(time.RFC3339))
			ui.KV("Expires", cred.ExpirationDate.Format("2006-01-02"))
			ui.KV("Proof Type", cred.Proof.Type)
			if cred.Proof.ProofValue != "" {
				ui.KV("Proof Value (trunc)", safeTrunc(cred.Proof.ProofValue, 24)+"...")
			}
			if cred.Proof.JWSSignature != "" {
				ui.KV("JWS (trunc)", safeTrunc(cred.Proof.JWSSignature, 24)+"...")
			}
			if *useWalletKey {
				ui.KVColor("Signed By", "wallet key", ui.BrightGreen)
			} else {
				ui.KV("Signed By", "ephemeral key")
			}

			ui.Info("Saved to ~/.stellar-go-cli/state/vc_latest.json")
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

			var doc map[string]interface{}
			if *vcFile != "" {
				data, err := os.ReadFile(*vcFile)
				if err != nil {
					return fmt.Errorf("read VC file: %w", err)
				}
				if err := json.Unmarshal(data, &doc); err != nil {
					return fmt.Errorf("parse VC file: %w", err)
				}
			} else if err := config.LoadState("vc_latest", &doc); err != nil {
				ui.Warn("No saved VC found. Run 'stellar-go-cli did attest' first.")
				return nil
			}

			spin := ui.NewSpinner("Verifying credential...")
			spin.Start()
			time.Sleep(500 * time.Millisecond)

			valid, err := vc.VerifyDocument(doc)
			if err != nil {
				spin.Stop(false, "Verification failed: "+err.Error())
				return fmt.Errorf("verification failed: %w", err)
			}
			spin.Stop(valid, map[bool]string{true: "Credential is VALID", false: "Credential is INVALID"}[valid])

			issuer, _ := doc["issuer"].(string)
			var subjectID string
			if subj, ok := doc["credentialSubject"].(map[string]interface{}); ok {
				subjectID, _ = subj["id"].(string)
			}
			selfIssued := "-"
			if subjectID != "" {
				selfIssued = fmt.Sprintf("%v", subjectID == issuer)
			}
			expires := ""
			for _, key := range []string{"validUntil", "expirationDate"} {
				if s, ok := doc[key].(string); ok && s != "" {
					expires = s[:10]
					break
				}
			}
			proofType := ""
			if proof, ok := doc["proof"].(map[string]interface{}); ok {
				proofType, _ = proof["type"].(string)
			}

			ui.SectionLabel("Verification Result")
			ui.KV("VC ID", fmt.Sprintf("%v", doc["id"]))
			ui.KV("Issuer", safeTrunc(issuer, 40)+"...")
			ui.KV("Subject", safeTrunc(subjectID, 40)+"...")
			ui.KV("Self-issued", selfIssued)
			ui.KV("Proof Type", proofType)
			ui.KV("Valid", fmt.Sprintf("%v", valid))
			if expires != "" {
				ui.KV("Expires", expires)
			}
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
				{"did:web", "W3C DID Core", "Web-hosted DIDs for organizations (TLS-anchored); verifiable"},
				{"did:key", "W3C DID Core", "Self-sovereign, portable, no registry required; verifiable"},
				{"did:ethr", "ERC-1056", "Key-embedded identifier, locally derived — no on-chain registration; verifiable (ES256 JWS)"},
				{"did:ebsi", "EBSI v3", "Key-embedded identifier, locally derived — no EBSI registry anchoring; verifiable (ES256 JWS)"},
			}

			t := ui.NewTable("Method", "Spec", "Use Case")
			for _, m := range methods {
				t.AddRow(ui.Teal_(m.method), m.spec, m.usecase)
			}
			t.Print()

			ui.Info("All methods support credential verification: Ed25519Signature2020 for did:key/did:web, ES256 JsonWebSignature2020 for ECDSA issuers.")

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
