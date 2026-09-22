# ISO 20022 XSD schemas

Place the official XSDs here to enable schema validation in tests
(`TestXSDValidation_*` run `xmllint --noout --schema` and skip when absent).

The schemas are checked into `messages/` at the repo root — run `make xsd`
to copy the four files used by the tests into this directory. They can also
be downloaded from https://www.iso20022.org (catalogue → message →
"XML schema"; registration required). `*.xsd` in this directory is
gitignored.

The schemas are ISO 20022 Registration Authority material reproduced under
the ISO 20022 Intellectual Property Right Policy — see `messages/README.md`
and the `NOTICE` file at the repository root for the attribution statement.

Expected filenames:

| File                     | Message                              |
|--------------------------|--------------------------------------|
| `pacs.008.001.14.xsd`    | FIToFICustomerCreditTransferV14      |
| `pacs.002.001.16.xsd`    | FIToFIPaymentStatusReportV16         |
| `pacs.004.001.15.xsd`    | PaymentReturnV15                     |
| `pacs.009.001.13.xsd`    | FinancialInstitutionCreditTransferV13|
| `pain.001.001.13.xsd`    | CustomerCreditTransferInitiationV13  |
| `camt.054.001.14.xsd`    | BankToCustomerDebitCreditNotificationV14 |

Manual check:

```bash
stellar-go-cli report iso20022 --type pacs.008 > /tmp/pacs008.xml
xmllint --noout --schema pacs.008.001.14.xsd /tmp/pacs008.xml
```
