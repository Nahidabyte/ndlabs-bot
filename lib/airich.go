package lib

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow/proto/waAICommon"
	waAICommonDeprecated "go.mau.fi/whatsmeow/proto/waAICommonDeprecated"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

type IEData struct {
	Type        string
	Key         string
	Text        string
	Url         string
	Width       float64
	Height      float64
	FontHeight  float64
	Padding     float64
	ReferenceId int
}

type ExtractResult struct {
	Text string
	IE   []IEData
}

func extractIE(text string, extract, hyperlink, citation, latex bool) ExtractResult {
	if !extract {
		return ExtractResult{Text: text, IE: []IEData{}}
	}

	var ie []IEData
	var result string
	last := 0
	citationIndex := 1
	hyperlinkIndex := 0
	latexIndex := 0

	var stack []int

	for i := 0; i < len(text); i++ {
		if text[i] == '[' && (i == 0 || text[i-1] != '\\') {
			stack = append(stack, i)
		} else if text[i] == ']' && i+1 < len(text) && (text[i+1] == '(' || text[i+1] == '<') {
			if len(stack) == 0 {
				continue
			}
			start := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			open := text[i+1]
			var closeByte byte = '>'
			typ := "latex"
			if open == '(' {
				closeByte = ')'
				typ = "link"
			}

			end := i + 2
			depth := 1
			for end < len(text) && depth > 0 {
				if text[end] == open && text[end-1] != '\\' {
					depth++
				} else if text[end] == closeByte && text[end-1] != '\\' {
					depth--
				}
				end++
			}
			if depth > 0 {
				continue
			}

			raw := strings.TrimSpace(text[start+1 : i])
			url := strings.TrimSpace(text[i+2 : end-1])

			var key, tag string
			var data IEData

			if typ == "latex" {
				if !latex {
					continue
				}
				parts := strings.Split(raw, "|")
				txt := ""
				if len(parts) > 0 {
					txt = parts[0]
					if strings.HasPrefix(txt, "B64:") {
						decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(txt, "B64:"))
						if err == nil {
							txt = string(decoded)
						}
					}
				}
				var width, height, fontHeight, padding float64
				if len(parts) > 1 && parts[1] != "" {
					width, _ = strconv.ParseFloat(parts[1], 64)
				}
				if len(parts) > 2 && parts[2] != "" {
					height, _ = strconv.ParseFloat(parts[2], 64)
				}
				if len(parts) > 3 && parts[3] != "" {
					fontHeight, _ = strconv.ParseFloat(parts[3], 64)
				}
				if len(parts) > 4 && parts[4] != "" {
					padding, _ = strconv.ParseFloat(parts[4], 64)
				}

				key = fmt.Sprintf("\u004E\u0049\u0058\u0045\u004C_LATEX_%d", latexIndex)
				latexIndex++
				displayTxt := txt
				if displayTxt == "" {
					displayTxt = "image"
				}
				tag = fmt.Sprintf("{{%s}}%s{{/%s}}", key, displayTxt, key)
				data = IEData{
					Type:       "latex",
					Key:        key,
					Text:       txt,
					Url:        url,
					Width:      width,
					Height:     height,
					FontHeight: fontHeight,
					Padding:    padding,
				}
			} else if raw != "" {
				if !hyperlink {
					continue
				}
				key = fmt.Sprintf("\u004E\u0049\u0058\u0045\u004C_HYPERLINK_%d", hyperlinkIndex)
				hyperlinkIndex++
				tag = fmt.Sprintf("{{%s}}%s{{/%s}}", key, raw, key)
				data = IEData{
					Type: "hyperlink",
					Key:  key,
					Text: raw,
					Url:  url,
				}
			} else {
				if !citation {
					continue
				}
				key = fmt.Sprintf("\u004E\u0049\u0058\u0045\u004C_CITATION_%d", citationIndex-1)
				tag = fmt.Sprintf("{{%s}}%s{{/%s}}", key, url, key)
				data = IEData{
					Type:        "citation",
					Key:         key,
					Url:         url,
					ReferenceId: citationIndex,
				}
				citationIndex++
			}

			result += text[last:start] + tag
			last = end
			ie = append(ie, data)
			i = end - 1
		}
	}
	result += text[last:]
	return ExtractResult{Text: result, IE: ie}
}

type AIRichBuilder struct {
	title               string
	submessages         []*waAICommonDeprecated.AIRichResponseSubMessage
	sections            []map[string]interface{}
	richResponseSources []*waAICommon.BotSourcesMetadata_BotSourceItem
}

