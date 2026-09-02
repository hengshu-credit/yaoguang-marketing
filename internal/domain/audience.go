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
	"unicode/utf8"

	"github.com/asaskevich/govalidator"
	"github.com/google/uuid"
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
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Description   string              `json:"description,omitempty"`
	Kind          AudienceKind        `json:"kind"`
	ActiveVersion int                 `json:"active_version"`
	ActiveBuildID string              `json:"active_build_id,omitempty"`
	Definition    *AudienceExpression `json:"definition,omitempty"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
}

// AudienceCustomerMatch is one live evaluation of the current Audience
// definition against the Customer's current facts.
type AudienceCustomerMatch struct {
	AudienceID      string       `json:"audience_id"`
	Name            string       `json:"name"`
	Kind            AudienceKind `json:"kind"`
	AudienceVersion int          `json:"audience_version"`
	Matches         bool         `json:"matches"`
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

const (
	DefaultAudienceMemberLimit = 50
	MaxAudienceMemberLimit     = 200
)

var audienceMemberStatuses = map[string]struct{}{
	"active": {}, "pending": {}, "unsubscribed": {}, "bounced": {}, "complained": {},
}

// AudienceMemberQuery lists customers from exactly one LIST, dynamic Audience,
// or immutable Audience build while applying current customer facts.
type AudienceMemberQuery struct {
	ListID         string
	AudienceID     string
	BuildID        string
	Status         string
	EventName      string
	JoinedAfter    *time.Time
	JoinedBefore   *time.Time
	AttributeKey   string
	AttributeValue string
	After          string
	Limit          int
}

func (query *AudienceMemberQuery) Validate() error {
	if query == nil {
		return errors.New("audience member query is required")
	}
	query.ListID = strings.TrimSpace(query.ListID)
	query.AudienceID = strings.TrimSpace(query.AudienceID)
	query.BuildID = strings.TrimSpace(query.BuildID)
	sources := 0
	for _, value := range []string{query.ListID, query.AudienceID, query.BuildID} {
		if value != "" {
			sources++
		}
	}
	if sources != 1 {
		return errors.New("exactly one member source is required")
	}
	if query.ListID != "" && (utf8.RuneCountInString(query.ListID) > 32 || !govalidator.IsAlphanumeric(query.ListID)) {
		return errors.New("list ID must be alphanumeric and contain 1 to 32 characters")
	}
	for label, value := range map[string]string{"audience ID": query.AudienceID, "build ID": query.BuildID} {
		if value == "" {
			continue
		}
		parsed, err := uuid.Parse(value)
		if err != nil || parsed == uuid.Nil {
			return fmt.Errorf("%s must be a non-nil UUID", label)
		}
	}
	query.Status = strings.TrimSpace(query.Status)
	if query.Status != "" {
		if _, ok := audienceMemberStatuses[query.Status]; !ok {
			return errors.New("subscription status is invalid")
		}
	}
	if (query.JoinedAfter != nil || query.JoinedBefore != nil) && query.ListID == "" {
		return errors.New("join time filters require a list source")
	}
	if query.JoinedAfter != nil && query.JoinedBefore != nil && !query.JoinedAfter.Before(*query.JoinedBefore) {
		return errors.New("joined_after must be before joined_before")
	}
	query.EventName = strings.TrimSpace(query.EventName)
	if utf8.RuneCountInString(query.EventName) > 100 {
		return errors.New("event name cannot exceed 100 characters")
	}
	query.AttributeKey = strings.TrimSpace(query.AttributeKey)
	query.AttributeValue = strings.TrimSpace(query.AttributeValue)
	if (query.AttributeKey == "") != (query.AttributeValue == "") {
		return errors.New("attribute key and value must be provided together")
	}
	if utf8.RuneCountInString(query.AttributeKey) > 255 || utf8.RuneCountInString(query.AttributeValue) > 255 {
		return errors.New("attribute key and value cannot exceed 255 characters")
	}
	query.After = strings.TrimSpace(query.After)
	if query.After != "" {
		parsed, err := uuid.Parse(query.After)
		if err != nil || parsed == uuid.Nil {
			return errors.New("member cursor must be a non-nil customer UUID")
		}
	}
	if query.Limit == 0 {
		query.Limit = DefaultAudienceMemberLimit
	}
	if query.Limit < 1 || query.Limit > MaxAudienceMemberLimit {
		return fmt.Errorf("audience member limit must be between 1 and %d", MaxAudienceMemberLimit)
	}
	return nil
}

type AudienceMember struct {
	Customer      CustomerSummary          `json:"customer"`
	Subscriptions []CustomerListMembership `json:"subscriptions"`
	JoinedAt      *time.Time               `json:"joined_at,omitempty"`
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
	ListAudienceMembers(context.Context, string, AudienceMemberQuery) ([]AudienceMember, string, error)
	ArchiveAudience(context.Context, string, string) error
}
