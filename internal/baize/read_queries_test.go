package baize

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadQueriesUseFixedEndpointsAndExcludeSensitiveFields(t *testing.T) {
	const (
		agentID   = "11111111-2222-3333-4444-555555555555"
		assetID   = "22222222-3333-4444-5555-666666666666"
		cronID    = "33333333-4444-5555-6666-777777777777"
		runbookID = "44444444-5555-6666-7777-888888888888"
		siteID    = "55555555-6666-7777-8888-999999999999"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer session-token" {
			t.Fatalf("missing authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case http.MethodGet + " /api/v1/observability/server-logs":
			if r.URL.Query().Get("level") != "error" || r.URL.Query().Get("limit") != "5" || r.URL.Query().Get("agent_id") != agentID {
				t.Fatalf("unexpected server log query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"source":"index","index":{"enabled":true,"started":true,"received":2,"indexed":2,"lastError":"private-index-error"},"items":[{"timestampMs":1,"level":"error","service":"server","module":"auth","message":"password=secret-log","source":"/private/log","requestId":"private-request","agentId":"` + agentID + `","taskId":"private-task","fingerprint":"private-fingerprint","error":"token=secret-error"}]}}`))
		case http.MethodGet + " /api/v1/observability/server-logs/overview":
			_, _ = w.Write([]byte(`{"code":0,"data":{"windowMinutes":60,"levelCounts":[{"level":"error","count":2}],"moduleCounts":[{"module":"auth","count":2}],"topErrors":[{"fingerprint":"private-fingerprint","level":"error","module":"auth","message":"token=top-secret","error":"password=top-error","count":2}],"index":{"enabled":true},"export":{"enabled":true,"format":"http_json","endpointConfigured":true,"endpoint":"https://private.example"}}}`))
		case http.MethodPost + " /api/v1/observability/agents/" + agentID + "/logs/query":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["limit"] != float64(5) {
				t.Fatalf("unexpected agent log body: %#v, err=%v", body, err)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"requestId":"private-agent-request","status":"completed","items":[{"level":"info","message":"api_key=agent-secret","source":"/var/private"}]}}`))
		case http.MethodGet + " /api/v1/alerts/incidents":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","source":"monitor","eventType":"cpu","resourceId":"private-resource","diagnosisAgentId":"` + agentID + `","diagnosisTargetType":"pid","diagnosisTargetValue":"/private/target","title":"High CPU","message":"token=alert-secret","severity":"critical","status":"open","acknowledgedBy":"private-user"}],"total":1,"page":1,"pageSize":20}}`))
		case http.MethodGet + " /api/v1/certificates":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":"bbbbbbbb-cccc-dddd-eeee-ffffffffffff","name":"Public site","host":"example.com","port":443,"enabled":true,"source":"nginx","agentId":"` + agentID + `","certificatePath":"/private/cert.pem","latestSnapshot":{"subject":"private-subject","issuer":"private-issuer","status":"error","errorMessage":"password=cert-secret"}}],"total":1,"page":1,"pageSize":20}}`))
		case http.MethodGet + " /api/v1/assets":
			if r.URL.Query().Get("pageSize") != "20" {
				t.Fatalf("asset pageSize = %q", r.URL.Query().Get("pageSize"))
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":"` + assetID + `","name":"Web asset","primaryIp":"10.0.0.1","secondaryIps":["10.0.0.2"],"hostname":"web-01","provider":"cloud","environment":"prod","status":"active","notes":"private-note","dueLevel":"normal","agentId":"` + agentID + `"}],"total":1,"page":1,"pageSize":20}}`))
		case http.MethodGet + " /api/v1/assets/summary":
			_, _ = w.Write([]byte(`{"code":0,"data":{"total":1,"active":1,"bound":1,"providerDistribution":[{"key":"cloud","count":1,"ratio":1}],"cost":{"primaryCurrency":"CNY","monthly":10,"yearly":120,"currencies":[{"currency":"CNY","monthly":10}]}}}`))
		case http.MethodGet + " /api/v1/assets/" + assetID:
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":"` + assetID + `","name":"Web asset","primaryIp":"10.0.0.1","hostname":"web-01","status":"active","notes":"private-note","summary":{"abnormal":false},"links":[{"url":"https://private.example"}]}}`))
		case http.MethodGet + " /api/v1/cron/jobs":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":"` + cronID + `","name":"Health check","scheduleType":"cron","cronExpr":"0 * * * *","command":"curl -H Authorization: Bearer secret","workDir":"/private/work","targetAgentIds":["` + agentID + `"],"operatorId":"private-user","operatorName":"Private User","enabled":true}],"total":1,"page":1,"pageSize":20}}`))
		case http.MethodGet + " /api/v1/cron/jobs/" + cronID:
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":"` + cronID + `","name":"Health check","scheduleType":"cron","command":"private-command","workDir":"/private/work","targetAgentIds":["` + agentID + `"],"operatorName":"Private User","enabled":true}}`))
		case http.MethodGet + " /api/v1/cron/jobs/" + cronID + "/logs":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":"cccccccc-dddd-eeee-ffff-000000000000","cronJobId":"` + cronID + `","execTaskId":"dddddddd-eeee-ffff-0000-111111111111","status":"failed","errorMessage":"token=cron-secret"}],"total":1,"page":1,"pageSize":20}}`))
		case http.MethodGet + " /api/v1/ops/runbooks":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":"` + runbookID + `","name":"Inspect service","description":"password=runbook-secret","status":"enabled","riskLevel":"read_only","requiredCapabilities":["runtime_diagnosis"],"inputs":[{"default":"private-input"}],"createdBy":"private-user"}],"total":1,"page":1,"pageSize":20}}`))
		case http.MethodGet + " /api/v1/ops/runbooks/" + runbookID:
			_, _ = w.Write([]byte(`{"code":0,"data":{"runbook":{"id":"` + runbookID + `","name":"Inspect service","status":"enabled","riskLevel":"read_only","inputs":[{"default":"private-input"}],"createdBy":"private-user"},"steps":[{"id":"eeeeeeee-ffff-0000-1111-222222222222","stepKey":"inspect","stepOrder":1,"stepType":"diagnosis","name":"Inspect","required":true,"riskLevel":"read_only","diagnosisTargetType":"service","inputBindings":{"secret":"private-binding"},"manualInstruction":"private-instruction","timeoutSec":30}]}}`))
		case http.MethodGet + " /api/v1/ops/runbooks/" + runbookID + "/audit-logs":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":"ffffffff-0000-1111-2222-333333333333","runbookId":"` + runbookID + `","action":"create_definition","operatorId":"private-user","operatorName":"Private User","clientIp":"10.0.0.1","detail":{"secret":"private-audit"}}],"total":1,"page":1,"pageSize":20}}`))
		case http.MethodGet + " /api/v1/nginx/sites":
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":"` + siteID + `","agentId":"` + agentID + `","name":"Web site","primaryHost":"example.com","status":"enabled","discoverySource":"agent","certificateStatus":"valid","enabled":true,"defaultServer":false,"todayRequestCount":10,"todayBlockCount":2,"configPath":"/private/nginx.conf"}]}`))
		case http.MethodGet + " /api/v1/nginx/sites/" + siteID:
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":"` + siteID + `","agentId":"` + agentID + `","name":"Web site","primaryHost":"example.com","status":"enabled","configPath":"/private/nginx.conf","serverBlocks":[{"configPath":"/private/site.conf"}]}}`))
		case http.MethodGet + " /api/v1/agents/" + agentID + "/analysis/nginx/overview":
			_, _ = w.Write([]byte(`{"code":0,"data":{"agentId":"` + agentID + `","traffic":{"totalRequests":10,"qps":1.2,"errorRate":0.1},"latency":{"p50Ms":5,"p90Ms":10,"p99Ms":20},"upstream":{"total":1,"healthy":1,"degraded":0,"unhealthy":0},"topSlow":[{"path":"/private/path","clientIp":"10.0.0.1"}]}}`))
		case http.MethodGet + " /api/v1/agents/" + agentID + "/nginx/latest":
			_, _ = w.Write([]byte(`{"code":0,"data":{"agentId":"` + agentID + `","isStale":false,"nginxNotDetected":false,"totalRequests":10,"qps":1.2,"status2xx":9,"status5xx":1,"activeConnections":2,"p50Ms":5}}`))
		case http.MethodGet + " /api/v1/agents/" + agentID + "/nginx/upstream":
			_, _ = w.Write([]byte(`{"code":0,"data":{"agentId":"` + agentID + `","items":[{"upstreamGroup":"api","address":"10.0.0.9:8080","requestCount":10,"errorRate":0.1,"status":"degraded"}]}}`))
		case http.MethodGet + " /api/v1/agents/" + agentID + "/nginx/slow-requests":
			_, _ = w.Write([]byte(`{"code":0,"data":{"agentId":"` + agentID + `","items":[{"time":"2026-08-20T00:00:00Z","method":"GET","path":"/private/path","clientIp":"10.0.0.1","status":500,"responseTime":1.5}],"total":1,"page":1,"pageSize":20}}`))
		case http.MethodGet + " /api/v1/agents/" + agentID + "/nginx/response-time-distribution":
			_, _ = w.Write([]byte(`{"code":0,"data":{"agentId":"` + agentID + `","p50Ms":5,"p90Ms":10,"p99Ms":20,"buckets":[{"label":"0-5ms","count":3}]}}`))
		case http.MethodGet + " /api/v1/security/exposure/overview":
			_, _ = w.Write([]byte(`{"code":0,"data":{"summary":{"totalFindings":2,"criticalFindings":1,"highFindings":1,"mediumFindings":0,"affectedAgentCount":1,"governableCount":2,"needConfirmationCount":1},"affectedAgents":[{"hostname":"private-host","ipAddress":"10.0.0.1"}]}}`))
		case http.MethodGet + " /api/v1/security/exposure/findings":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","code":"public_listener","title":"Public listener","severity":"high","category":"listener","status":"open","host":"private-host","evidenceSummary":"password=private-evidence","firstSeenAt":"2026-08-20T00:00:00Z","lastSeenAt":"2026-08-20T00:00:00Z"}],"total":1,"page":1,"pageSize":20}}`))
		case http.MethodGet + " /api/v1/security/exposure/scans":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":"bbbbbbbb-cccc-dddd-eeee-ffffffffffff","scopeIds":["` + agentID + `"],"status":"completed","findingCount":2,"criticalCount":1,"highCount":1,"startedAt":"2026-08-20T00:00:00Z","finishedAt":"2026-08-20T00:01:00Z"}],"total":1,"page":1,"pageSize":20}}`))
		case http.MethodGet + " /api/v1/security/network-entry/overview":
			_, _ = w.Write([]byte(`{"code":0,"data":{"summary":{"agentCount":1,"observationCount":2,"pathCount":1,"riskCount":1,"criticalRisks":1,"highRisks":0,"staleAgents":0,"warningCount":0},"topRisks":[{"host":"private-host","port":22}]}}`))
		case http.MethodGet + " /api/v1/security/network-entry/observations":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":"cccccccc-dddd-eeee-ffff-000000000000","sourceType":"listener","protocol":"tcp","bindAddress":"0.0.0.0","processPath":"/private/bin","summary":"token=private-observation","observedAt":"2026-08-20T00:00:00Z"}],"total":1,"page":1,"pageSize":20}}`))
		case http.MethodGet + " /api/v1/security/network-entry/paths":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":"dddddddd-eeee-ffff-0000-111111111111","protocol":"tcp","entryAddress":"0.0.0.0","targetAddress":"10.0.0.2","confidence":"confirmed","riskCodes":["public_listener"],"observedAt":"2026-08-20T00:00:00Z"}],"total":1,"page":1,"pageSize":20}}`))
		case http.MethodGet + " /api/v1/security/network-entry/risks":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":"eeeeeeee-ffff-0000-1111-222222222222","code":"public_listener","severity":"critical","category":"listener","status":"open","sourceType":"listener","host":"private-host","evidenceSummary":"token=private-risk","firstSeenAt":"2026-08-20T00:00:00Z","lastSeenAt":"2026-08-20T00:00:00Z"}],"total":1,"page":1,"pageSize":20}}`))
		case http.MethodGet + " /api/v1/system/release/status":
			_, _ = w.Write([]byte(`{"code":0,"data":{"generatedAt":"2026-08-20T00:00:00Z","components":[{"key":"server","label":"Server","currentVersion":"0.1.0","latestVersion":"0.2.0","currentCommit":"private-commit","image":"private-image","updateAvailable":true,"upgradeSupported":false,"upgradeDisabledReason":"deployment_not_supported"}],"updateSource":{"status":"ok","url":"https://private.example","message":"token=private-source"}}}`))
		case http.MethodGet + " /api/v1/system/release/changelog":
			_, _ = w.Write([]byte(`{"code":0,"data":{"entries":[{"component":"server","version":"0.2.0","title":"Release title","summary":"password=private-note","risk":"token=private-risk"}]}}`))
		case http.MethodGet + " /api/v1/subscription/status":
			_, _ = w.Write([]byte(`{"code":0,"data":{"planCode":"pro","licenseStatus":"valid","status":"active","billingCycle":"monthly","installId":"private-install","upgradeUrl":"https://private.example","telemetryPolicy":"optional","features":[{"featureKey":"remote_task","mode":"enabled"}],"limits":[{"limitKey":"agents","limitValue":10,"usedValue":2}],"restriction":{"active":false,"mode":"enabled"}}}`))
		case http.MethodGet + " /api/v1/subscription/usage":
			_, _ = w.Write([]byte(`{"code":0,"data":{"counters":[{"key":"tasks","label":"Tasks","value":2}],"windowFrom":"2026-08-01T00:00:00Z","windowTo":"2026-08-20T00:00:00Z"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/api/v1", "session-token", true, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	results := make([]any, 0, 15)
	serverLogs, err := client.QueryLogs(context.Background(), LogsQueryOptions{Source: "server", AgentID: agentID, Level: "error", Limit: 5})
	if err != nil || !serverLogs.RedactionApplied || serverLogs.Items[0].Message != "password=******" {
		t.Fatalf("server logs = %#v, err=%v", serverLogs, err)
	}
	results = append(results, serverLogs)
	overview, err := client.QueryLogs(context.Background(), LogsQueryOptions{Source: "overview"})
	if err != nil || overview.Overview == nil || !overview.RedactionApplied {
		t.Fatalf("log overview = %#v, err=%v", overview, err)
	}
	results = append(results, overview)
	agentLogs, err := client.QueryLogs(context.Background(), LogsQueryOptions{Source: "agent", AgentID: agentID, Limit: 5})
	if err != nil || !agentLogs.RedactionApplied {
		t.Fatalf("agent logs = %#v, err=%v", agentLogs, err)
	}
	results = append(results, agentLogs)
	alerts, err := client.ListAlerts(context.Background(), AlertsListOptions{Page: 1, PageSize: 20, Status: "open"})
	if err != nil || !alerts.RedactionApplied || len(alerts.Items) != 1 {
		t.Fatalf("alerts = %#v, err=%v", alerts, err)
	}
	results = append(results, alerts)
	certificates, err := client.ListCertificates(context.Background(), CertificatesListOptions{Page: 1, PageSize: 20})
	if err != nil || !certificates.RedactionApplied || len(certificates.Items) != 1 {
		t.Fatalf("certificates = %#v, err=%v", certificates, err)
	}
	results = append(results, certificates)
	for _, options := range []AssetsQueryOptions{{View: "list"}, {View: "summary"}, {View: "detail", ID: assetID}} {
		result, err := client.QueryAssets(context.Background(), options)
		if err != nil {
			t.Fatalf("QueryAssets(%q) error = %v", options.View, err)
		}
		results = append(results, result)
	}
	for _, options := range []CronJobsQueryOptions{{View: "list"}, {View: "detail", ID: cronID}, {View: "logs", ID: cronID}} {
		result, err := client.QueryCronJobs(context.Background(), options)
		if err != nil {
			t.Fatalf("QueryCronJobs(%q) error = %v", options.View, err)
		}
		results = append(results, result)
	}
	for _, options := range []RunbooksQueryOptions{{View: "list"}, {View: "detail", ID: runbookID}, {View: "audit", ID: runbookID}} {
		result, err := client.QueryRunbooks(context.Background(), options)
		if err != nil {
			t.Fatalf("QueryRunbooks(%q) error = %v", options.View, err)
		}
		results = append(results, result)
	}
	for _, options := range []NginxObserveOptions{{View: "sites"}, {View: "site", SiteID: siteID}, {View: "overview", AgentID: agentID}, {View: "latest", AgentID: agentID}, {View: "upstream", AgentID: agentID}, {View: "slow_requests", AgentID: agentID}, {View: "response_time", AgentID: agentID}} {
		result, err := client.ObserveNginx(context.Background(), options)
		if err != nil {
			t.Fatalf("ObserveNginx(%q) error = %v", options.View, err)
		}
		results = append(results, result)
	}
	for _, options := range []SecurityObserveOptions{{View: "exposure_overview"}, {View: "exposure_findings"}, {View: "exposure_scans"}, {View: "network_overview", AgentID: agentID}, {View: "network_observations", AgentID: agentID}, {View: "network_paths", AgentID: agentID}, {View: "network_risks", AgentID: agentID}} {
		result, err := client.ObserveSecurity(context.Background(), options)
		if err != nil {
			t.Fatalf("ObserveSecurity(%q) error = %v", options.View, err)
		}
		results = append(results, result)
	}
	release, err := client.GetSystemRelease(context.Background(), SystemReleaseOptions{})
	if err != nil {
		t.Fatalf("GetSystemRelease() error = %v", err)
	}
	results = append(results, release)
	subscription, err := client.GetSubscription(context.Background(), SubscriptionOptions{})
	if err != nil {
		t.Fatalf("GetSubscription() error = %v", err)
	}
	results = append(results, subscription)
	raw, err := json.Marshal(results)
	if err != nil {
		t.Fatalf("marshal results: %v", err)
	}
	for _, forbidden := range []string{"secret-log", "secret-error", "private-request", "private-task", "private-fingerprint", "/private/log", "private-agent-request", "/var/private", "private-resource", "/private/target", "private-user", "private-subject", "private-issuer", "/private/cert.pem", "10.0.0.1", "10.0.0.2", "private-note", "private.example", "private-command", "/private/work", "Private User", "private-input", "private-binding", "private-instruction", "private-audit"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("bounded read results contain %q: %s", forbidden, raw)
		}
	}
	for _, forbidden := range []string{"private-host", "private-evidence", "private-observation", "private-risk", "private-install", "private-commit", "private-image", "/private/nginx.conf", "/private/site.conf"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("new bounded read results contain %q: %s", forbidden, raw)
		}
	}
}

func TestReadQueriesRejectUnsupportedViewsAndUnboundedInput(t *testing.T) {
	client, err := NewClient("https://baize.example.com/api/v1", "session-token", false, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	from := time.Now().UTC()
	to := from.Add(8 * 24 * time.Hour)
	checks := []func() error{
		func() error {
			_, err := client.QueryLogs(context.Background(), LogsQueryOptions{Source: "http", Limit: 1})
			return err
		},
		func() error {
			_, err := client.ListAlerts(context.Background(), AlertsListOptions{Page: 1, PageSize: 51})
			return err
		},
		func() error {
			_, err := client.ListCertificates(context.Background(), CertificatesListOptions{Page: 1, PageSize: 20, Search: strings.Repeat("x", 201)})
			return err
		},
		func() error {
			_, err := client.QueryAssets(context.Background(), AssetsQueryOptions{View: "credentials"})
			return err
		},
		func() error {
			_, err := client.QueryCronJobs(context.Background(), CronJobsQueryOptions{View: "run"})
			return err
		},
		func() error {
			_, err := client.QueryRunbooks(context.Background(), RunbooksQueryOptions{View: "detail", ID: "../auth/profile"})
			return err
		},
		func() error {
			_, err := client.ObserveNginx(context.Background(), NginxObserveOptions{View: "config"})
			return err
		},
		func() error {
			_, err := client.ObserveSecurity(context.Background(), SecurityObserveOptions{View: "raw"})
			return err
		},
		func() error {
			_, err := client.ObserveNginx(context.Background(), NginxObserveOptions{View: "latest", AgentID: "not-a-uuid"})
			return err
		},
		func() error {
			_, err := client.ObserveNginx(context.Background(), NginxObserveOptions{View: "slow_requests", AgentID: "11111111-2222-3333-4444-555555555555", From: &from, To: &to})
			return err
		},
	}
	for index, check := range checks {
		if err := check(); err == nil {
			t.Errorf("check %d accepted unsupported or unbounded input", index)
		}
	}
}
