package iso20022

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
)

func demoInstruction(i int) *CreditTransferInstruction {
	p := demoPayment()
	p.ID = fmt.Sprintf("disb-pay-%d", i)
	p.Amount = []string{"10.0000000", "20.5000000", "5.2500000"}[i%3]
	return &CreditTransferInstruction{Payment: p}
}

// ─────────────────────────────────────────────
// pain.001
// ─────────────────────────────────────────────

func TestBuildPain001(t *testing.T) {
	instrs := []*CreditTransferInstruction{
		demoInstruction(0),
		demoInstruction(1),
		{
			Payment: func() *models.Payment {
				p := demoPayment()
				p.ID = "disb-pay-3"
				p.Amount = "4.7500000"
				return p
			}(),
			Creditor: &Party{
				Name:        "Amina Diallo",
				Phone:       "+221-771234567",
				Email:       "amina@example.org",
				AcctProxyID: "+221-771234567",
				AcctProxyTp: "PHONE",
			},
			RemittanceInfo: []string{"Cash transfer — October"},
		},
	}

	xmlStr, err := BuildPain001(instrs, &Pain001Options{
		InitiatingParty: &Party{Name: "Relief Org", ID: "ORG-123"},
		Debtor: &Party{
			Name:     "Relief Org",
			AcctID:   "GDEBTOR_ACCT0000000000000000000000000000000000000000",
			AgentBIC: "DEUTDEFF",
		},
	})
	if err != nil {
		t.Fatalf("BuildPain001 failed: %v", err)
	}

	for _, want := range []string{
		"pain.001.001.13", "CstmrCdtTrfInitn", "PmtInf", "CdtTrfTxInf",
		"<NbOfTxs>3</NbOfTxs>", "<CtrlSum>35.25</CtrlSum>",
		"<InitgPty>", "Relief Org", "<ReqdExctnDt>",
		"Amina Diallo", "+221-771234567", "CtctDtls", "Prxy",
		"Cash transfer — October",
	} {
		if !strings.Contains(xmlStr, want) {
			t.Errorf("XML does not contain %s", want)
		}
	}
	if err := ValidateXML([]byte(xmlStr)); err != nil {
		t.Errorf("XML is not well-formed: %v", err)
	}
	if got := strings.Count(xmlStr, "<CdtTrfTxInf>"); got != 3 {
		t.Errorf("expected 3 CdtTrfTxInf, got %d", got)
	}
}

func TestBuildPain001Order(t *testing.T) {
	xmlStr, err := BuildPain001([]*CreditTransferInstruction{demoInstruction(0)}, &Pain001Options{
		Debtor: &Party{AgentBIC: "DEUTDEFF", AcctID: "GABC"},
	})
	if err != nil {
		t.Fatalf("BuildPain001 failed: %v", err)
	}
	assertChildOrder(t, xmlStr, "Document/CstmrCdtTrfInitn/GrpHdr", []string{
		"MsgId", "CreDtTm", "Authstn", "NbOfTxs", "CtrlSum", "InitgPty",
		"FwdgAgt", "InitnSrc",
	})
	assertChildOrder(t, xmlStr, "Document/CstmrCdtTrfInitn/PmtInf", []string{
		"PmtInfId", "PmtMtd", "ReqdAdvcTp", "BtchBookg", "NbOfTxs", "CtrlSum",
		"PmtTpInf", "ReqdExctnDt", "PoolgAdjstmntDt", "Dbtr", "DbtrAcct",
		"DbtrAgt", "DbtrAgtAcct", "InstrForDbtrAgt", "UltmtDbtr", "ChrgBr",
		"ChrgsAcct", "ChrgsAcctAgt", "CdtTrfTxInf",
	})
	assertChildOrder(t, xmlStr, "Document/CstmrCdtTrfInitn/PmtInf/CdtTrfTxInf", []string{
		"PmtId", "PmtTpInf", "Amt", "XchgRateInf", "ChrgBr", "MndtRltdInf",
		"ChqInstr", "UltmtDbtr",
		"IntrmyAgt1", "IntrmyAgt1Acct", "IntrmyAgt2", "IntrmyAgt2Acct",
		"IntrmyAgt3", "IntrmyAgt3Acct",
		"CdtrAgt", "CdtrAgtAcct", "Cdtr", "CdtrAcct", "UltmtCdtr",
		"InstrForCdtrAgt", "InstrForDbtrAgt", "Purp", "RgltryRptg", "Tax",
		"RltdRmtInf", "RmtInf", "SplmtryData",
	})
}

