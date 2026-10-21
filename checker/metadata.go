package checker

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"

	nethtml "golang.org/x/net/html"
)

type ProfileMetadata struct {
	DisplayName string `json:"display_name,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Bio         string `json:"bio,omitempty"`
	Followers   string `json:"followers,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	Location    string `json:"location,omitempty"`
}

func (m ProfileMetadata) HasData() bool {
	return m.DisplayName != "" || m.AvatarURL != "" || m.Bio != "" || m.Followers != "" || m.CreatedAt != "" || m.Location != ""
}

var (
	followerRegex = regexp.MustCompile(`(?i)([\d,\.]+[kKmMbB]?)\s+(?:Followers|Subscribers)`)
	tagStripRegex = regexp.MustCompile(`<[^>]*>`)
)

func stripEmojis(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 0x1F600 && r <= 0x1F64F) ||
			(r >= 0x1F300 && r <= 0x1F5FF) ||
			(r >= 0x1F680 && r <= 0x1F6FF) ||
			(r >= 0x1F1E0 && r <= 0x1F1FF) ||
			(r >= 0x2600 && r <= 0x26FF) ||
			(r >= 0x2700 && r <= 0x27BF) ||
			(r >= 0xFE00 && r <= 0xFE0F) ||
			(r >= 0x1F900 && r <= 0x1F9FF) ||
			(r >= 0x1FA00 && r <= 0x1FAFF) ||
			r == 0x200D {
			return -1
		}
		return r
	}, s)
}

func cleanMetadataString(s string) string {
	stripped := tagStripRegex.ReplaceAllString(s, " ")
	unescaped := html.UnescapeString(stripped)
	noEmojis := stripEmojis(unescaped)
	fields := strings.Fields(noEmojis)
	return strings.Join(fields, " ")
}

func ExtractProfileMetadata(htmlStr string) ProfileMetadata {
	var meta ProfileMetadata
	if strings.TrimSpace(htmlStr) == "" {
		return meta
	}

	tokenizer := nethtml.NewTokenizer(strings.NewReader(htmlStr))
	ogTitle := ""
	ogDesc := ""
	ogImage := ""
	twitterTitle := ""
	twitterDesc := ""
	twitterImage := ""
	standardDesc := ""

	for {
		tt := tokenizer.Next()
		if tt == nethtml.ErrorToken {
			break
		}

		tok := tokenizer.Token()

		if (tt == nethtml.StartTagToken || tt == nethtml.SelfClosingTagToken) && strings.ToLower(tok.Data) == "meta" {
			var propKey, contentVal string
			for _, attr := range tok.Attr {
				k := strings.ToLower(attr.Key)
				if k == "property" || k == "name" {
					propKey = strings.ToLower(strings.TrimSpace(attr.Val))
				} else if k == "content" {
					contentVal = strings.TrimSpace(attr.Val)
				}
			}

			if contentVal != "" {
				switch propKey {
				case "og:title":
					if ogTitle == "" {
						ogTitle = contentVal
					}
				case "og:description":
					if ogDesc == "" {
						ogDesc = contentVal
					}
				case "og:image", "og:image:url":
					if ogImage == "" {
						ogImage = contentVal
					}
				case "twitter:title":
					if twitterTitle == "" {
						twitterTitle = contentVal
					}
				case "twitter:description":
					if twitterDesc == "" {
						twitterDesc = contentVal
					}
				case "twitter:image", "twitter:image:src":
					if twitterImage == "" {
						twitterImage = contentVal
					}
				case "description":
					if standardDesc == "" {
						standardDesc = contentVal
					}
				}
			}
		}

		if tt == nethtml.StartTagToken && strings.ToLower(tok.Data) == "script" {
			isJSONLD := false
			for _, attr := range tok.Attr {
				if strings.ToLower(attr.Key) == "type" && strings.ToLower(strings.TrimSpace(attr.Val)) == "application/ld+json" {
					isJSONLD = true
					break
				}
			}

			if isJSONLD {
				if nextTokType := tokenizer.Next(); nextTokType == nethtml.TextToken {
					rawScript := tokenizer.Token().Data
					var rawData interface{}
					if err := json.Unmarshal([]byte(rawScript), &rawData); err == nil {
						parseJSONLD(rawData, &meta)
					}
				}
			}
		}
	}

	if meta.DisplayName == "" {
		if ogTitle != "" {
			meta.DisplayName = cleanMetadataString(ogTitle)
		} else if twitterTitle != "" {
			meta.DisplayName = cleanMetadataString(twitterTitle)
		}
	}

	if meta.Bio == "" {
		if ogDesc != "" {
			meta.Bio = cleanMetadataString(ogDesc)
		} else if twitterDesc != "" {
			meta.Bio = cleanMetadataString(twitterDesc)
		} else if standardDesc != "" {
			meta.Bio = cleanMetadataString(standardDesc)
		}
	}

	if meta.AvatarURL == "" {
		if ogImage != "" {
			meta.AvatarURL = ogImage
		} else if twitterImage != "" {
			meta.AvatarURL = twitterImage
		}
	}

	if meta.Followers == "" {
		combined := meta.Bio
		if match := followerRegex.FindStringSubmatch(combined); len(match) > 1 {
			meta.Followers = match[1]
		}
	}

	return meta
}

