# ISO 20022 XSD schemas

Place the official XSDs here to enable schema validation in tests
(`TestXSDValidation_*` run `xmllint --noout --schema` and skip when absent).

The schemas are checked into `messages/` at the repo root — run `make xsd`
to copy the four files used by the tests into this directory. They can also
be downloaded from https://www.iso20022.org (catalogue → message →
"XML schema"; registration required). `*.xsd` in this directory is
gitignored.

Expected filenames:

| File                  | Message                          |
|-----------------------|----------------------------------|
| `pacs.008.001.14.xsd` | FIToFICustomerCreditTransferV14  |
| `pacs.002.001.16.xsd` | FIToFIPaymentStatusReportV16     |
| `pacs.004.001.15.xsd` | PaymentReturnV15                 |
| `pacs.009.001.13.xsd` | FinancialInstitutionCreditTransferV13 |

Manual check:

```bash
stellar-go-cli report iso20022 --type pacs.008 > /tmp/pacs008.xml
xmllint --noout --schema pacs.008.001.14.xsd /tmp/pacs008.xml
```