func TestBuildPain001Empty(t *testing.T) {
	if _, err := BuildPain001(nil, nil); err == nil {
		t.Error("expected error for empty batch")
	}
	if _, err := BuildPain001([]*CreditTransferInstruction{{}}, nil); err == nil {
		t.Error("expected error for nil payment")
	}
}

func TestXSDValidation_Pain001(t *testing.T) {
	instrs := []*CreditTransferInstruction{
		demoInstruction(0),
		{
			Payment: func() *models.Payment {
				p := demoPayment()
				p.ID = "disb-pay-2"
				p.Amount = "7.0000000"
				return p
			}(),
			Creditor: &Party{
				Name:        "Fatou Ndiaye",
				Phone:       "+221-770001122",
				AcctProxyID: "+221-770001122",
				AcctProxyTp: "PHONE",
			},
		},
	}
	xmlStr, err := BuildPain001(instrs, &Pain001Options{
		InitiatingParty: &Party{Name: "Relief Org", ID: "ORG-123"},
		Debtor: &Party{
			Name:     "Relief Org",
			AcctID:   "GDEBTOR_ACCT0000000000000000000000000000000000000000",
			AgentBIC: "DEUTDEFF",
		},
	})
	if err != nil {
		t.Fatalf("BuildPain001 failed: %v", err)
	}
	validateWithXSD(t, xmlStr, "pain.001.001.13.xsd")
}

// ─────────────────────────────────────────────
// pacs.008 batch
// ─────────────────────────────────────────────

func TestBuildPacs008Batch(t *testing.T) {
	instrs := []*CreditTransferInstruction{
		demoInstruction(0),
		demoInstruction(1),
		demoInstruction(2),
	}
	xmlStr, err := BuildPacs008Batch(instrs, nil)
	if err != nil {
		t.Fatalf("BuildPacs008Batch failed: %v", err)
	}
	for _, want := range []string{
		"pacs.008.001.14", "<NbOfTxs>3</NbOfTxs>",
		"<CtrlSum>35.75</CtrlSum>", "TtlIntrBkSttlmAmt", `Ccy="XXX"`,
	} {
		if !strings.Contains(xmlStr, want) {
			t.Errorf("XML does not contain %s", want)
		}
	}
	if got := strings.Count(xmlStr, "<CdtTrfTxInf>"); got != 3 {
		t.Errorf("expected 3 CdtTrfTxInf, got %d", got)
	}
	if err := ValidateXML([]byte(xmlStr)); err != nil {
		t.Errorf("XML is not well-formed: %v", err)
	}
}

func TestBuildPacs008BatchMixedCurrency(t *testing.T) {
	i1 := demoInstruction(0)
	i2 := demoInstruction(1)
	i2.Payment.Asset = "USD"
	xmlStr, err := BuildPacs008Batch([]*CreditTransferInstruction{i1, i2}, nil)
	if err != nil {
		t.Fatalf("BuildPacs008Batch failed: %v", err)
	}
	// Mixed currencies: no TtlIntrBkSttlmAmt, but CtrlSum still sums.
	assertAbsent(t, xmlStr, "TtlIntrBkSttlmAmt")
	if !strings.Contains(xmlStr, "<CtrlSum>30.5</CtrlSum>") {
		t.Error("expected CtrlSum=30.5")
	}
}

func TestXSDValidation_Pacs008Batch(t *testing.T) {
	instrs := []*CreditTransferInstruction{demoInstruction(0), demoInstruction(1)}
	xmlStr, err := BuildPacs008Batch(instrs, &Pacs008BatchOptions{
		Pacs008Options: Pacs008Options{DbtrBIC: "DEUTDEFF", CdtrBIC: "BNPAFRPP"},
	})
	if err != nil {
		t.Fatalf("BuildPacs008Batch failed: %v", err)
	}
	validateWithXSD(t, xmlStr, "pacs.008.001.14.xsd")
}