func NewAIRichBuilder() *AIRichBuilder {
	return &AIRichBuilder{
		submessages:         make([]*waAICommonDeprecated.AIRichResponseSubMessage, 0),
		sections:            make([]map[string]interface{}, 0),
		richResponseSources: make([]*waAICommon.BotSourcesMetadata_BotSourceItem, 0),
	}
}

func (b *AIRichBuilder) SetTitle(title string) *AIRichBuilder {
	b.title = title
	return b
}

func (b *AIRichBuilder) AddRawSection(section map[string]interface{}) *AIRichBuilder {
	b.sections = append(b.sections, section)
	return b
}

func (b *AIRichBuilder) AddText(text string) *AIRichBuilder {
	res := extractIE(text, true, true, true, true)

	var inlineEntities []map[string]interface{}
	for _, ie := range res.IE {
		if ie.Type == "hyperlink" {
			inlineEntities = append(inlineEntities, map[string]interface{}{
				"key": ie.Key,
				"metadata": map[string]interface{}{
					"display_name": ie.Text,
					"is_trusted":   true,
					"url":          ie.Url,
					"__typename":   "GenAIInlineLinkItem",
				},
			})
		} else if ie.Type == "citation" {
			inlineEntities = append(inlineEntities, map[string]interface{}{
				"key": ie.Key,
				"metadata": map[string]interface{}{
					"reference_id":           ie.ReferenceId,
					"reference_url":          ie.Url,
					"reference_title":        ie.Url,
					"reference_display_name": ie.Url,
					"sources":                []interface{}{},
					"__typename":             "GenAISearchCitationItem",
				},
			})
		} else if ie.Type == "latex" {
			width := ie.Width
			if width == 0 {
				width = 100
			}
			height := ie.Height
			if height == 0 {
				height = 100
			}
			fontHeight := ie.FontHeight
			if fontHeight == 0 {
				fontHeight = 83.333333333333
			}
			padding := ie.Padding
			if padding == 0 {
				padding = 15
			}
			inlineEntities = append(inlineEntities, map[string]interface{}{
				"key": ie.Key,
				"metadata": map[string]interface{}{
					"latex_expression": ie.Text,
					"latex_image": map[string]interface{}{
						"url":    ie.Url,
						"width":  width,
						"height": height,
					},
					"font_height": fontHeight,
					"padding":     padding,
					"__typename":  "GenAILatexItem",
				},
			})
		}
	}

	b.submessages = append(b.submessages, &waAICommonDeprecated.AIRichResponseSubMessage{
		MessageType: waAICommonDeprecated.AIRichResponseSubMessageType_AI_RICH_RESPONSE_TEXT.Enum(),
		MessageText: proto.String(res.Text),
	})

	primitive := map[string]interface{}{
		"text":       res.Text,
		"__typename": "GenAIMarkdownTextUXPrimitive",
	}
	if len(inlineEntities) > 0 {
		primitive["inline_entities"] = inlineEntities
	}

	b.sections = append(b.sections, map[string]interface{}{
		"view_model": map[string]interface{}{
			"primitive":  primitive,
			"__typename": "GenAISingleLayoutViewModel",
		},
	})

	return b
}

func (b *AIRichBuilder) AddCode(language string, code string) *AIRichBuilder {
	b.submessages = append(b.submessages, &waAICommonDeprecated.AIRichResponseSubMessage{
		MessageType: waAICommonDeprecated.AIRichResponseSubMessageType_AI_RICH_RESPONSE_CODE.Enum(),
		CodeMetadata: &waAICommonDeprecated.AIRichResponseCodeMetadata{
			CodeLanguage: proto.String(language),
			CodeBlocks: []*waAICommonDeprecated.AIRichResponseCodeMetadata_AIRichResponseCodeBlock{
				{
					CodeContent:   proto.String(code),
					HighlightType: waAICommonDeprecated.AIRichResponseCodeMetadata_AI_RICH_RESPONSE_CODE_HIGHLIGHT_DEFAULT.Enum(),
				},
			},
		},
	})

	b.sections = append(b.sections, map[string]interface{}{
		"view_model": map[string]interface{}{
			"primitive": map[string]interface{}{
				"language": language,
				"code_blocks": []map[string]interface{}{
					{"content": code, "type": "DEFAULT"},
				},
				"__typename": "GenAICodeUXPrimitive",
			},
			"__typename": "GenAISingleLayoutViewModel",
		},
	})
	return b
}

