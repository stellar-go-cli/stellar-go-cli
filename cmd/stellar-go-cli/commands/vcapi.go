package commands

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/vc"
)

func newVCApiCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("vc-api", flag.ContinueOnError)
	port := fs.Int("port", 4000, "HTTP server port")

	return &Command{
		Name:  "vc-api",
		Short: "VC API server for W3C compliance testing",
		Long:  "Starts an HTTP server exposing /credentials/issue and /credentials/verify endpoints conforming to the W3C VC API specification.",
		Flags: fs,
		Run: func(cmd *Command, args []string) error {
			p := *port
			addr := fmt.Sprintf(":%d", p)

			svc, err := vc.NewService()
			if err != nil {
				return fmt.Errorf("failed to create DID service: %w", err)
			}

			mux := http.NewServeMux()
			mux.HandleFunc("/health", healthHandler)
			mux.HandleFunc("/credentials/issue", issueHandler(svc))
			mux.HandleFunc("/credentials/verify", verifyHandler(svc))

			ui.Info(fmt.Sprintf("VC API server starting on http://localhost:%d", p))
			ui.Info("Endpoints:")
			ui.Info("  POST /credentials/issue  - Issue a signed VC")
			ui.Info("  POST /credentials/verify - Verify a signed VC")
			ui.Info("  GET  /health            - Health check")

			server := &http.Server{
				Addr:              addr,
				Handler:           mux,
				ReadHeaderTimeout: 10 * time.Second,
			}

			return server.ListenAndServe()
		},
	}
}

// writeJSON encodes v as a JSON response body; headers must already be set.
func writeJSON(w http.ResponseWriter, v interface{}) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		ui.Error(fmt.Sprintf("vc-api: failed to encode response: %v", err))
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeJSON(w, map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	})
}

type issueRequest struct {
	Credential map[string]interface{} `json:"credential"`
	Options    map[string]interface{} `json:"options,omitempty"`
}

func issueHandler(svc *vc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			httpError(w, http.StatusBadRequest, "failed to read request body")
			return
		}
		defer r.Body.Close() //nolint:errcheck // best-effort close

		var req issueRequest
		if err := json.Unmarshal(body, &req); err != nil {
			httpError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
			return
		}

		if req.Credential == nil {
			httpError(w, http.StatusBadRequest, "missing 'credential' field")
			return
		}

		cred := req.Credential

		vcType := "VerifiableCredential"
		if typeVal, ok := cred["type"]; ok {
			switch t := typeVal.(type) {
			case string:
				vcType = t
			case []interface{}:
				if len(t) > 0 {
					if s, ok := t[len(t)-1].(string); ok {
						vcType = s
					}
				}
			}
		}

		subject, _ := cred["credentialSubject"].(map[string]interface{})
		if subject == nil {
			subject = map[string]interface{}{}
		}

		didDoc, err := svc.CreateDID(models.DIDMethodKey)
		if err != nil {
			httpError(w, http.StatusInternalServerError, fmt.Sprintf("DID creation failed: %v", err))
			return
		}
		didStr := didDoc.ID

		issued, err := svc.IssueVC(didStr, vcType, subject)
		if err != nil {
			httpError(w, http.StatusBadRequest, fmt.Sprintf("issuance failed: %v", err))
			return
		}

		if id, ok := cred["id"].(string); ok && id != "" {
			issued.ID = id
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, issued)
	}
}

type verifyRequest struct {
	VerifiableCredential models.VerifiableCredential `json:"verifiableCredential"`
	Options              map[string]interface{}      `json:"options,omitempty"`
}

type verifyResult struct {
	Checks []checkResult `json:"checks"`
}

type checkResult struct {
	Check  string `json:"check"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func verifyHandler(svc *vc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			httpError(w, http.StatusBadRequest, "failed to read request body")
			return
		}
		defer r.Body.Close() //nolint:errcheck // best-effort close

		var req verifyRequest
		if err := json.Unmarshal(body, &req); err != nil {
			httpError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
			return
		}

		if req.VerifiableCredential.ID == "" {
			httpError(w, http.StatusBadRequest, "missing 'verifiableCredential' field")
			return
		}

		valid, verr := svc.Verify(&req.VerifiableCredential)

		result := verifyResult{
			Checks: []checkResult{
				{
					Check:  "proof",
					Status: map[bool]string{true: "passed", false: "failed"}[valid],
				},
			},
		}

		if !valid && verr != nil {
			result.Checks[0].Error = verr.Error()
		}

		status := http.StatusOK
		if !valid {
			status = http.StatusBadRequest
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		writeJSON(w, result)
	}
}

func httpError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	writeJSON(w, map[string]interface{}{
		"error": message,
	})
}
