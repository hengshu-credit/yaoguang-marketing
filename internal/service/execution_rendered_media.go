package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	pkgcrypto "github.com/hengshu-credit/yaoguang-marketing/pkg/crypto"
	"net/url"
	"regexp"
	"strings"
)

type executionMediaReference struct {
	SourceURL string `json:"source_url"`
	Hash      string `json:"hash"`
	MediaType string `json:"media_type"`
}

func (s *ExecutionSnapshotService) archiveMediaSet(ctx context.Context, ws string, urls []string, existing map[string]executionMediaReference) ([]executionMediaReference, error) {
	queue := append([]string{}, urls...)
	seen := map[string]bool{}
	result := []executionMediaReference{}
	size := 0
	for len(queue) > 0 {
		raw := queue[0]
		queue = queue[1:]
		if seen[raw] {
			continue
		}
		seen[raw] = true
		if len(seen) > 100 {
			return nil, errors.New("execution media exceeds 100 resources")
		}
		if ref, ok := existing[raw]; ok {
			result = append(result, ref)
			continue
		}
		asset, err := s.archive(ctx, ws, raw)
		if err != nil {
			return nil, fmt.Errorf("archive execution media: %w", err)
		}
		size += len(asset.Data)
		if size > 100<<20 {
			return nil, errors.New("execution media exceeds 100 MiB")
		}
		result = append(result, executionMediaReference{raw, asset.Hash, asset.MediaType})
		if strings.Contains(strings.ToLower(asset.MediaType), "text/css") {
			base, err := url.Parse(raw)
			if err != nil {
				return nil, err
			}
			for _, pattern := range []*regexp.Regexp{executionCSSMedia, executionCSSImport} {
				for _, match := range pattern.FindAllStringSubmatch(string(asset.Data), -1) {
					source := match[1]
					if strings.HasPrefix(source, "data:") || strings.HasPrefix(source, "#") {
						continue
					}
					parsed, err := url.Parse(source)
					if err != nil {
						return nil, err
					}
					resolved := base.ResolveReference(parsed)
					if resolved.Scheme != "http" && resolved.Scheme != "https" {
						return nil, errors.New("unsupported stylesheet media URL")
					}
					queue = append(queue, resolved.String())
				}
			}
		} else if strings.Contains(strings.ToLower(asset.MediaType), "svg") {
			encoded, _ := json.Marshal(string(asset.Data))
			refs, err := executionMediaReferences(encoded, false)
			if err != nil {
				return nil, err
			}
			queue = append(queue, refs...)
		} else if strings.Contains(strings.ToLower(asset.MediaType), "mpegurl") || strings.HasSuffix(strings.ToLower(raw), ".m3u8") {
			return nil, errors.New("streaming media playlists must be replaced with a self-contained media asset")
		}
	}
	return result, nil
}
func isExecutionTrackingPixel(raw, ws, messageID string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if strings.HasSuffix(parsed.Path, "/opens") {
		return parsed.Query().Get("wid") == ws && parsed.Query().Get("mid") == messageID
	}
	if tokenIndex := strings.LastIndex(parsed.Path, "/t/"); tokenIndex >= 0 {
		plain, err := pkgcrypto.DecryptTrackingToken(parsed.Path[tokenIndex+3:])
		if err == nil {
			parts := strings.Split(plain, "\n")
			return len(parts) == 3 && parts[0] == messageID && parts[1] == ws
		}
	}
	return false
}
func (s *ExecutionSnapshotService) ArchiveRenderedMedia(ctx context.Context, ws, executionID, deliveryID string, content any) error {
	encoded, err := json.Marshal(content)
	if err != nil {
		return err
	}
	refs, err := executionMediaReferences(encoded, false)
	if err != nil {
		return err
	}
	filtered := refs[:0]
	for _, raw := range refs {
		if !isExecutionTrackingPixel(raw, ws, deliveryID) {
			filtered = append(filtered, raw)
		}
	}
	refs = filtered
	if len(refs) == 0 {
		return nil
	}
	if saved, ok := s.repository.(interface {
		DeliveryMediaReferences(context.Context, string, string, string) (json.RawMessage, error)
	}); ok {
		previous, err := saved.DeliveryMediaReferences(ctx, ws, executionID, deliveryID)
		if err != nil {
			return err
		}
		if len(previous) > 0 {
			return nil
		}
	}
	snapshot, err := s.repository.Get(ctx, ws, executionID)
	if err != nil {
		return err
	}
	var materials struct {
		Assets []executionMediaReference `json:"assets"`
	}
	if err = json.Unmarshal(snapshot.Materials, &materials); err != nil {
		return err
	}
	existing := map[string]executionMediaReference{}
	for _, ref := range materials.Assets {
		existing[ref.SourceURL] = ref
	}
	assets, err := s.archiveMediaSet(ctx, ws, refs, existing)
	if err != nil {
		return err
	}
	// Preserve dependencies already archived with a static stylesheet as well.
	for _, ref := range materials.Assets {
		found := false
		for _, current := range assets {
			if current.SourceURL == ref.SourceURL {
				found = true
				break
			}
		}
		if !found {
			assets = append(assets, ref)
		}
	}
	payload, err := domain.SanitizeExecutionJSON(map[string]any{"message_id": deliveryID, "assets": assets})
	if err != nil {
		return err
	}
	customerID := ""
	if resolver, ok := s.repository.(interface {
		ResolveMediaCustomer(context.Context, string, string, string) (string, error)
	}); ok {
		var identity struct {
			CustomerID string `json:"customer_id"`
			Email      string `json:"contact_email"`
		}
		if err = json.Unmarshal(encoded, &identity); err != nil {
			return err
		}
		customerID, err = resolver.ResolveMediaCustomer(ctx, ws, identity.CustomerID, identity.Email)
		if err != nil {
			return err
		}
	}
	return s.AppendInternal(ctx, ws, domain.ExecutionEvidence{ExecutionID: executionID, Key: "media:" + deliveryID, Kind: "delivery_media", CustomerID: customerID, DeliveryID: deliveryID, Status: "archived", Payload: payload})
}
