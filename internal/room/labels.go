package room

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/m1k1o/neko-rooms/internal/types"
)

var labelRegex = regexp.MustCompile(`^[a-z0-9.-]+$`)

type RoomLabels struct {
	Name        string
	URL         string
	CotesterURL string
	Mux         bool
	Epr         EprPorts

	NekoImage  string
	ApiVersion int

	BrowserPolicy *BrowserPolicyLabels
	UserDefined   map[string]string
	Deadline      time.Time
	ApiEndpoint   string
	SessionID     string
	ApiKey        string
}

type BrowserPolicyLabels struct {
	Type types.BrowserPolicyType
	Path string
}

func (manager *RoomManagerCtx) extractLabels(labels map[string]string) (*RoomLabels, error) {
	name, ok := labels["m1k1o.neko_rooms.name"]
	if !ok {
		return nil, fmt.Errorf("damaged container labels: name not found")
	}

	url, ok := labels["m1k1o.neko_rooms.url"]
	if !ok {
		// TODO: It should be always available.
		url = manager.config.GetRoomUrl(name)
		//return nil, fmt.Errorf("damaged container labels: url not found")
	}

	cotesterURL, ok := labels["cotester.vb-orchestrator.url"]
	if !ok {
		return nil, fmt.Errorf("damaged container labels: cotester.vb-orchestrator.url not found")
	}

	var mux bool
	var epr EprPorts

	muxStr, ok := labels["m1k1o.neko_rooms.mux"]
	if ok {
		muxPort, err := strconv.ParseUint(muxStr, 10, 16)
		if err != nil {
			return nil, err
		}

		mux = true
		epr = EprPorts{
			Min: uint16(muxPort),
			Max: uint16(muxPort),
		}
	} else {
		eprMinStr, ok := labels["m1k1o.neko_rooms.epr.min"]
		if !ok {
			return nil, fmt.Errorf("damaged container labels: epr.min not found")
		}

		eprMin, err := strconv.ParseUint(eprMinStr, 10, 16)
		if err != nil {
			return nil, err
		}

		eprMaxStr, ok := labels["m1k1o.neko_rooms.epr.max"]
		if !ok {
			return nil, fmt.Errorf("damaged container labels: epr.max not found")
		}

		eprMax, err := strconv.ParseUint(eprMaxStr, 10, 16)
		if err != nil {
			return nil, err
		}

		mux = false
		epr = EprPorts{
			Min: uint16(eprMin),
			Max: uint16(eprMax),
		}
	}

	nekoImage, ok := labels["m1k1o.neko_rooms.neko_image"]
	if !ok {
		return nil, fmt.Errorf("damaged container labels: neko_image not found")
	}

	apiVersion := 2 // default, prior to api versioning
	apiVersionStr, ok := labels["m1k1o.neko_rooms.api_version"]
	if ok {
		var err error
		apiVersion, err = strconv.Atoi(apiVersionStr)
		if err != nil {
			return nil, err
		}
	}

	var browserPolicy *BrowserPolicyLabels
	if val, ok := labels["m1k1o.neko_rooms.browser_policy"]; ok && val == "true" {
		policyType, ok := labels["m1k1o.neko_rooms.browser_policy.type"]
		if !ok {
			return nil, fmt.Errorf("damaged container labels: browser_policy.type not found")
		}

		policyPath, ok := labels["m1k1o.neko_rooms.browser_policy.path"]
		if !ok {
			return nil, fmt.Errorf("damaged container labels: browser_policy.path not found")
		}

		browserPolicy = &BrowserPolicyLabels{
			Type: types.BrowserPolicyType(policyType),
			Path: policyPath,
		}
	}

	// extract user defined labels
	userDefined := map[string]string{}
	for key, val := range labels {
		if strings.HasPrefix(key, "m1k1o.neko_rooms.x-") {
			userDefined[strings.TrimPrefix(key, "m1k1o.neko_rooms.x-")] = val
		}
	}

	deadlineStr, ok := labels["cotester.vb-orchestrator.deadline"]
	if !ok {
		return nil, fmt.Errorf("damaged container labels: cotester.vb-orchestrator.deadline not found")
	}

	deadline, err := time.Parse(time.RFC3339, deadlineStr)
	if err != nil {
		return nil, err
	}

	apiEndpoint, ok := labels["cotester.vb-orchestrator.api-endpoint"]
	if !ok {
		return nil, fmt.Errorf("damaged container labels: cotester.vb-orchestrator.api-endpoint not found")
	}

	sessionID, ok := labels["cotester.vb-orchestrator.session-id"]
	if !ok {
		return nil, fmt.Errorf("damaged container labels: cotester.vb-orchestrator.session-id not found")
	}

	apiKey, ok := labels["cotester.vb-orchestrator.api-key"]
	if !ok {
		return nil, fmt.Errorf("damaged container labels: cotester.vb-orchestrator.api-key not found")
	}

	return &RoomLabels{
		Name:        name,
		URL:         url,
		CotesterURL: cotesterURL,
		Mux:         mux,
		Epr:         epr,

		NekoImage:  nekoImage,
		ApiVersion: apiVersion,

		BrowserPolicy: browserPolicy,
		UserDefined:   userDefined,
		Deadline:      deadline,
		ApiEndpoint:   apiEndpoint,
		SessionID:     sessionID,
		ApiKey:        apiKey,
	}, nil
}

func (manager *RoomManagerCtx) serializeLabels(labels RoomLabels) map[string]string {
	labelsMap := map[string]string{
		"m1k1o.neko_rooms.name":                 labels.Name,
		"m1k1o.neko_rooms.url":                  manager.config.GetRoomUrl(labels.Name),
		"cotester.vb-orchestrator.url":          manager.config.GetCotesterUrl(labels.Name),
		"m1k1o.neko_rooms.instance":             manager.config.InstanceName,
		"m1k1o.neko_rooms.neko_image":           labels.NekoImage,
		"cotester.vb-orchestrator.deadline":     labels.Deadline.Format(time.RFC3339),
		"cotester.vb-orchestrator.api-endpoint": labels.ApiEndpoint,
		"cotester.vb-orchestrator.session-id":   labels.SessionID,
		"cotester.vb-orchestrator.api-key":      labels.ApiKey,
	}

	// api version 2 is currently default
	if labels.ApiVersion != 2 {
		labelsMap["m1k1o.neko_rooms.api_version"] = fmt.Sprintf("%d", labels.ApiVersion)
	}

	if labels.Mux && labels.Epr.Min == labels.Epr.Max {
		labelsMap["m1k1o.neko_rooms.mux"] = fmt.Sprintf("%d", labels.Epr.Min)
	} else {
		labelsMap["m1k1o.neko_rooms.epr.min"] = fmt.Sprintf("%d", labels.Epr.Min)
		labelsMap["m1k1o.neko_rooms.epr.max"] = fmt.Sprintf("%d", labels.Epr.Max)
	}

	if labels.BrowserPolicy != nil {
		labelsMap["m1k1o.neko_rooms.browser_policy"] = "true"
		labelsMap["m1k1o.neko_rooms.browser_policy.type"] = string(labels.BrowserPolicy.Type)
		labelsMap["m1k1o.neko_rooms.browser_policy.path"] = labels.BrowserPolicy.Path
	}

	for key, val := range labels.UserDefined {
		// to lowercase
		key = strings.ToLower(key)

		labelsMap[fmt.Sprintf("m1k1o.neko_rooms.x-%s", key)] = val
	}

	return labelsMap
}

func CheckLabelKey(name string) bool {
	return labelRegex.MatchString(name)
}
