package iso20022

import (
	"fmt"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
)

// ─────────────────────────────────────────────
// Instruction-level types shared by batch builders
// ─────────────────────────────────────────────

// Party identifies a debtor or creditor in a credit transfer, with enough
// optional detail to reach them: a real-world name, contact handles
// (phone/email — e.g. disbursement receivers), an account in IBAN, generic
// ID, or proxy (wallet handle) form, and the servicing agent's BIC.
type Party struct {
	Name        string           `json:"name,omitempty"`
	Address     *PostalAddress27 `json:"address,omitempty"`
	Phone       string           `json:"phone,omitempty"`
	Email       string           `json:"email,omitempty"`
	ID          string           `json:"id,omitempty"`
	IDScheme    string           `json:"idScheme,omitempty"` // Othr>SchmeNm>Cd for ID
	AgentBIC    string           `json:"agentBic,omitempty"`
	AcctIBAN    string           `json:"acctIban,omitempty"`
	AcctID      string           `json:"acctId,omitempty"`      // Othr/Id (≤34 chars); longer values go to Prxy
	AcctProxyID string           `json:"acctProxyId,omitempty"` // Prxy/Id — phone/email/wallet handle
	AcctProxyTp string           `json:"acctProxyTp,omitempty"` // Prxy/Tp>Prtry — e.g. "PHONE","EMAIL","WALLET"
}

// CreditTransferInstruction pairs a Stellar payment with the party detail an
// ISO 20022 message needs. A bare Payment is sufficient — missing party fields
// fall back to From/To-derived defaults — but disbursement-style flows
// (receiver name, phone/email, wallet handle) populate Creditor explicitly.
//
// TxStatus/TxReason are only read by BuildPacs002Batch.
type CreditTransferInstruction struct {
	Payment        *models.Payment   `json:"payment"`
	Debtor         *Party            `json:"debtor,omitempty"`
	Creditor       *Party            `json:"creditor,omitempty"`
	RemittanceInfo []string          `json:"remittanceInfo,omitempty"`
	TxStatus       TransactionStatus `json:"txStatus,omitempty"`
	TxReason       string            `json:"txReason,omitempty"`
}

// requireInstructions validates a non-empty batch of instructions whose
// payments are non-nil.
func requireInstructions(instrs []*CreditTransferInstruction, builder string) error {
	if len(instrs) == 0 {
		return fmt.Errorf("%s: no instructions", builder)
	}
	for i, in := range instrs {
		if in == nil || in.Payment == nil {
			return fmt.Errorf("%s: instruction %d has nil payment", builder, i)
		}
	}
	return nil
}

// ─────────────────────────────────────────────
// Party → XSD mapping helpers
// ─────────────────────────────────────────────

// partyIdentification maps a Party to PartyIdentification272. When p is nil,
// fallbackNm (usually a truncated account string) is used for the name so the
// mandatory element still emits.
func partyIdentification(p *Party, fallbackNm string) *PartyIdentification272 {
	out := &PartyIdentification272{Nm: fallbackNm}
	if p == nil {
		return out
	}
	if p.Name != "" {
		out.Nm = p.Name
	}
	out.PstlAdr = p.Address
	if p.ID != "" {
		othr := GenericOrganisationIdentification3{Id: p.ID}
		if p.IDScheme != "" {
			othr.SchmeNm = &OrganisationIdentificationSchemeName1Choice{Cd: p.IDScheme}
		}
		out.Id = &Party52Choice{
			OrgId: &OrganisationIdentification39{
				Othr: []GenericOrganisationIdentification3{othr},
			},
		}
	}
	if p.Phone != "" || p.Email != "" {
		out.CtctDtls = &Contact13{
			Nm:       p.Name,
			PhneNb:   p.Phone,
			EmailAdr: p.Email,
		}
	}
	return out
}

// partyAccount maps a Party's account fields to a CashAccount40. Precedence:
// IBAN, then Othr/Id, then Prxy. Othr/Id is Max34Text — a value that doesn't
// fit (e.g. a 56-char Stellar address, or any AcctProxyID) is placed in
// Prxy/Id (Max2048Text) instead, preserving the full identifier.
// Returns nil when the party has no account details at all.
func partyAccount(p *Party) *CashAccount40 {
	if p == nil {
		return nil
	}
	acct := &CashAccount40{}
	switch {
	case p.AcctIBAN != "":
		acct.Id = &AccountIdentification4Choice{IBAN: p.AcctIBAN}
	case p.AcctID != "" && len(p.AcctID) <= 34:
		acct.Id = &AccountIdentification4Choice{
			Othr: &GenericAccountIdentification1{Id: p.AcctID},
		}
	case p.AcctID != "" || p.AcctProxyID != "":
		id := p.AcctProxyID
		if id == "" {
			id = p.AcctID
		}
		acct.Prxy = &ProxyAccountIdentification1{Id: id}
		if p.AcctProxyTp != "" {
			acct.Prxy.Tp = &ProxyAccountType1Choice{Prtry: p.AcctProxyTp}
		}
	default:
		return nil
	}
	return acct
}

// endToEndID derives the EndToEndId for a payment: the transaction hash
// (truncated to the ISO 35-char limit is already handled by callers' use of
// safeTruncate) or the payment ID.
func endToEndID(p *models.Payment) string {
	if id := safeTruncate(p.TxHash, 16); id != "" {
		return id
	}
	return p.ID
}

// paymentAsset resolves the asset code, defaulting to XLM.
func paymentAsset(p *models.Payment) string {
	if p.Asset == "" {
		return "XLM"
	}
	return p.Asset
}

// remittanceInfo builds RmtInf from explicit lines, else from the payment
// memo (memo_type "id"/"hash"/"return" maps to a structured reference).
func remittanceInfo(lines []string, p *models.Payment) *RemittanceInformation2 {
	if len(lines) > 0 {
		return &RemittanceInformation2{Ustrd: lines}
	}
	if p.Memo == "" {
		return nil
	}
	switch p.MemoType {
	case "id", "hash", "return":
		return &RemittanceInformation2{
			Strd: []StructuredRemittanceInformation1{{
				RfrdDocInf: &ReferredDocumentInformation1{Nb: p.Memo},
			}},
		}
	default:
		return &RemittanceInformation2{Ustrd: []string{p.Memo}}
	}
}
