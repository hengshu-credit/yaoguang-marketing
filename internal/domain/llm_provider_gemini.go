package domain

import (
	"fmt"

	"github.com/Notifuse/notifuse/pkg/crypto"
)

// GeminiSettings contains configuration for Google Gemini (Gemini Developer API)
type GeminiSettings struct {
	EncryptedAPIKey string `json:"encrypted_api_key,omitempty"`
	Model           string `json:"model"` // free text - e.g. gemini-2.5-flash

	// Decoded API key, not stored in the database
	APIKey string `json:"api_key,omitempty"`
}

// DecryptAPIKey decrypts the encrypted API key
func (g *GeminiSettings) DecryptAPIKey(passphrase string) error {
	apiKey, err := crypto.DecryptFromHexString(g.EncryptedAPIKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to decrypt Gemini API key: %w", err)
	}
	g.APIKey = apiKey
	return nil
}

// EncryptAPIKey encrypts the API key
func (g *GeminiSettings) EncryptAPIKey(passphrase string) error {
	encryptedAPIKey, err := crypto.EncryptString(g.APIKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to encrypt Gemini API key: %w", err)
	}
	g.EncryptedAPIKey = encryptedAPIKey
	return nil
}

// Validate validates the Gemini settings
func (g *GeminiSettings) Validate(passphrase string) error {
	if g.Model == "" {
		return fmt.Errorf("model is required for Gemini configuration")
	}

	// Encrypt API key if it's not empty
	if g.APIKey != "" {
		if err := g.EncryptAPIKey(passphrase); err != nil {
			return fmt.Errorf("failed to encrypt Gemini API key: %w", err)
		}
	}

	return nil
}