func (b *AIRichBuilder) AddTable(table [][]string) *AIRichBuilder {
	if len(table) < 2 {
		return b
	}

	maxLen := 0
	for _, row := range table {
		if len(row) > maxLen {
			maxLen = len(row)
		}
	}

	normalize := func(r []string) []string {
		res := make([]string, maxLen)
		for i := 0; i < maxLen; i++ {
			if i < len(r) {
				res[i] = r[i]
			} else {
				res[i] = ""
			}
		}
		return res
	}

	var unifiedRows []map[string]interface{}
	var rowsMeta []*waAICommonDeprecated.AIRichResponseTableMetadata_AIRichResponseTableRow

	for i, row := range table {
		isHeader := i == 0
		cells := normalize(row)

		unifiedRows = append(unifiedRows, map[string]interface{}{
			"is_header": isHeader,
			"cells":     cells,
		})

		tr := &waAICommonDeprecated.AIRichResponseTableMetadata_AIRichResponseTableRow{
			Items: cells,
		}
		if isHeader {
			tr.IsHeading = proto.Bool(true)
		}
		rowsMeta = append(rowsMeta, tr)
	}

	b.submessages = append(b.submessages, &waAICommonDeprecated.AIRichResponseSubMessage{
		MessageType: waAICommonDeprecated.AIRichResponseSubMessageType_AI_RICH_RESPONSE_TABLE.Enum(),
		TableMetadata: &waAICommonDeprecated.AIRichResponseTableMetadata{
			Title: proto.String(""),
			Rows:  rowsMeta,
		},
	})

	b.sections = append(b.sections, map[string]interface{}{
		"view_model": map[string]interface{}{
			"primitive": map[string]interface{}{
				"rows":       unifiedRows,
				"__typename": "GenATableUXPrimitive",
			},
			"__typename": "GenAISingleLayoutViewModel",
		},
	})

	return b
}

func (b *AIRichBuilder) AddSource(sources [][]string) *AIRichBuilder {
	var primitiveSources []map[string]interface{}

	for _, s := range sources {
		profileURL, url, text := "", "", ""
		if len(s) > 0 {
			profileURL = s[0]
		}
		if len(s) > 1 {
			url = s[1]
		}
		if len(s) > 2 {
			text = s[2]
		}

		primitiveSources = append(primitiveSources, map[string]interface{}{
			"source_type":         "THIRD_PARTY",
			"source_display_name": text,
			"source_subtitle":     "AI",
			"source_url":          url,
			"favicon": map[string]interface{}{
				"url":       profileURL,
				"mime_type": "image/jpeg",
				"width":     16,
				"height":    16,
			},
		})
	}

	b.sections = append(b.sections, map[string]interface{}{
		"view_model": map[string]interface{}{
			"primitive": map[string]interface{}{
				"sources":    primitiveSources,
				"__typename": "GenAISearchResultPrimitive",
			},
			"__typename": "GenAISingleLayoutViewModel",
		},
	})

	return b
}

type ReelItem struct {
	Title          string
	ProfileIconUrl string
	ThumbnailUrl   string
	VideoUrl       string
	ReelsTitle     string
	LikesCount     int
	SharesCount    int
	ViewCount      int
	ReelSource     string
	IsVerified     bool
}

func (b *AIRichBuilder) AddReels(reels []ReelItem) *AIRichBuilder {
	var itemsMetadata []*waAICommonDeprecated.AIRichResponseContentItemsMetadata_AIRichResponseContentItemMetadata
	var primitives []map[string]interface{}

	for idx, item := range reels {
		itemsMetadata = append(itemsMetadata, &waAICommonDeprecated.AIRichResponseContentItemsMetadata_AIRichResponseContentItemMetadata{
			AIRichResponseContentItem: &waAICommonDeprecated.AIRichResponseContentItemsMetadata_AIRichResponseContentItemMetadata_ReelItem{
				ReelItem: &waAICommonDeprecated.AIRichResponseContentItemsMetadata_AIRichResponseReelItem{
					Title:          proto.String(item.Title),
					ProfileIconURL: proto.String(item.ProfileIconUrl),
					ThumbnailURL:   proto.String(item.ThumbnailUrl),
					VideoURL:       proto.String(item.VideoUrl),
				},
			},
		})

		b.richResponseSources = append(b.richResponseSources, &waAICommon.BotSourcesMetadata_BotSourceItem{
			Provider:          waAICommon.BotSourcesMetadata_BotSourceItem_UNKNOWN.Enum(),
			ThumbnailCDNURL:   proto.String(item.ThumbnailUrl),
			SourceProviderURL: proto.String(item.VideoUrl),
			SourceQuery:       proto.String(""),
			FaviconCDNURL:     proto.String(item.ProfileIconUrl),
			CitationNumber:    proto.Uint32(uint32(idx + 1)),
			SourceTitle:       proto.String(item.Title),
		})

		rs := item.ReelSource
		if rs == "" {
			rs = "IG"
		}

		primitives = append(primitives, map[string]interface{}{
			"reels_url":     item.VideoUrl,
			"thumbnail_url": item.ThumbnailUrl,
			"creator":       item.Title,
			"avatar_url":    item.ProfileIconUrl,
			"reels_title":   item.ReelsTitle,
			"likes_count":   item.LikesCount,
			"shares_count":  item.SharesCount,
			"view_count":    item.ViewCount,
			"reel_source":   rs,
			"is_verified":   item.IsVerified,
			"__typename":    "GenAIReelPrimitive",
		})
	}

	b.submessages = append(b.submessages, &waAICommonDeprecated.AIRichResponseSubMessage{
		MessageType: waAICommonDeprecated.AIRichResponseSubMessageType_AI_RICH_RESPONSE_CONTENT_ITEMS.Enum(),
		ContentItemsMetadata: &waAICommonDeprecated.AIRichResponseContentItemsMetadata{
			ContentType:   waAICommonDeprecated.AIRichResponseContentItemsMetadata_CAROUSEL.Enum(),
			ItemsMetadata: itemsMetadata,
		},
	})

	b.sections = append(b.sections, map[string]interface{}{
		"view_model": map[string]interface{}{
			"primitives": primitives,
			"__typename": "GenAIHScrollLayoutViewModel",
		},
	})

	return b
}

