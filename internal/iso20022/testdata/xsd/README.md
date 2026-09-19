# ISO 20022 XSD schemas

Place the official XSDs here to enable schema validation in tests
(`TestXSDValidation_*` run `xmllint --noout --schema` and skip when absent).

Download from https://www.iso20022.org (catalogue → message → "XML schema").
Registration is required; redistribution terms prevent vendoring them into
this repo, so `*.xsd` in this directory is gitignored.

Expected filenames:

| File                  | Message                          |
|-----------------------|----------------------------------|
| `pacs.008.001.08.xsd` | FIToFICustomerCreditTransferV08  |
| `pacs.002.001.12.xsd` | FIToFIPaymentStatusReportV12     |
| `pacs.004.001.12.xsd` | PaymentReturnV12                 |
| `pacs.009.001.10.xsd` | FinancialInstitutionCreditTransferV10 |

Manual check:

```bash
mozartpay report iso20022 --type pacs.008 > /tmp/pacs008.xml
xmllint --noout --schema pacs.008.001.08.xsd /tmp/pacs008.xml
```
