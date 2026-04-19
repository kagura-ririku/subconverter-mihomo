package model

import (
	"strconv"
	"strings"
	"time"
)

type SubscriptionInfo struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
	Total    int64 `json:"total"`
	Expire   int64 `json:"expire"`
}

func (s SubscriptionInfo) HeaderValue() string {
	return "upload=" + itoa64(s.Upload) +
		"; download=" + itoa64(s.Download) +
		"; total=" + itoa64(s.Total) +
		"; expire=" + itoa64(s.Expire)
}

type Provider struct {
	Name             string           `json:"name"`
	Type             string           `json:"type"`
	VehicleType      string           `json:"vehicleType"`
	UpdatedAt        time.Time        `json:"updatedAt"`
	SubscriptionInfo SubscriptionInfo `json:"subscriptionInfo"`
	Proxies          []Proxy          `json:"proxies"`
}

type Proxy map[string]any

type ProvidersResponse struct {
	Providers map[string]Provider `json:"providers"`
}

type Node struct {
	ProviderName string
	OriginalName string
	WorkingName  string
	FinalName    string
	Proxy        map[string]any
}

func itoa64(v int64) string {
	if v == 0 {
		return "0"
	}
	negative := v < 0
	if negative {
		v = -v
	}
	buf := [20]byte{}
	pos := len(buf)
	for v > 0 {
		pos--
		buf[pos] = byte('0' + (v % 10))
		v /= 10
	}
	if negative {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func HasSubscriptionInfo(s SubscriptionInfo) bool {
	return s.Upload != 0 || s.Download != 0 || s.Total != 0 || s.Expire != 0
}

func ParseSubscriptionInfo(raw string) *SubscriptionInfo {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	info := &SubscriptionInfo{}
	for _, field := range strings.Split(raw, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(field), "=")
		if !ok {
			continue
		}
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "upload":
			info.Upload = parsed
		case "download":
			info.Download = parsed
		case "total":
			info.Total = parsed
		case "expire":
			info.Expire = parsed
		}
	}
	if !HasSubscriptionInfo(*info) {
		return nil
	}
	return info
}
