package baize

import (
	"context"
	"net/http"
	"strings"
)

var alertChangeActions = map[string]struct{}{
	"acknowledge": {},
	"resolve":     {},
}

// AlertChangeOptions 描述一次告警状态变更请求。
// 动作、权限、状态冲突和审计均由白泽服务端最终判断。
type AlertChangeOptions struct {
	IncidentID string
	Action     string
}

// AlertChangeResult 只表示服务端已接受状态变更请求。
// 告警接口当前返回空数据，调用方应再查询告警确认最终状态。
type AlertChangeResult struct {
	IncidentID        string `json:"incidentId"`
	Action            string `json:"action"`
	Accepted          bool   `json:"accepted"`
	StatusQueryNeeded bool   `json:"statusQueryNeeded"`
	Notice            string `json:"notice"`
}

// ChangeAlert 确认或解决一条告警事件，不接受任意路径或 HTTP 方法。
func (c *Client) ChangeAlert(ctx context.Context, options AlertChangeOptions) (AlertChangeResult, error) {
	incidentID, err := validateUUID(options.IncidentID, "incident ID")
	if err != nil {
		return AlertChangeResult{}, err
	}
	action := strings.ToLower(strings.TrimSpace(options.Action))
	if _, ok := alertChangeActions[action]; !ok {
		return AlertChangeResult{}, newInputError("alert action must be acknowledge or resolve")
	}
	if err := c.do(ctx, http.MethodPost, []string{"alerts", "incidents", incidentID, action}, nil, nil, nil, true); err != nil {
		return AlertChangeResult{}, err
	}
	return AlertChangeResult{
		IncidentID:        incidentID,
		Action:            action,
		Accepted:          true,
		StatusQueryNeeded: true,
		Notice:            "Baize accepted the alert change request; query the alert again to confirm its final status.",
	}, nil
}
