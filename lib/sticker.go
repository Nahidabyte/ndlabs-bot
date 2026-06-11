package lib

import (
	"encoding/json"
	"fmt"
)

type StickerMetadata struct {
	StickerPackID        string `json:"sticker-pack-id"`
	StickerPackName      string `json:"sticker-pack-name"`
	StickerPackPublisher string `json:"sticker-pack-publisher"`
}

func AddStickerWatermark(webpData []byte, packName, author string) []byte {
	metadata := StickerMetadata{
		StickerPackID:        "ndlabs-bot-" + fmt.Sprintf("%d", len(webpData)),
		StickerPackName:      packName,
		StickerPackPublisher: author,
	}

	jsonMetadata, _ := json.Marshal(metadata)

	return append(webpData, jsonMetadata...)
}
