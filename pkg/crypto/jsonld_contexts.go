package crypto

import (
	"embed"
	"encoding/json"

	"github.com/piprate/json-gold/ld"
)

// jsonld_contexts.go — preloads the immutable W3C/W3ID JSON-LD contexts this
// codebase's VC/DID documents reference, so JSON-LD expansion/normalization
// never needs the network (offline-safe; avoids w3.org rate limiting).
//
// The fetched documents live under contexts/ — they are immutable published
// context definitions, safe to vendor.

//go:embed contexts/*.json
var jsonLdContextFS embed.FS

// jsonLdDocLoader is the shared document loader for all JSON-LD processing:
// well-known contexts resolve from the embedded copies; anything else falls
// through to an in-process caching remote loader.
var jsonLdDocLoader = newJSONLdDocLoader()

// #nosec G101 -- public immutable W3C context definitions, not credentials
var jsonLdContextURLs = map[string]string{
	"https://www.w3.org/ns/credentials/v2":             "contexts/credentials-v2.json",
	"https://www.w3.org/ns/credentials/examples/v2":    "contexts/credentials-examples-v2.json",
	"https://w3id.org/security/suites/ed25519-2020/v1": "contexts/ed25519-2020-v1.json",
	"https://w3id.org/security/suites/jws-2020/v1":     "contexts/jws-2020-v1.json",
	"https://www.w3.org/ns/did/v1":                     "contexts/did-v1.json",
}

func newJSONLdDocLoader() *ld.CachingDocumentLoader {
	loader := ld.NewCachingDocumentLoader(ld.NewDefaultDocumentLoader(nil))
	for u, file := range jsonLdContextURLs {
		data, err := jsonLdContextFS.ReadFile(file)
		if err != nil {
			continue
		}
		var doc interface{}
		if err := json.Unmarshal(data, &doc); err != nil {
			continue
		}
		loader.AddDocument(u, doc)
	}
	return loader
}

// JSONLdDocumentLoader exposes the shared loader for processors that need it.
func JSONLdDocumentLoader() ld.DocumentLoader { return jsonLdDocLoader }

// jsonLdProcessorOpts returns options bound to the shared context loader.
func jsonLdProcessorOpts() *ld.JsonLdOptions {
	opts := ld.NewJsonLdOptions("")
	opts.DocumentLoader = jsonLdDocLoader
	return opts
}
