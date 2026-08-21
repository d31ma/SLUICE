package s3store

import (
	"encoding/base64"
	"encoding/json"
	"errors"
)

const cursorVersion = 1

type listCursor struct {
	Prefix  string `json:"prefix"`
	Token   string `json:"token"`
	Version int    `json:"version"`
}

func encodeCursor(prefix, token string) (string, error) {
	payload, err := json.Marshal(listCursor{Prefix: prefix, Token: token, Version: cursorVersion})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeCursor(prefix, cursor string) (string, error) {
	if cursor == "" {
		return "", nil
	}
	if len(cursor) > maximumCursorBytes {
		return "", errors.New("list cursor is too large")
	}
	payload, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", errors.New("list cursor is malformed")
	}
	var decoded listCursor
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return "", errors.New("list cursor is malformed")
	}
	if decoded.Version != cursorVersion || decoded.Prefix != prefix || decoded.Token == "" {
		return "", errors.New("list cursor does not match this request")
	}
	return decoded.Token, nil
}