// ─────────────────────────────────────────────
// pacs.002 batch
// ─────────────────────────────────────────────

func TestBuildPacs002Batch(t *testing.T) {
	i1 := demoInstruction(0)
	i2 := demoInstruction(1)
	i2.TxStatus = TxStsRJCT
	i2.TxReason = "AC01"
	xmlStr, err := BuildPacs002Batch([]*CreditTransferInstruction{i1, i2}, nil)
	if err != nil {
		t.Fatalf("BuildPacs002Batch failed: %v", err)
	}
	if got := strings.Count(xmlStr, "<TxInfAndSts>"); got != 2 {
		t.Errorf("expected 2 TxInfAndSts, got %d", got)
	}
	if !strings.Contains(xmlStr, "<TxSts>ACSC</TxSts>") {
		t.Error("default status ACSC missing")
	}
	if !strings.Contains(xmlStr, "<TxSts>RJCT</TxSts>") {
		t.Error("per-tx status RJCT missing")
	}
	if !strings.Contains(xmlStr, "<Cd>AC01</Cd>") {
		t.Error("per-tx reason AC01 missing")
	}
	if err := ValidateXML([]byte(xmlStr)); err != nil {
		t.Errorf("XML is not well-formed: %v", err)
	}
}

func TestXSDValidation_Pacs002Batch(t *testing.T) {
	xmlStr, err := BuildPacs002Batch([]*CreditTransferInstruction{demoInstruction(0)}, nil)
	if err != nil {
		t.Fatalf("BuildPacs002Batch failed: %v", err)
	}
	validateWithXSD(t, xmlStr, "pacs.002.001.16.xsd")
}

// ─────────────────────────────────────────────
// camt.054
// ─────────────────────────────────────────────

func TestBuildCamt054(t *testing.T) {
	instrs := []*CreditTransferInstruction{
		demoInstruction(0),
		{
			Payment: func() *models.Payment {
				p := demoPayment()
				p.ID = "disb-pay-2"
				return p
			}(),
			Creditor: &Party{Name: "Fatou Ndiaye"},
		},
	}
	xmlStr, err := BuildCamt054(instrs, &Camt054Options{
		Account: &Party{Name: "Relief Org", AcctID: "GDEBTOR_ACCT0000000000000000000000000000000000000000"},
	})
	if err != nil {
		t.Fatalf("BuildCamt054 failed: %v", err)
	}
	for _, want := range []string{
		"camt.054.001.14", "BkToCstmrDbtCdtNtfctn", "<Ntfctn>", "<Ntry>",
		"<CdtDbtInd>DBIT</CdtDbtInd>", "<Cd>BOOK</Cd>", "<BkTxCd>",
		"<Cd>PMNT</Cd>", "<NtryDtls>", "<TxDtls>", "<Refs>", "<RltdPties>",
		"Fatou Ndiaye",
	} {
		if !strings.Contains(xmlStr, want) {
			t.Errorf("XML does not contain %s", want)
		}
	}
	if got := strings.Count(xmlStr, "<Ntry>"); got != 2 {
		t.Errorf("expected 2 Ntry, got %d", got)
	}
	if err := ValidateXML([]byte(xmlStr)); err != nil {
		t.Errorf("XML is not well-formed: %v", err)
	}
}

