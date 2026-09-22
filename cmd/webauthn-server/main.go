package main

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

// WebAuthnChallenge represents the authentication challenge
type WebAuthnChallenge struct {
	Challenge   string `json:"challenge"`
	UserID      string `json:"userId"`
	UserName    string `json:"userName"`
	DisplayName string `json:"displayName"`
	RPName      string `json:"rpName"`
	RPID        string `json:"rpId"`
	CallbackURL string `json:"callbackUrl"`
}

// WebAuthnResponse represents the response from WebAuthn
type WebAuthnResponse struct {
	ID       string `json:"id"`
	RawID    string `json:"rawId"`
	Type     string `json:"type"`
	Response struct {
		ClientDataJSON    string `json:"clientDataJSON"`
		AttestationObject string `json:"attestationObject"`
	} `json:"response"`
}

// CredentialInfo represents the created credential
type CredentialInfo struct {
	CredentialID string    `json:"credentialId"`
	PublicKey    string    `json:"publicKey"`
	Algorithm    string    `json:"algorithm"`
	Origin       string    `json:"origin"`
	CreatedAt    time.Time `json:"createdAt"`
}

var (
	store = make(map[string]*CredentialInfo)
)

// renderTemplate parses and renders an HTML template. Parse failures return
// a 500 before any body is written; execute failures are logged since the
// response may already be partially committed.
func renderTemplate(w http.ResponseWriter, name, tmpl string, data interface{}) {
	t, err := template.New(name).Parse(tmpl)
	if err != nil {
		http.Error(w, "internal template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	if err := t.Execute(w, data); err != nil {
		log.Printf("execute template %q: %v", name, err)
	}
}

// writeJSON encodes v as a JSON response, logging encoding errors.
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode JSON response: %v", err)
	}
}

func main() {
	port := getPort()
	if port == "" {
		port = "8000"
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)
	r.Use(middleware.AllowContentType("application/json"))

	// Routes
	r.Get("/", homeHandler)
	r.Get("/health", healthHandler)
	r.Get("/webauthn/auth", webAuthnAuthHandler)
	r.Post("/webauthn/auth", webAuthnVerifyHandler)
	r.Get("/webauthn/sign", webAuthnSignHandler)
	r.Post("/webauthn/sign", webAuthnSignVerifyHandler)
	r.Get("/callback", callbackHandler)

	log.Printf("🚀 WebAuthn server starting on port %s", port)
	log.Printf("📡 Health check: http://localhost:%s/health", port)
	log.Printf("🔐 Auth endpoint: http://localhost:%s/webauthn/auth", port)

	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("❌ Failed to start server: %v", err)
	}
}

func getPort() string {
	if port := os.Getenv("PORT"); port != "" {
		return port
	}
	return "8000"
}

