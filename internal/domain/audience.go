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

type AudienceKind string

const (
	AudienceKindStatic    AudienceKind = "static"
	AudienceKindDynamic   AudienceKind = "dynamic"
	AudienceKindComposite AudienceKind = "composite"
)

type AudienceOperator string

const (
	AudienceOperatorUnion        AudienceOperator = "union"
	AudienceOperatorIntersection AudienceOperator = "intersection"
	AudienceOperatorExclusion    AudienceOperator = "exclusion"
)

type AudienceLeafType string

const (
	AudienceLeafList     AudienceLeafType = "list"
	AudienceLeafSegment  AudienceLeafType = "segment"
	AudienceLeafAudience AudienceLeafType = "audience"
)

// AudienceExpression is a tagged JSON tree. A leaf has LeafType and RefID;
// a composite has Operator and Children. It deliberately cannot contain SQL.
type AudienceExpression struct {
	LeafType AudienceLeafType     `json:"leaf_type,omitempty"`
	RefID    string               `json:"ref_id,omitempty"`
	Operator AudienceOperator     `json:"operator,omitempty"`
	Children []AudienceExpression `json:"children,omitempty"`
}

func (e AudienceExpression) Validate() error {
	isLeaf := e.LeafType != "" || e.RefID != ""
	isComposite := e.Operator != "" || len(e.Children) > 0
	if isLeaf == isComposite {
		return errors.New("audience expression must be exactly one leaf or composite")
	}
	if isLeaf {
		switch e.LeafType {
		case AudienceLeafList, AudienceLeafSegment, AudienceLeafAudience:
		default:
			return fmt.Errorf("unsupported audience leaf type: %s", e.LeafType)
		}
		if strings.TrimSpace(e.RefID) == "" {
			return errors.New("audience leaf ref_id is required")
		}
		return nil
	}
	switch e.Operator {
	case AudienceOperatorUnion, AudienceOperatorIntersection:
		if len(e.Children) < 2 {
			return errors.New("union and intersection require at least two children")
		}
	case AudienceOperatorExclusion:
		if len(e.Children) != 2 {
			return errors.New("exclusion requires exactly two children")
		}
	default:
		return fmt.Errorf("unsupported audience operator: %s", e.Operator)
	}
	for index := range e.Children {
		if err := e.Children[index].Validate(); err != nil {
			return fmt.Errorf("audience child %d: %w", index, err)
		}
	}
	return nil
}

func (e AudienceExpression) CanonicalJSON() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(e)
}

func (e AudienceExpression) VersionHash() (string, error) {
	canonical, err := e.CanonicalJSON()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

type Audience struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	Description   string       `json:"description,omitempty"`
	Kind          AudienceKind `json:"kind"`
	ActiveVersion int          `json:"active_version"`
	ActiveBuildID string       `json:"active_build_id,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
}

type AudienceVersion struct {
	AudienceID     string             `json:"audience_id"`
	Version        int                `json:"version"`
	Definition     AudienceExpression `json:"definition"`
	DefinitionHash string             `json:"definition_hash"`
	CreatedAt      time.Time          `json:"created_at"`
}

type AudienceRepository interface {
	CreateAudience(context.Context, string, Audience, AudienceVersion) error
	GetAudience(context.Context, string, string) (*Audience, error)
	GetAudienceVersion(context.Context, string, string, int) (*AudienceVersion, error)
}
