package iso20022

import (
	"bytes"
	"encoding/xml"
	"fmt"
)

// MarshalXML marshals an ISO 20022 document struct to indented XML with declaration
func MarshalXML(doc interface{}, namespace string) (string, error) {
	data, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("XML marshal: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	buf.Write(data)
	buf.WriteString("\n")

	return buf.String(), nil
}

// UnmarshalXML parses XML data into the provided struct
func UnmarshalXML(data []byte, v interface{}) error {
	return xml.Unmarshal(data, v)
}

// ValidateXML parses the XML to verify it is well-formed
func ValidateXML(data []byte) error {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		_, err := dec.Token()
		if err != nil {
			if err.Error() == "EOF" {
				return nil
			}
			return fmt.Errorf("XML validation: %w", err)
		}
	}
}

// MessageType constants for dispatch
const (
	MsgPacs008 = "pacs.008"
	MsgPacs002 = "pacs.002"
	MsgPacs004 = "pacs.004"
	MsgPacs009 = "pacs.009"
)