// corsMiddleware adds CORS headers to allow requests from any origin
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <title>Stellar Go CLI WebAuthn Server</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; background: #f5f5f5; }
        .container { max-width: 600px; margin: 0 auto; background: white; padding: 30px; border-radius: 10px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        h1 { color: #333; text-align: center; }
        .status { background: #e8f5e8; padding: 15px; border-radius: 5px; margin: 20px 0; }
        .endpoint { background: #f0f0f0; padding: 10px; border-radius: 5px; font-family: monospace; margin: 10px 0; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔐 Stellar Go CLI WebAuthn Server</h1>
        <div class="status">
            <strong>✅ Server is running</strong><br>
            Ready to handle WebAuthn passkey authentication
        </div>
        <h3>Available Endpoints:</h3>
        <div class="endpoint">GET /health - Health check</div>
        <div class="endpoint">GET /webauthn/auth - Start WebAuthn ceremony</div>
        <div class="endpoint">POST /webauthn/auth - Verify WebAuthn response</div>
        <div class="endpoint">GET /callback - OAuth callback handler</div>
    </div>
</body>
</html>`
	renderTemplate(w, "home", tmpl, nil)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().UTC(),
		"server":    "Stellar Go CLI WebAuthn Server",
		"version":   "1.0.0",
	})
}

func webAuthnAuthHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	challenge := r.URL.Query().Get("challenge")
	userID := r.URL.Query().Get("userId")
	userName := r.URL.Query().Get("userName")
	displayName := r.URL.Query().Get("displayName")
	rpName := r.URL.Query().Get("rpName")
	rpID := r.URL.Query().Get("rpId")
	callbackURL := r.URL.Query().Get("callback")

	if challenge == "" || userID == "" || userName == "" {
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
		return
	}

	// Store challenge for later verification
	_ = &WebAuthnChallenge{
		Challenge:   challenge,
		UserID:      userID,
		UserName:    userName,
		DisplayName: displayName,
		RPName:      rpName,
		RPID:        rpID,
		CallbackURL: callbackURL,
	}

	// Generate a session ID
	sessionID := uuid.New().String()
	store[sessionID] = &CredentialInfo{
		CredentialID: sessionID,
		CreatedAt:    time.Now(),
	}

	// Serve WebAuthn authentication page
	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <title>WebAuthn Authentication - Stellar Go CLI</title>
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; padding: 20px; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); min-height: 100vh; display: flex; align-items: center; justify-content: center; }
        .container { max-width: 500px; width: 100%; background: white; padding: 40px; border-radius: 20px; box-shadow: 0 20px 40px rgba(0,0,0,0.1); text-align: center; }
        .logo { font-size: 48px; margin-bottom: 20px; }
        h1 { color: #333; margin-bottom: 10px; font-size: 28px; }
        .subtitle { color: #666; margin-bottom: 30px; font-size: 16px; }
        .user-info { background: #f8f9fa; padding: 20px; border-radius: 10px; margin: 20px 0; text-align: left; }
        .user-info h3 { margin: 0 0 10px 0; color: #333; }
        .user-info p { margin: 5px 0; color: #666; font-size: 14px; }
        .btn { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; border: none; padding: 15px 30px; border-radius: 10px; font-size: 16px; cursor: pointer; margin: 20px 0; transition: transform 0.2s; width: 100%; }
        .btn:hover { transform: translateY(-2px); }
        .btn:disabled { background: #ccc; cursor: not-allowed; transform: none; }
        .status { margin: 20px 0; padding: 15px; border-radius: 10px; }
        .status.success { background: #d4edda; color: #155724; }
        .status.error { background: #f8d7da; color: #721c24; }
        .status.info { background: #d1ecf1; color: #0c5460; }
        .spinner { border: 3px solid #f3f3f3; border-top: 3px solid #667eea; border-radius: 50%; width: 40px; height: 40px; animation: spin 1s linear infinite; margin: 20px auto; }
        @keyframes spin { 0% { transform: rotate(0deg); } 100% { transform: rotate(360deg); } }
        .hidden { display: none; }
    </style>
</head>
<body>
    <div class="container">
        <div class="logo">🔐</div>
        <h1>WebAuthn Authentication</h1>
        <p class="subtitle">Create your secure passkey for {{.RPName}}</p>
        
        <div class="user-info">
            <h3>User Information</h3>
            <p><strong>Name:</strong> {{.DisplayName}}</p>
            <p><strong>Email:</strong> {{.UserName}}</p>
            <p><strong>Service:</strong> {{.RPName}}</p>
        </div>
        
        <div id="status" class="status info">
            Ready to create your passkey. Click below to begin.
        </div>
        
        <button id="createBtn" class="btn" onclick="createPasskey()">
            🔑 Create Passkey
        </button>
        
        <div id="loading" class="hidden">
            <div class="spinner"></div>
            <p>Creating passkey... Please use your fingerprint or security key.</p>
        </div>
        
        <div id="success" class="status success hidden">
            ✅ Passkey created successfully! Redirecting...
        </div>
        
        <div id="error" class="status error hidden">
            ❌ Failed to create passkey. Please try again.
        </div>
    </div>

    <script>
        const challenge = "{{.Challenge}}";
        const userId = "{{.UserID}}";
        const userName = "{{.UserName}}";
        const displayName = "{{.DisplayName}}";
        const rpName = "{{.RPName}}";
        const rpId = "{{.RPID}}";
        const callbackUrl = "{{.CallbackURL}}";

        async function createPasskey() {
            const btn = document.getElementById('createBtn');
            const loading = document.getElementById('loading');
            const status = document.getElementById('status');
            const success = document.getElementById('success');
            const error = document.getElementById('error');
            
            btn.disabled = true;
            btn.classList.add('hidden');
            loading.classList.remove('hidden');
            status.classList.add('hidden');
            
            try {
                // Create credential options
                const credentialCreationOptions = {
                    publicKey: {
                        challenge: base64url.decode(challenge),
                        rp: {
                            name: rpName,
                            id: rpId
                        },
                        user: {
                            id: base64url.decode(userId),
                            name: userName,
                            displayName: displayName
                        },
                        pubKeyCredParams: [
                            { alg: -7, type: "public-key" }, // ES256
                            { alg: -257, type: "public-key" } // RS256
                        ],
                        authenticatorSelection: {
                            authenticatorAttachment: "platform",
                            userVerification: "required"
                        },
                        timeout: 60000,
                        attestation: "direct"
                    }
                };

                // Create credential
                const credential = await navigator.credentials.create(credentialCreationOptions);
                
                // Prepare response
                const response = {
                    id: credential.id,
                    rawId: base64url.encode(new Uint8Array(credential.rawId)),
                    type: credential.type,
                    response: {
                        clientDataJSON: base64url.encode(new Uint8Array(credential.response.clientDataJSON)),
                        attestationObject: base64url.encode(new Uint8Array(credential.response.attestationObject))
                    }
                };

                // Send to server
                const serverResponse = await fetch('/webauthn/auth', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify(response)
                });

                if (serverResponse.ok) {
                    loading.classList.add('hidden');
                    success.classList.remove('hidden');
                    
                    // Redirect to callback
                    setTimeout(() => {
                        window.location.href = callbackUrl + '?status=success&credentialId=' + credential.id;
                    }, 2000);
                } else {
                    throw new Error('Server verification failed');
                }

            } catch (err) {
                console.error('WebAuthn error:', err);
                loading.classList.add('hidden');
                error.classList.remove('hidden');
                btn.disabled = false;
                btn.classList.remove('hidden');
                
                // Fallback - redirect with error
                setTimeout(() => {
                    window.location.href = callbackUrl + '?status=error&message=' + encodeURIComponent(err.message);
                }, 3000);
            }
        }

        // Base64URL encoding/decoding utilities
        const base64url = {
            encode: function(arraybuffer) {
                return btoa(String.fromCharCode.apply(null, new Uint8Array(arraybuffer)))
                    .replace(/\+/g, '-')
                    .replace(/\//g, '_')
                    .replace(/=/g, '');
            },
            decode: function(base64url) {
                return Uint8Array.from(atob(base64url.replace(/-/g, '+').replace(/_/g, '/')), c => c.charCodeAt(0));
            }
        };

        // Auto-start if parameters are present
        if (challenge && userId && userName) {
            setTimeout(createPasskey, 1000);
        }
    </script>
</body>
</html>`

	data := struct {
		Challenge   string
		UserID      string
		UserName    string
		DisplayName string
		RPName      string
		RPID        string
		CallbackURL string
	}{
		Challenge:   challenge,
		UserID:      userID,
		UserName:    userName,
		DisplayName: displayName,
		RPName:      rpName,
		RPID:        rpID,
		CallbackURL: callbackURL,
	}

	renderTemplate(w, "webauthn", tmpl, data)
}

func webAuthnVerifyHandler(w http.ResponseWriter, r *http.Request) {
	var webAuthnResp WebAuthnResponse
	if err := json.NewDecoder(r.Body).Decode(&webAuthnResp); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// In a real implementation, you would:
	// 1. Verify the client data JSON
	// 2. Verify the attestation object
	// 3. Extract the public key
	// 4. Store the credential securely

	// For demo purposes, we'll simulate a successful verification
	credentialInfo := &CredentialInfo{
		CredentialID: webAuthnResp.ID,
		PublicKey:    "pkcpub_" + strings.ToUpper(uuid.New().String()[:16]),
		Algorithm:    "ES256",
		Origin:       "https://wwwallet.app",
		CreatedAt:    time.Now(),
	}

	// Store the credential (in production, use secure storage)
	store[webAuthnResp.ID] = credentialInfo

	writeJSON(w, map[string]interface{}{
		"status":     "success",
		"credential": credentialInfo,
		"verified":   true,
		"timestamp":  time.Now().UTC(),
	})
}

func callbackHandler(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	credentialID := r.URL.Query().Get("credentialId")
	message := r.URL.Query().Get("message")

	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <title>Authentication Result - Stellar Go CLI</title>
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; padding: 20px; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); min-height: 100vh; display: flex; align-items: center; justify-content: center; }
        .container { max-width: 500px; width: 100%; background: white; padding: 40px; border-radius: 20px; box-shadow: 0 20px 40px rgba(0,0,0,0.1); text-align: center; }
        .icon { font-size: 64px; margin-bottom: 20px; }
        h1 { color: #333; margin-bottom: 10px; }
        .message { color: #666; margin-bottom: 30px; }
        .status { margin: 20px 0; padding: 15px; border-radius: 10px; }
        .status.success { background: #d4edda; color: #155724; }
        .status.error { background: #f8d7da; color: #721c24; }
        .details { background: #f8f9fa; padding: 15px; border-radius: 10px; margin: 20px 0; text-align: left; font-size: 14px; }
        .btn { background: #6c757d; color: white; border: none; padding: 10px 20px; border-radius: 5px; cursor: pointer; }
    </style>
</head>
<body>
    <div class="container">
        <div class="icon">{{if eq .Status "success"}}✅{{else}}❌{{end}}</div>
        <h1>{{if eq .Status "success"}}Authentication Successful{{else}}Authentication Failed{{end}}</h1>
        <p class="message">{{.Message}}</p>
        
        <div class="status {{.Status}}">
            {{if eq .Status "success"}}
                Your passkey has been created and linked to your wallet successfully.
            {{else}}
                There was an issue creating your passkey. Please try again.
            {{end}}
        </div>
        
        {{if .CredentialID}}
        <div class="details">
            <strong>Credential Details:</strong><br>
            ID: {{.CredentialID}}<br>
            Created: {{.Timestamp}}
        </div>
        {{end}}
        
        <button class="btn" onclick="window.close()">Close Window</button>
    </div>
</body>
</html>`

	data := struct {
		Status       string
		Message      string
		CredentialID string
		Timestamp    string
	}{
		Status:       status,
		Message:      message,
		CredentialID: credentialID,
		Timestamp:    time.Now().Format(time.RFC3339),
	}

	renderTemplate(w, "callback", tmpl, data)
}

// webAuthnSignHandler handles GET requests for WebAuthn signing
func webAuthnSignHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	challenge := r.URL.Query().Get("challenge")
	credentialID := r.URL.Query().Get("credentialId")
	callback := r.URL.Query().Get("callback")
	transaction := r.URL.Query().Get("transaction")

	if challenge == "" || credentialID == "" || callback == "" {
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
		return
	}

	tmpl := `
<!DOCTYPE html>
<html>
<head>
	<title>Sign Transaction - Stellar Go CLI</title>
	<meta charset="UTF-8">
	<style>
		body { 
			font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; 
			margin: 0; 
			padding: 0; 
			background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); 
			min-height: 100vh; 
			display: flex; 
			align-items: center; 
			justify-content: center; 
		}
		.container { 
			max-width: 500px; 
			background: white; 
			padding: 40px; 
			border-radius: 20px; 
			box-shadow: 0 20px 40px rgba(0,0,0,0.1); 
			text-align: center; 
		}
		.icon { font-size: 64px; margin-bottom: 20px; }
		h1 { color: #333; margin-bottom: 10px; font-size: 24px; }
		p { color: #666; margin-bottom: 20px; line-height: 1.5; }
		.btn { 
			background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); 
			color: white; 
			border: none; 
			padding: 15px 30px; 
			border-radius: 8px; 
			font-size: 16px; 
			cursor: pointer; 
			margin: 10px;
			width: 100%%;
		}
		.transaction { 
			background: #f5f5f5; 
			padding: 15px; 
			border-radius: 8px; 
			margin: 20px 0; 
			font-family: monospace; 
			font-size: 12px; 
			word-break: break-all;
		}
	</style>
</head>
<body>
	<div class="container">
		<div class="icon">🔐</div>
		<h1>Sign Transaction</h1>
		<p>Please authenticate to sign this Stellar transaction.</p>
		
		<div class="transaction">
			<strong>Transaction:</strong><br>
			{{.Transaction}}
		</div>
		
		<button class="btn" onclick="signTransaction()">Sign with Passkey</button>
		<button class="btn" onclick="cancel()" style="background: #ccc;">Cancel</button>
	</div>

	<script>
		async function signTransaction() {
			try {
				// Create WebAuthn assertion options
				const options = {
					challenge: Uint8Array.from(atob('{{.Challenge}}), c => c.charCodeAt(0)),
					allowCredentials: [{
						id: Uint8Array.from(atob('{{.CredentialID}}'), c => c.charCodeAt(0)),
						type: 'public-key',
						transports: ['internal', 'usb', 'ble', 'nfc']
					}],
					userVerification: 'required'
				};

				// Get credential
				const credential = await navigator.credentials.get({
					publicKey: options
				});

				// Send signature to callback
				const callbackUrl = '{{.Callback}}?status=success&signature=' + 
					btoa(String.fromCharCode(...new Uint8Array(credential.response.signatureData))) +
					'&authenticatorData=' + 
					btoa(String.fromCharCode(...new Uint8Array(credential.response.authenticatorData))) +
					'&clientDataJSON=' + 
					btoa(String.fromCharCode(...new Uint8Array(credential.response.clientDataJSON)));

				window.location.href = callbackUrl;
			} catch (error) {
				console.error('Signing failed:', error);
				window.location.href = '{{.Callback}}?status=error&message=' + encodeURIComponent(error.message);
			}
		}

		function cancel() {
			window.location.href = '{{.Callback}}?status=cancelled';
		}
	</script>
</body>
</html>`

	data := struct {
		Challenge    string
		CredentialID string
		Callback     string
		Transaction  string
	}{
		Challenge:    challenge,
		CredentialID: credentialID,
		Callback:     callback,
		Transaction:  transaction,
	}

	renderTemplate(w, "sign", tmpl, data)
}

// webAuthnSignVerifyHandler handles POST requests for WebAuthn signing verification
func webAuthnSignVerifyHandler(w http.ResponseWriter, r *http.Request) {
	// For now, just return success - in real implementation would verify signature
	writeJSON(w, map[string]interface{}{
		"status":  "success",
		"message": "Transaction signed successfully",
	})
}
