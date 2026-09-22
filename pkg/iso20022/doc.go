// Package iso20022 generates ISO 20022 payment messages (pacs family) as XML
// from Stellar payment data.
//
// Supported messages:
//
//   - pacs.008.001.14 — FIToFICustomerCreditTransfer
//   - pacs.002.001.16 — FIToFIPaymentStatusReport
//   - pacs.004.001.15 — PaymentReturn
//   - pacs.009.001.13 — FinancialInstitutionCreditTransfer
//
// Each message is produced by a BuildPacsXXX function that maps a
// models.Payment onto the corresponding ISO 20022 document type and marshals
// it with encoding/xml. Output validates against the official XSDs published
// at https://www.iso20022.org (see testdata/xsd/README.md).
package iso20022
