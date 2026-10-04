package iso20022_test

import (
	"fmt"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/iso20022"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
)

// Mirrored from stellar-disbursement-platform-backend's internal/data models —
// the fields a disbursement payment carries. The example maps them onto
// CreditTransferInstruction without importing SDP's module (which would pull
// its full dependency tree).
type sdpPayment struct {
	ID                   string
	Amount               string
	StellarTransactionID string
	Status               string
	SenderAddress        string
	ReceiverWallet       *sdpReceiverWallet
	CreatedAt            time.Time
}

type sdpReceiverWallet struct {
	StellarAddress string
	Receiver       *sdpReceiver
}

type sdpReceiver struct {
	PhoneNumber string
	Email       string
	ExternalID  string
}

// toInstruction maps an SDP payment to a CreditTransferInstruction. The
// receiver's wallet address becomes the creditor's proxy account; phone/email
// travel as creditor contact details for the receiving institution.
func toInstruction(p *sdpPayment) *iso20022.CreditTransferInstruction {
	instr := &iso20022.CreditTransferInstruction{
		Payment: &models.Payment{
			ID:        p.ID,
			From:      p.SenderAddress,
			To:        p.ReceiverWallet.StellarAddress,
			Amount:    p.Amount,
			Asset:     "USDC",
			TxHash:    p.StellarTransactionID, // becomes OrgnlEndToEndId
			CreatedAt: p.CreatedAt,
		},
	}
	if r := p.ReceiverWallet.Receiver; r != nil {
		instr.Creditor = &iso20022.Party{
			Phone: r.PhoneNumber,
			Email: r.Email,
		}
		if r.PhoneNumber != "" {
			instr.Creditor.AcctProxyID = r.PhoneNumber
			instr.Creditor.AcctProxyTp = "PHONE"
		}
		if r.ExternalID != "" {
			instr.RemittanceInfo = []string{"ext:" + r.ExternalID}
		}
	}
	return instr
}

// Example of the full disbursement loop a platform like SDP runs:
//
//	sent payments → pain.001 file → bank → pain.002 status report →
//	reconcile against the sent batch → update payment statuses
func Example_sdpReconciliationLoop() {
	payments := []*sdpPayment{
		{ID: "pay-001", Amount: "25.0000000", SenderAddress: "GORG_TREASURY",
			StellarTransactionID: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
			CreatedAt:            time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			ReceiverWallet: &sdpReceiverWallet{
				StellarAddress: "GRECEIVER_1",
				Receiver:       &sdpReceiver{PhoneNumber: "+221770001122", ExternalID: "r-1001"},
			}},
		{ID: "pay-002", Amount: "25.0000000", SenderAddress: "GORG_TREASURY",
			StellarTransactionID: "ffee00112233445566778899aabbccddeeff00112233445566778899aabbccdd",
			CreatedAt:            time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			ReceiverWallet: &sdpReceiverWallet{
				StellarAddress: "GRECEIVER_2",
				Receiver:       &sdpReceiver{Email: "r2@example.org"},
			}},
	}
	instrs := make([]*iso20022.CreditTransferInstruction, len(payments))
	for i, p := range payments {
		instrs[i] = toInstruction(p)
	}

	// 1. Emit the disbursement file for the bank/agent.
	pain001, err := iso20022.BuildPain001(instrs, &iso20022.Pain001Options{
		MsgID:           "SGC1P-BATCH7",
		InitiatingParty: &iso20022.Party{Name: "Relief Org"},
		Debtor:          &iso20022.Party{AcctID: "GORG_TREASURY", AgentBIC: "DEUTDEFF"},
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(len(pain001) > 0)

	// 2. The bank replies with a pain.002 — simulate it here (mark pay-002
	// rejected), then parse as a consumer would.
	instrs[1].TxStatus = iso20022.TxStsRJCT
	instrs[1].TxReason = "AC01"
	pain002, _ := iso20022.BuildPain002Batch(instrs, &iso20022.Pain002Options{OrgnlMsgId: "SGC1P-BATCH7"})

	if _, err := iso20022.DetectMessageType([]byte(pain002)); err != nil {
		panic(err)
	}
	doc, err := iso20022.ParsePain002([]byte(pain002))
	if err != nil {
		panic(err)
	}

	// 3. Reconcile: map ISO statuses onto the platform's state machine.
	sum := iso20022.ReconcileInstructions(instrs, doc.StatusReports())
	for _, r := range sum.Results {
		switch {
		case !r.Matched:
			fmt.Printf("%s → still pending (no report)\n", r.PaymentID)
		case r.Status == string(iso20022.TxStsACSC):
			fmt.Printf("%s → success\n", r.PaymentID)
		case r.Status == string(iso20022.TxStsRJCT):
			fmt.Printf("%s → failed (%s)\n", r.PaymentID, r.ReasonCode)
		default:
			fmt.Printf("%s → %s\n", r.PaymentID, r.Status)
		}
	}

	// Output:
	// true
	// pay-001 → success
	// pay-002 → failed (AC01)
}