func parseJSONLD(data interface{}, meta *ProfileMetadata) {
	switch val := data.(type) {
	case []interface{}:
		for _, item := range val {
			parseJSONLD(item, meta)
		}
	case map[string]interface{}:
		if graph, ok := val["@graph"].([]interface{}); ok {
			for _, item := range graph {
				parseJSONLD(item, meta)
			}
		}

		if meta.DisplayName == "" {
			if name, ok := val["name"].(string); ok && name != "" {
				meta.DisplayName = cleanMetadataString(name)
			} else if alt, ok := val["alternateName"].(string); ok && alt != "" {
				meta.DisplayName = cleanMetadataString(alt)
			} else if headline, ok := val["headline"].(string); ok && headline != "" {
				meta.DisplayName = cleanMetadataString(headline)
			}
		}

		if meta.AvatarURL == "" {
			if img, ok := val["image"].(string); ok && img != "" {
				meta.AvatarURL = img
			} else if imgObj, ok := val["image"].(map[string]interface{}); ok {
				if u, ok := imgObj["url"].(string); ok && u != "" {
					meta.AvatarURL = u
				} else if cu, ok := imgObj["contentUrl"].(string); ok && cu != "" {
					meta.AvatarURL = cu
				}
			} else if imgList, ok := val["image"].([]interface{}); ok && len(imgList) > 0 {
				if firstStr, ok := imgList[0].(string); ok && firstStr != "" {
					meta.AvatarURL = firstStr
				}
			}
		}

		if meta.Bio == "" {
			if desc, ok := val["description"].(string); ok && desc != "" {
				meta.Bio = cleanMetadataString(desc)
			}
		}

		if meta.CreatedAt == "" {
			if dc, ok := val["dateCreated"].(string); ok && dc != "" {
				meta.CreatedAt = cleanMetadataString(dc)
			} else if sd, ok := val["startDate"].(string); ok && sd != "" {
				meta.CreatedAt = cleanMetadataString(sd)
			}
		}

		if meta.Location == "" {
			if locObj, ok := val["homeLocation"].(map[string]interface{}); ok {
				if n, ok := locObj["name"].(string); ok && n != "" {
					meta.Location = cleanMetadataString(n)
				}
			} else if addrObj, ok := val["address"].(map[string]interface{}); ok {
				var parts []string
				if loc, ok := addrObj["addressLocality"].(string); ok && loc != "" {
					parts = append(parts, loc)
				}
				if reg, ok := addrObj["addressRegion"].(string); ok && reg != "" {
					parts = append(parts, reg)
				}
				if country, ok := addrObj["addressCountry"].(string); ok && country != "" {
					parts = append(parts, country)
				}
				if len(parts) > 0 {
					meta.Location = cleanMetadataString(strings.Join(parts, ", "))
				}
			} else if locStr, ok := val["location"].(string); ok && locStr != "" {
				meta.Location = cleanMetadataString(locStr)
			}
		}

		if meta.Followers == "" {
			if stats, ok := val["interactionStatistic"]; ok {
				checkStat := func(s map[string]interface{}) {
					if count, exists := s["userInteractionCount"]; exists {
						meta.Followers = fmt.Sprintf("%v", count)
					}
				}
				if sMap, ok := stats.(map[string]interface{}); ok {
					checkStat(sMap)
				} else if sArr, ok := stats.([]interface{}); ok {
					for _, item := range sArr {
						if itemMap, ok := item.(map[string]interface{}); ok {
							iType, _ := itemMap["interactionType"].(string)
							if strings.Contains(strings.ToLower(iType), "follow") || strings.Contains(strings.ToLower(iType), "subscribe") {
								checkStat(itemMap)
								if meta.Followers != "" {
									break
								}
							}
						}
					}
					if meta.Followers == "" && len(sArr) > 0 {
						if firstMap, ok := sArr[0].(map[string]interface{}); ok {
							checkStat(firstMap)
						}
					}
				}
			}
		}
	}
}

