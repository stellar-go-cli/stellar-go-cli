// Package iso20022 generates ISO 20022 payment messages as XML from Stellar
// payment data.
//
// Supported messages:
//
//   - pacs.008.001.14 — FIToFICustomerCreditTransfer (single and batch)
//   - pacs.002.001.16 — FIToFIPaymentStatusReport (single and batch)
//   - pacs.004.001.15 — PaymentReturn
//   - pacs.009.001.13 — FinancialInstitutionCreditTransfer
//   - pain.001.001.13 — CustomerCreditTransferInitiation (disbursement batch)
//   - camt.054.001.14 — BankToCustomerDebitCreditNotification (settlement evidence)
//
// Each message is produced by a BuildPacsXXX/BuildPainXXX/BuildCamtXXX
// function that maps a models.Payment (or a CreditTransferInstruction batch)
// onto the corresponding ISO 20022 document type and marshals it with
// encoding/xml. Output validates against the official XSDs published at
// https://www.iso20022.org (see testdata/xsd/README.md).
package iso20022