func (b *AIRichBuilder) AddImage(imageURLs []string) *AIRichBuilder {
	if len(imageURLs) == 0 {
		return b
	}

	var gridImageURLs []*waAICommonDeprecated.AIRichResponseImageURL
	sourceURL := string([]byte{104, 116, 116, 112, 115, 58, 47, 47, 102, 105, 111, 114, 97, 46, 110, 105, 120, 101, 108, 46, 109, 121, 46, 105, 100, 47})

	for _, u := range imageURLs {
		gridImageURLs = append(gridImageURLs, &waAICommonDeprecated.AIRichResponseImageURL{
			ImagePreviewURL: proto.String(u),
			ImageHighResURL: proto.String(u),
			SourceURL:       proto.String(sourceURL),
		})

		b.sections = append(b.sections, map[string]interface{}{
			"view_model": map[string]interface{}{
				"primitive": map[string]interface{}{
					"media": map[string]interface{}{
						"url":       u,
						"mime_type": "image/jpeg",
					},
					"imagine_type": 3,
					"status": map[string]interface{}{
						"status": "READY",
					},
					"__typename": "GenAIImaginePrimitive",
				},
				"__typename": "GenAISingleLayoutViewModel",
			},
		})
	}

	b.submessages = append(b.submessages, &waAICommonDeprecated.AIRichResponseSubMessage{
		MessageType: waAICommonDeprecated.AIRichResponseSubMessageType_AI_RICH_RESPONSE_GRID_IMAGE.Enum(),
		GridImageMetadata: &waAICommonDeprecated.AIRichResponseGridImageMetadata{
			GridImageURL: &waAICommonDeprecated.AIRichResponseImageURL{
				ImagePreviewURL: proto.String(imageURLs[0]),
			},
			ImageURLs: gridImageURLs,
		},
	})

	return b
}

func (b *AIRichBuilder) Build() *waE2E.Message {
	id := uuid.New().String()
	unifiedResponseData, _ := json.Marshal(map[string]interface{}{
		"response_id": id,
		"sections":    b.sections,
	})

	ctxInfo := &waE2E.ContextInfo{
		ForwardingScore: proto.Uint32(1),
		IsForwarded:     proto.Bool(true),
		ForwardedAiBotMessageInfo: &waAICommon.ForwardedAIBotMessageInfo{
			BotJID: proto.String("0@bot"),
		},
		ForwardOrigin: waE2E.ContextInfo_META_AI.Enum(),
	}

	botMetadata := &waAICommon.BotMetadata{}
	if b.title != "" {
		botMetadata.MessageDisclaimerText = proto.String(b.title)
	}
	if len(b.richResponseSources) > 0 {
		botMetadata.RichResponseSourcesMetadata = &waAICommon.BotSourcesMetadata{
			Sources: b.richResponseSources,
		}
	}

	return &waE2E.Message{
		MessageContextInfo: &waE2E.MessageContextInfo{
			DeviceListMetadataVersion: proto.Int32(2),
			BotMetadata:               botMetadata,
		},
		BotForwardedMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				RichResponseMessage: &waE2E.AIRichResponseMessage{
					MessageType: waAICommonDeprecated.AIRichResponseMessageType_AI_RICH_RESPONSE_TYPE_STANDARD.Enum(),
					Submessages: b.submessages,
					UnifiedResponse: &waAICommon.AIRichResponseUnifiedResponse{
						Data: unifiedResponseData,
					},
					ContextInfo: ctxInfo,
				},
			},
		},
	}
}
