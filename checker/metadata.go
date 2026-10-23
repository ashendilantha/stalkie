package checker

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
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

func isPlaceholderAvatar(rawURL string) bool {
	u := strings.ToLower(rawURL)
	if u == "" {
		return true
	}
	markers := []string{
		"default_avatar",
		"default-avatar",
		"default_profile",
		"default-profile",
		"blank.gif",
		"blank.png",
		"placeholder",
		"anonymous.png",
		"missing.png",
		"none.png",
		"avatar_default",
		"no_avatar",
		"default_user",
		"default_pic",
		"avatar-default",
		"defaultuser",
		"00000000000000000000000000000000",
		"d=identicon",
		"d=mm",
		"d=mystery",
		"d=blank",
		"d=mp",
		"mona-loading-default",
		"reddit-logo",
		"site-logo",
		"favicon",
		"logo.png",
		"logo.svg",
		"header.png",
		"banner.png",
	}
	for _, m := range markers {
		if strings.Contains(u, m) {
			return true
		}
	}
	return false
}

func isAvatarAttr(s string) bool {
	lower := strings.ToLower(s)
	markers := []string{
		"avatar",
		"profile-pic",
		"profile_pic",
		"userpic",
		"user-avatar",
		"user_avatar",
		"profile-image",
		"profile_image",
		"profile-photo",
		"author-img",
		"author-image",
		"member-avatar",
	}
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

func resolveAvatarURL(baseStr, refStr string) string {
	refStr = strings.TrimSpace(refStr)
	if refStr == "" {
		return ""
	}
	if strings.HasPrefix(refStr, "//") {
		return "https:" + refStr
	}
	if strings.HasPrefix(refStr, "http://") || strings.HasPrefix(refStr, "https://") {
		return refStr
	}
	if baseStr == "" {
		return refStr
	}
	base, err := url.Parse(baseStr)
	if err != nil {
		return refStr
	}
	ref, err := url.Parse(refStr)
	if err != nil {
		return refStr
	}
	return base.ResolveReference(ref).String()
}

func extractFromJSON(data interface{}, meta *ProfileMetadata) bool {
	switch v := data.(type) {
	case []interface{}:
		for _, item := range v {
			if extractFromJSON(item, meta) {
				return true
			}
		}
	case map[string]interface{}:
		avatarKeys := []string{
			"avatar_url", "avatar", "avatar_720_url", "avatar_large",
			"picture", "profile_image_url", "profile_image_url_https",
			"profile_image", "user_avatar", "user_pic", "photo_url",
			"photo", "image_url", "image", "icon", "thumbnail",
		}
		for _, k := range avatarKeys {
			if raw, ok := v[k]; ok {
				if s, ok := raw.(string); ok && s != "" && !isPlaceholderAvatar(s) {
					meta.AvatarURL = s
					break
				}
			}
		}

		nameKeys := []string{
			"name", "display_name", "displayName", "full_name",
			"screen_name", "screenname", "user_name", "username", "title",
		}
		for _, k := range nameKeys {
			if raw, ok := v[k]; ok {
				if s, ok := raw.(string); ok && s != "" && meta.DisplayName == "" {
					meta.DisplayName = cleanMetadataString(s)
					break
				}
			}
		}

		bioKeys := []string{"bio", "description", "about", "status", "summary"}
		for _, k := range bioKeys {
			if raw, ok := v[k]; ok {
				if s, ok := raw.(string); ok && s != "" && meta.Bio == "" {
					meta.Bio = cleanMetadataString(s)
					break
				}
			}
		}

		followerKeys := []string{
			"followers", "followers_count", "followersCount",
			"subscribers_count", "subscribersCount",
		}
		for _, k := range followerKeys {
			if raw, ok := v[k]; ok {
				if meta.Followers == "" {
					meta.Followers = cleanMetadataString(fmt.Sprintf("%v", raw))
					break
				}
			}
		}

		locKeys := []string{"location", "city", "country", "address"}
		for _, k := range locKeys {
			if raw, ok := v[k]; ok {
				if s, ok := raw.(string); ok && s != "" && meta.Location == "" {
					meta.Location = cleanMetadataString(s)
					break
				}
			}
		}

		createdKeys := []string{"created_at", "createdAt", "created_time", "date_joined", "joined"}
		for _, k := range createdKeys {
			if raw, ok := v[k]; ok {
				if s, ok := raw.(string); ok && s != "" && meta.CreatedAt == "" {
					meta.CreatedAt = cleanMetadataString(s)
					break
				}
			}
		}

		nestedKeys := []string{"user", "profile", "data", "attributes", "author", "actor"}
		for _, nk := range nestedKeys {
			if nested, ok := v[nk]; ok {
				extractFromJSON(nested, meta)
			}
		}
		return meta.HasData()
	}
	return false
}

func ExtractProfileMetadata(content string, targetURL ...string) ProfileMetadata {
	var meta ProfileMetadata
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return meta
	}

	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var raw interface{}
		if err := json.Unmarshal([]byte(trimmed), &raw); err == nil {
			if extractFromJSON(raw, &meta) {
				if len(targetURL) > 0 && targetURL[0] != "" && meta.AvatarURL != "" {
					meta.AvatarURL = resolveAvatarURL(targetURL[0], meta.AvatarURL)
				}
				return meta
			}
		}
	}

	tokenizer := nethtml.NewTokenizer(strings.NewReader(content))
	var ogTitle, ogDesc, ogImage string
	var twitterTitle, twitterDesc, twitterImage string
	var standardDesc string
	var imgAvatar string
	var linkImage string

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
				case "og:image", "og:image:url", "og:image:secure_url":
					if ogImage == "" && !isPlaceholderAvatar(contentVal) {
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
					if twitterImage == "" && !isPlaceholderAvatar(contentVal) {
						twitterImage = contentVal
					}
				case "description":
					if standardDesc == "" {
						standardDesc = contentVal
					}
				}
			}
		}

		if (tt == nethtml.StartTagToken || tt == nethtml.SelfClosingTagToken) && strings.ToLower(tok.Data) == "link" {
			var relVal, hrefVal string
			for _, attr := range tok.Attr {
				k := strings.ToLower(attr.Key)
				if k == "rel" {
					relVal = strings.ToLower(strings.TrimSpace(attr.Val))
				} else if k == "href" {
					hrefVal = strings.TrimSpace(attr.Val)
				}
			}
			if hrefVal != "" && !isPlaceholderAvatar(hrefVal) {
				if relVal == "image_src" || relVal == "apple-touch-icon" || relVal == "profile_image" {
					if linkImage == "" {
						linkImage = hrefVal
					}
				}
			}
		}

		if (tt == nethtml.StartTagToken || tt == nethtml.SelfClosingTagToken) && strings.ToLower(tok.Data) == "img" {
			var srcVal, classVal, idVal, altVal, itempropVal string
			for _, attr := range tok.Attr {
				k := strings.ToLower(attr.Key)
				switch k {
				case "src":
					if srcVal == "" {
						srcVal = strings.TrimSpace(attr.Val)
					}
				case "data-src", "data-original":
					if attr.Val != "" {
						srcVal = strings.TrimSpace(attr.Val)
					}
				case "class":
					classVal = attr.Val
				case "id":
					idVal = attr.Val
				case "alt":
					altVal = attr.Val
				case "itemprop":
					itempropVal = attr.Val
				}
			}
			if srcVal != "" && !isPlaceholderAvatar(srcVal) {
				if isAvatarAttr(classVal) || isAvatarAttr(idVal) || isAvatarAttr(altVal) || isAvatarAttr(itempropVal) {
					if imgAvatar == "" {
						imgAvatar = srcVal
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
		if imgAvatar != "" && !isPlaceholderAvatar(imgAvatar) {
			meta.AvatarURL = imgAvatar
		} else if ogImage != "" && !isPlaceholderAvatar(ogImage) {
			meta.AvatarURL = ogImage
		} else if twitterImage != "" && !isPlaceholderAvatar(twitterImage) {
			meta.AvatarURL = twitterImage
		} else if linkImage != "" && !isPlaceholderAvatar(linkImage) {
			meta.AvatarURL = linkImage
		}
	}

	if meta.AvatarURL != "" && isPlaceholderAvatar(meta.AvatarURL) {
		meta.AvatarURL = ""
	}

	if len(targetURL) > 0 && targetURL[0] != "" && meta.AvatarURL != "" {
		meta.AvatarURL = resolveAvatarURL(targetURL[0], meta.AvatarURL)
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
			if img, ok := val["image"].(string); ok && img != "" && !isPlaceholderAvatar(img) {
				meta.AvatarURL = img
			} else if imgObj, ok := val["image"].(map[string]interface{}); ok {
				if u, ok := imgObj["url"].(string); ok && u != "" && !isPlaceholderAvatar(u) {
					meta.AvatarURL = u
				} else if cu, ok := imgObj["contentUrl"].(string); ok && cu != "" && !isPlaceholderAvatar(cu) {
					meta.AvatarURL = cu
				}
			} else if imgList, ok := val["image"].([]interface{}); ok && len(imgList) > 0 {
				if firstStr, ok := imgList[0].(string); ok && firstStr != "" && !isPlaceholderAvatar(firstStr) {
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
