package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"golang.org/x/net/html"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

var executionCSSMedia = regexp.MustCompile(`(?i)url\(\s*["']?([^"'()]+?)["']?\s*\)`)
var executionCSSImport = regexp.MustCompile(`(?i)@import\s+["']([^"']+)["']`)

func executionMediaReferences(data []byte, allowDynamic bool) ([]string, error) {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	found := map[string]bool{}
	var failure error
	add := func(raw string) {
		raw = html.UnescapeString(strings.TrimSpace(raw))
		if raw == "" || strings.HasPrefix(raw, "#") {
			return
		}
		if strings.Contains(raw, "{{") || strings.Contains(raw, "{%") {
			if allowDynamic {
				return
			}
			failure = fmt.Errorf("media reference was not resolved during rendering")
			return
		}
		parsed, err := url.Parse(raw)
		if err != nil {
			failure = fmt.Errorf("invalid media reference")
			return
		}
		if parsed.Scheme == "data" {
			parts := strings.SplitN(raw, ",", 2)
			if len(parts) != 2 {
				failure = fmt.Errorf("invalid inline media data")
				return
			}
			if strings.Contains(strings.ToLower(parts[0]), "text/html") || strings.Contains(strings.ToLower(parts[0]), "text/css") {
				failure = fmt.Errorf("inline active documents must be replaced with self-contained media")
				return
			}
			if strings.Contains(strings.ToLower(parts[0]), "svg") {
				var content []byte
				if strings.Contains(parts[0], ";base64") {
					content, err = base64.StdEncoding.DecodeString(parts[1])
				} else {
					var decoded string
					decoded, err = url.PathUnescape(parts[1])
					content = []byte(decoded)
				}
				if err != nil {
					failure = fmt.Errorf("invalid inline SVG data")
					return
				}
				encoded, _ := json.Marshal(string(content))
				refs, err := executionMediaReferences(encoded, allowDynamic)
				if err != nil || len(refs) > 0 {
					failure = fmt.Errorf("inline SVG must contain all its media; use an absolute SVG URL for external dependencies")
				}
			}
			return
		}
		if (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil {
			failure = fmt.Errorf("execution media must use absolute HTTP(S) URLs or inline data: %s", raw)
			return
		}
		found[raw] = true
	}
	var walk func(any, bool)
	walk = func(value any, media bool) {
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				k := strings.ReplaceAll(strings.ToLower(key), "_", "")
				candidate := k == "src" || k == "image" || k == "imageurl" || k == "iconurl" || k == "mediaurl" || k == "backgroundurl" || k == "poster" || (media && k == "url") || strings.Contains(k, "attachment")
				walk(child, candidate)
			}
		case []any:
			for _, child := range v {
				walk(child, media)
			}
		case string:
			if media {
				add(v)
			}
			for _, pattern := range []*regexp.Regexp{executionCSSMedia, executionCSSImport} {
				for _, match := range pattern.FindAllStringSubmatch(v, -1) {
					add(match[1])
				}
			}
			if strings.Contains(v, "<") {
				tokenizer := html.NewTokenizer(strings.NewReader(v))
				for {
					kind := tokenizer.Next()
					if kind == html.ErrorToken {
						if tokenizer.Err() != io.EOF {
							failure = tokenizer.Err()
						}
						break
					}
					if kind != html.StartTagToken && kind != html.SelfClosingTagToken {
						continue
					}
					token := tokenizer.Token()
					for _, attr := range token.Attr {
						switch attr.Key {
						case "src", "poster", "background":
							if attr.Key == "src" && (token.Data == "iframe" || token.Data == "embed" || token.Data == "script") {
								failure = fmt.Errorf("remote active documents cannot be captured as execution media")
								continue
							}
							add(attr.Val)
						case "srcset":
							if strings.Contains(attr.Val, "data:") {
								failure = fmt.Errorf("inline data srcset is unsupported; use an inline src or absolute image URLs")
								continue
							}
							for _, candidate := range strings.Split(attr.Val, ",") {
								fields := strings.Fields(strings.TrimSpace(candidate))
								if len(fields) > 0 {
									add(fields[0])
								}
							}
						case "href", "xlink:href":
							if token.Data == "link" || token.Data == "image" || token.Data == "use" {
								add(attr.Val)
							}
						case "data":
							if token.Data == "object" {
								add(attr.Val)
							}
						}
					}
				}
			}
		}
	}
	walk(value, false)
	urls := []string{}
	for raw := range found {
		urls = append(urls, raw)
	}
	sort.Strings(urls)
	return urls, failure
}
func executionMediaURLs(data []byte) []string {
	urls, _ := executionMediaReferences(data, true)
	return urls
}
