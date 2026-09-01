package application

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

var errInvalidCursor = errors.New("messaging: invalid message cursor")

func encodeCursor(cursor MessageCursor) string {
	raw := strconv.FormatInt(cursor.CreatedAt.UTC().UnixMicro(), 10) + ":" + cursor.ID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(value string) (MessageCursor, error) {
	if value == "" || len(value) > 256 {
		return MessageCursor{}, errInvalidCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return MessageCursor{}, errInvalidCursor
	}
	microsRaw, id, found := strings.Cut(string(raw), ":")
	if !found || id == "" || len([]rune(id)) > 64 {
		return MessageCursor{}, errInvalidCursor
	}
	micros, err := strconv.ParseInt(microsRaw, 10, 64)
	if err != nil {
		return MessageCursor{}, errInvalidCursor
	}
	createdAt := time.UnixMicro(micros).UTC()
	if createdAt.IsZero() {
		return MessageCursor{}, errInvalidCursor
	}
	return MessageCursor{CreatedAt: createdAt, ID: id}, nil
}
