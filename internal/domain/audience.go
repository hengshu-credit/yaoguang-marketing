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
	LeafType  AudienceLeafType     `json:"leaf_type,omitempty"`
	RefID     string               `json:"ref_id,omitempty"`
	Condition *TreeNode            `json:"condition,omitempty"`
	Operator  AudienceOperator     `json:"operator,omitempty"`
	Children  []AudienceExpression `json:"children,omitempty"`
}

func (e AudienceExpression) Validate() error {
	isReference := e.LeafType != "" || e.RefID != ""
	isCondition := e.Condition != nil
	isComposite := e.Operator != "" || len(e.Children) > 0
	shapeCount := 0
	for _, present := range []bool{isReference, isCondition, isComposite} {
		if present {
			shapeCount++
		}
	}
	if shapeCount != 1 {
		return errors.New("audience expression must be exactly one reference leaf, condition leaf, or composite")
	}
	if isReference {
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
	if isCondition {
		if err := e.Condition.Validate(); err != nil {
			return fmt.Errorf("audience condition: %w", err)
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

type AudienceBuild struct {
	ID              string    `json:"id"`
	AudienceID      string    `json:"audience_id"`
	AudienceVersion int       `json:"audience_version"`
	Status          string    `json:"status"`
	LastCustomerID  string    `json:"last_customer_id,omitempty"`
	MemberCount     int64     `json:"member_count"`
	ErrorDetail     string    `json:"error_detail,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type AudienceRepository interface {
	CreateAudience(context.Context, string, Audience, AudienceVersion) error
	GetAudience(context.Context, string, string) (*Audience, error)
	ListAudiences(context.Context, string, int, int) ([]Audience, int, error)
	GetAudienceVersion(context.Context, string, string, int) (*AudienceVersion, error)
	SaveAudienceVersion(context.Context, string, string, AudienceExpression) (*AudienceVersion, error)
	PreviewAudience(context.Context, string, AudienceExpression, int) ([]CustomerSummary, int64, error)
	BuildAudience(context.Context, string, string, int) (string, int64, error)
	GetAudienceBuild(context.Context, string, string) (*AudienceBuild, error)
	ListAudienceMembers(context.Context, string, string, string, int) ([]CustomerSummary, string, error)
	ArchiveAudience(context.Context, string, string) error
}
