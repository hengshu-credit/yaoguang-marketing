package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrExecutionSnapshotNotFound = errors.New("execution snapshot not found")
var ErrExecutionSnapshotConflict = errors.New("execution snapshot is immutable")

// ExecutionSnapshot is an immutable run record, never a Campaign version.
// JSON values own their bytes and cannot alias a live template or configuration.
type ExecutionSnapshot struct {
	ID            string          `json:"id"`
	Kind          string          `json:"kind"`
	ExecutionID   string          `json:"execution_id"`
	SourceID      string          `json:"source_id"`
	Configuration json.RawMessage `json:"configuration"`
	Materials     json.RawMessage `json:"materials"`
	Dependencies  json.RawMessage `json:"dependencies"`
	ContentHash   string          `json:"content_hash"`
	Completeness  string          `json:"completeness"`
	Missing       []string        `json:"missing"`
	CreatedAt     time.Time       `json:"created_at"`
}

type ExecutionEvidence struct {
	ID          string          `json:"id"`
	ExecutionID string          `json:"execution_id"`
	Key         string          `json:"key"`
	Kind        string          `json:"kind"`
	NodeID      string          `json:"node_id,omitempty"`
	CustomerID  string          `json:"customer_id,omitempty"`
	DeliveryID  string          `json:"delivery_id,omitempty"`
	Status      string          `json:"status"`
	Payload     json.RawMessage `json:"payload"`
	OccurredAt  time.Time       `json:"occurred_at"`
}

type ExecutionAsset struct {
	Hash      string `json:"hash"`
	MediaType string `json:"media_type"`
	Data      []byte `json:"-"`
}

type ExecutionDetails struct {
	Snapshot *ExecutionSnapshot  `json:"snapshot"`
	Evidence []ExecutionEvidence `json:"evidence"`
	Next     string              `json:"next,omitempty"`
}

type ExecutionSnapshotRepository interface {
	Save(context.Context, string, *ExecutionSnapshot) error
	Get(context.Context, string, string) (*ExecutionSnapshot, error)
	List(context.Context, string, string, int) ([]ExecutionSnapshot, error)
	AppendEvidence(context.Context, string, ExecutionEvidence) error
	ListEvidence(context.Context, string, string, string, int) ([]ExecutionEvidence, string, error)
	SaveAsset(context.Context, string, ExecutionAsset) error
	GetAsset(context.Context, string, string) (*ExecutionAsset, error)
}

func NewExecutionSnapshot(kind, executionID string, configuration any, templates []*Template) (*ExecutionSnapshot, error) {
	if (kind != "campaign" && kind != "journey" && kind != "decision") || strings.TrimSpace(executionID) == "" {
		return nil, errors.New("execution kind and id are required")
	}
	config, err := SanitizeExecutionJSON(configuration)
	if err != nil {
		return nil, err
	}
	if templates == nil {
		templates = []*Template{}
	}
	materials, err := SanitizeExecutionJSON(templates)
	if err != nil {
		return nil, err
	}
	s := &ExecutionSnapshot{Kind: kind, ExecutionID: executionID, Configuration: config, Materials: materials, Dependencies: json.RawMessage(`{}`), Completeness: "complete", Missing: []string{}, CreatedAt: time.Now().UTC()}
	if err := s.Rehash(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *ExecutionSnapshot) Rehash() error {
	body, err := json.Marshal(struct {
		Configuration json.RawMessage
		Materials     json.RawMessage
		Dependencies  json.RawMessage
	}{s.Configuration, s.Materials, s.Dependencies})
	if err != nil {
		return err
	}
	var canonical any
	if err = json.Unmarshal(body, &canonical); err != nil {
		return err
	}
	body, err = json.Marshal(canonical)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(body)
	s.ContentHash = hex.EncodeToString(hash[:])
	return nil
}

func (e ExecutionEvidence) Validate() error {
	if e.ExecutionID == "" || e.Key == "" || e.Kind == "" || e.Status == "" {
		return errors.New("execution evidence requires execution, idempotency key, kind and status")
	}
	if !json.Valid(e.Payload) || string(e.Payload) == "null" {
		return errors.New("execution evidence requires a JSON payload")
	}
	return nil
}

// SanitizeExecutionJSON removes credentials recursively, retaining business
// predicates and render inputs. Reading PII additionally requires contact access.
func SanitizeExecutionJSON(value any) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode execution snapshot: %w", err)
	}
	var decoded any
	if err = json.Unmarshal(encoded, &decoded); err != nil {
		return nil, err
	}
	redactExecutionSecrets(decoded)
	return json.Marshal(decoded)
}

func redactExecutionSecrets(value any) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			normalized := strings.ToLower(strings.NewReplacer("-", "", "_", "", " ", "").Replace(key))
			switch normalized {
			case "authorization", "apikey", "secret", "clientsecret", "password", "accesstoken", "refreshtoken", "privatekey", "jwtsecret", "hmacsecret", "cookie", "setcookie", "authtoken", "encryptedauthtoken", "apikeysecret", "encryptedapikeysecret", "signingsecret", "providersettings":
				v[key] = "[REDACTED]"
			default:
				redactExecutionSecrets(child)
			}
		}
	case []any:
		for _, child := range v {
			redactExecutionSecrets(child)
		}
	}
}
