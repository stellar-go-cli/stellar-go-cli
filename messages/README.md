# ISO 20022 message schemas

Official XML schemas (XSDs) published by the ISO 20022 Registration
Authority at https://www.iso20022.org (catalogue → message → "XML schema").

`make xsd` copies the six schemas exercised by `TestXSDValidation_*` into
`pkg/iso20022/testdata/xsd/`; the remaining files are kept for reference and
future message types.

| File | Message family |
|------|----------------|
| `camt.054.001.14.xsd` | Bank-to-Customer Debit/Credit Notification (used by tests) |
| `pain.001.001.13.xsd` | Customer Credit Transfer Initiation (used by tests) |
| `pacs.002.001.12.xsd` | FI-to-FI Payment Status Report |
| `pacs.002.001.16.xsd` | FI-to-FI Payment Status Report (used by tests) |
| `pacs.003.001.12.xsd` | FI-to-FI Customer Direct Debit |
| `pacs.004.001.15.xsd` | Payment Return (used by tests) |
| `pacs.007.001.14.xsd` | FI-to-FI Payment Reversal |
| `pacs.008.001.14.xsd` | FI-to-FI Customer Credit Transfer (used by tests) |
| `pacs.009.001.13.xsd` | Financial Institution Credit Transfer (used by tests) |
| `pacs.010.001.06.xsd` | Financial Institution Direct Debit |
| `pacs.028.001.07.xsd` | FI-to-FI Payment Status Request |
| `pacs.029.001.02.xsd` | Resolution of Investigation |

## Attribution

These files are reproduced under the ISO 20022 Intellectual Property Right
Policy, which permits free use and reproduction by all interested users.
This repository is not the official ISO 20022 site — the sole source of
up-to-date materials and information on ISO 20022 message standards and the
Repository is https://www.iso20022.org/. See the `NOTICE` file at the
repository root.
