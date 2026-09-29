package api

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
)

// 统一错误形：{"error":{"code":"...","message":"..."}}
type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// 部分错误附带的额外字段（如冷却到期时间）。
	Extra map[string]interface{} `json:"extra,omitempty"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: message}})
}

func writeErrExtra(w http.ResponseWriter, status int, code, message string, extra map[string]interface{}) {
	writeJSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: message, Extra: extra}})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求体不是合法 JSON: "+err.Error())
		return false
	}
	return true
}

// clientIP 在反代场景取 X-Forwarded-For 首跳，直连场景取 RemoteAddr。
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first := strings.TrimSpace(strings.Split(xff, ",")[0])
			if first != "" {
				return first
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func isErrNotFound(err error) bool { return errors.Is(err, errNotFound) }