func TestBuildCamt054Order(t *testing.T) {
	xmlStr, err := BuildCamt054([]*CreditTransferInstruction{demoInstruction(0)}, nil)
	if err != nil {
		t.Fatalf("BuildCamt054 failed: %v", err)
	}
	assertChildOrder(t, xmlStr, "Document/BkToCstmrDbtCdtNtfctn/Ntfctn/Ntry", []string{
		"NtryRef", "Amt", "CdtDbtInd", "RvslInd", "Sts", "BookgDt", "ValDt",
		"AcctSvcrRef", "Avlbty", "BkTxCd", "ComssnWvrInd", "AddtlInfInd",
		"AmtDtls", "Chrgs", "TechInptChanl", "Intrst", "CardTx", "NtryDtls",
		"AddtlNtryInf",
	})
	assertChildOrder(t, xmlStr, "Document/BkToCstmrDbtCdtNtfctn/Ntfctn/Ntry/NtryDtls/TxDtls", []string{
		"Refs", "Amt", "CdtDbtInd", "AmtDtls", "Avlbty", "BkTxCd", "Chrgs",
		"Intrst", "RltdPties", "RltdAgts", "LclInstrm", "PmtTpInf", "Purp",
		"RltdRmtInf", "RmtInf", "RltdDts", "RltdPric", "RltdQties",
		"FinInstrmId", "Tax", "RtrInf", "RltdCorpActn", "SfkpgAcct",
		"UndrlygAllcn", "CshDpst", "CardTx", "InstrCpy", "AddtlTxInf",
		"SplmtryData",
	})
}

func TestXSDValidation_Camt054(t *testing.T) {
	xmlStr, err := BuildCamt054([]*CreditTransferInstruction{demoInstruction(0)}, &Camt054Options{
		Account: &Party{AcctID: "GDEBTOR_ACCT0000000000000000000000000000000000000000"},
	})
	if err != nil {
		t.Fatalf("BuildCamt054 failed: %v", err)
	}
	validateWithXSD(t, xmlStr, "camt.054.001.14.xsd")
}

// ─────────────────────────────────────────────
// MemoType / AssetIssuer propagation
// ─────────────────────────────────────────────

func TestAssetIssuerPropagation(t *testing.T) {
	p := demoPayment()
	p.Asset = "USDC"
	p.AssetIssuer = "GISSUER000000000000000000000000000000000000000000000001"
	xmlStr, err := BuildPacs008(p, nil)
	if err != nil {
		t.Fatalf("BuildPacs008 failed: %v", err)
	}
	if !strings.Contains(xmlStr, "<Issr>GISSUER") {
		t.Error("expected asset issuer in SplmtryData")
	}
}

func TestMemoTypeStructured(t *testing.T) {
	p := demoPayment()
	p.Memo = "INV-2026-0042"
	p.MemoType = "id"
	xmlStr, err := BuildPacs008(p, nil)
	if err != nil {
		t.Fatalf("BuildPacs008 failed: %v", err)
	}
	if !strings.Contains(xmlStr, "<Strd>") || !strings.Contains(xmlStr, "INV-2026-0042") {
		t.Error("expected structured remittance for memo_type=id")
	}
	if strings.Contains(xmlStr, "<Ustrd>") {
		t.Error("Ustrd must not be emitted for memo_type=id")
	}
}

func TestBuildPain001ReceiverProxyAccount(t *testing.T) {
	p := demoPayment()
	instr := &CreditTransferInstruction{
		Payment: p,
		Creditor: &Party{
			Name:        "Moussa Sow",
			AcctProxyID: "moussa.wallet",
			AcctProxyTp: "WALLET",
		},
	}
	xmlStr, err := BuildPain001([]*CreditTransferInstruction{instr}, &Pain001Options{
		Debtor: &Party{AcctID: "GABC", AgentBIC: "DEUTDEFF"},
	})
	if err != nil {
		t.Fatalf("BuildPain001 failed: %v", err)
	}
	if !strings.Contains(xmlStr, "<Prxy>") || !strings.Contains(xmlStr, "moussa.wallet") {
		t.Error("expected proxy account for wallet receiver")
	}
	if !strings.Contains(xmlStr, "<Prtry>WALLET</Prtry>") {
		t.Error("expected WALLET proxy type")
	}
}

func TestSumAmounts(t *testing.T) {
	sum, err := sumAmounts([]string{"10.0000000", "20.5", "0.25000"})
	if err != nil {
		t.Fatalf("sumAmounts: %v", err)
	}
	if sum != "30.75" {
		t.Errorf("got %q, want 30.75", sum)
	}
	if _, err := sumAmounts([]string{"not-a-number"}); err == nil {
		t.Error("expected error for invalid amount")
	}
}
