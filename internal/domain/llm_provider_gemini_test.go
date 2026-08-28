package domain_test

import (
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiSettings_EncryptAPIKey(t *testing.T) {
	passphrase := "test-passphrase"
	apiKey := "AIzaSy-test-gemini-key"

	settings := domain.GeminiSettings{
		APIKey: apiKey,
		Model:  "gemini-2.5-flash",
	}

	// Test encryption
	err := settings.EncryptAPIKey(passphrase)
	require.NoError(t, err)
	assert.NotEmpty(t, settings.EncryptedAPIKey)

	// Verify by decrypting directly
	decrypted, err := crypto.DecryptFromHexString(settings.EncryptedAPIKey, passphrase)
	require.NoError(t, err)
	assert.Equal(t, apiKey, decrypted)
}

func TestGeminiSettings_DecryptAPIKey(t *testing.T) {
	passphrase := "test-passphrase"
	apiKey := "AIzaSy-test-gemini-key"

	// Create encrypted key
	encryptedKey, err := crypto.EncryptString(apiKey, passphrase)
	require.NoError(t, err)

	settings := domain.GeminiSettings{
		EncryptedAPIKey: encryptedKey,
		Model:           "gemini-2.5-flash",
	}

	// Test decryption
	err = settings.DecryptAPIKey(passphrase)
	require.NoError(t, err)
	assert.Equal(t, apiKey, settings.APIKey)

	// Test with wrong passphrase
	settings.APIKey = ""
	err = settings.DecryptAPIKey("wrong-passphrase")
	assert.Error(t, err)
}

func TestGeminiSettings_Validate(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("valid settings", func(t *testing.T) {
		settings := domain.GeminiSettings{
			APIKey: "AIzaSy-test-gemini-key",
			Model:  "gemini-2.5-flash",
		}

		err := settings.Validate(passphrase)
		require.NoError(t, err)
		// API key should be encrypted after validation
		assert.NotEmpty(t, settings.EncryptedAPIKey)
	})

	t.Run("missing model", func(t *testing.T) {
		settings := domain.GeminiSettings{
			APIKey: "AIzaSy-test-gemini-key",
		}

		err := settings.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "model is required")
	})

	t.Run("empty API key is allowed", func(t *testing.T) {
		// This allows updating settings without providing a new API key
		settings := domain.GeminiSettings{
			Model: "gemini-2.5-flash",
		}

		err := settings.Validate(passphrase)
		require.NoError(t, err)
		assert.Empty(t, settings.EncryptedAPIKey)
	})

	t.Run("any model name is allowed", func(t *testing.T) {
		// Model is free text - no validation on specific model names
		settings := domain.GeminiSettings{
			APIKey: "AIzaSy-test-gemini-key",
			Model:  "gemini-9-ultra-future",
		}

		err := settings.Validate(passphrase)
		require.NoError(t, err)
	})
}
