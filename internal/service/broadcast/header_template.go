package broadcast

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/notifuse_mjml"
)

// renderDataFeedHeaders resolves Liquid placeholders against the same context
// that is sent in the data-feed request body. Header names remain literal.
func renderDataFeedHeaders(headers []domain.DataFeedHeader, payloadBytes []byte) ([]domain.DataFeedHeader, error) {
	if len(headers) == 0 {
		return headers, nil
	}

	contextData := make(map[string]interface{})
	if err := json.Unmarshal(payloadBytes, &contextData); err != nil {
		return nil, fmt.Errorf("build header template context: %w", err)
	}

	engine := notifuse_mjml.NewSecureLiquidEngine()
	renderedHeaders := make([]domain.DataFeedHeader, len(headers))
	for i, header := range headers {
		renderedHeaders[i] = header
		if !strings.Contains(header.Value, "{{") && !strings.Contains(header.Value, "{%") {
			continue
		}

		renderedValue, err := engine.Render(header.Value, contextData)
		if err != nil {
			return nil, fmt.Errorf("render header %s: %w", header.Name, err)
		}
		renderedHeaders[i].Value = renderedValue
	}

	return renderedHeaders, nil
}
