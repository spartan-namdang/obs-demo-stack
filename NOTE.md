# Note

## 1. Observability Stack Configuration

### Adding Grafana dashboards by IDs
- **Loki**: 14055
- **Redis**: 11835
- **Postgres**: 9628
- **Go Metrics**: 13240
- **Go Processes**: 6671

### Linking Logs (Loki) and Traces (Tempo)
- To enable navigation from Logs to Traces in Grafana, a specific configuration is used in `k8s/observability/components/grafana.yaml`:

```yaml
# 2. Loki (Logs)
- name: Loki
  type: loki
  url: http://loki.monitoring.svc.cluster.local:3100
  jsonData:
    derivedFields:
      # THE MAGIC GLUE: Links Logs -> Traces
      - datasourceUid: Tempo
        matcherRegex: \\\"trace_id\\\":\\\"(\w+)\\\"
        name: TraceID
        url: "$${__value.raw}"
```
- **Explanation**: Logs from services include the `trace_id`. We use regex to extract this ID and create a link to the Tempo datasource.
- **Too many '\\' character?** This is expected because the regex is written inside a YAML string (and ultimately rendered through Helm). Each layer (YAML -> Helm -> Grafana) requires escaping, so the extra backslashes ensure the final regex received by Grafana is \"trace_id\":\"(\w+)\".
- **How to test the matcherRegex?** Grafana - Connections - Data sources - Loki - Derived fields - Show example log message - Test regex with example log line

### ServiceMonitor requirements
For Prometheus to collect metrics from services via a `ServiceMonitor`, the following conditions must be met:
1. **Labels**: `Service.metadata.labels` must match `ServiceMonitor.spec.selector.matchLabels`.
2. **Namespace**: `Service.metadata.namespace` must match `ServiceMonitor.spec.namespaceSelector.matchNames`.
3. **Port Name**: `Service.spec.ports[].name` must match `ServiceMonitor.spec.endpoints[].port`.

### Config rules for Alertmanager (Prometheus)
- Disable default rules to avoid noise (30+ rules)
```yaml
defaultRules:
  create: false
```
- Add custom rules:
  + 1. Add PrometheusRule config with label `release: prometheus` to `observability/components/alerts` folder
  + 2. Add `release: prometheus` to `prometheus.prometheusSpec.ruleSelector.matchLabels` in `prometheus.yaml`

## 2. Troubleshooting & Known Issues

### Prometheus targets down (Minikube)
- **Issue**: `kube-controller-manager`, `kube-etcd`, and `kube-scheduler` targets appear as **DOWN** in Prometheus.

- **Cause**:
On Minikube, control-plane components run as static pods listening on `127.0.0.1` (not bound to the node IP). Prometheus attempts to scrape them via the Node IP, which fails.

- **Solution**:
In local development (Minikube), these monitors can be disabled in the `kube-prometheus-stack` values:

```yaml
kubeControllerManager:
  enabled: false
kubeScheduler:
  enabled: false
kubeEtcd:
  enabled: false
```
*Note: `kube-prometheus-stack` must be deployed before deploying ServiceMonitors to avoid missing CRD errors.*

### Loki deployment & Startup issues
- **Note**: `loki-stack` is deprecated; use `loki` instead.
  + **Deployment Mode**: The demo uses `SingleBinary` (Monolithic). Default is `SimpleScalable`.
  + If using `SingleBinary`, ensure `singleBinary`, `read`, `write`, and `backend` values are set correctly to avoid validation failures.

#### Error: Loki cannot start (schema version)
- **Error log**:
```
CONFIG ERROR: schema v13 is required ... your schema version is v12. Set allow_structured_metadata: false ...
```
- **Solution**: Disable structured metadata in the Loki configuration.

#### Error: Loki chunks cache (insufficient memory)
- **Error Log**:
```
0/1 nodes are available: 1 Insufficient memory...
```
- **Solution**: Disable the chunks cache.
```yaml
chunksCache:
  enabled: false
```

### Grafana: No data or missing metrics
If dashboards show no data:
1. **Check Prometheus targets**: Go to `/targets` in Prometheus and check if the service endpoint is UP.
2. **Verify metrics locally**:
  - Port-forward the service (e.g., `kubectl port-forward -n demo svc/checkout-api 8888`).
  - Check metrics: `curl http://localhost:8888/metrics`.
3. **Compare keys**: Ensure the metric keys in the curl response match exactly what is used in the Grafana dashboard JSON (case-sensitive).

## 3. Practical Experiences

### Design from correlation first, not from tools
- Metrics -> What is wrong? (Mimir/Prometheus)
- Logs -> Why is it wrong? (Loki)
- Traces -> Where exactly is it wrong? (Tempo)
- Grafana -> The brain & correlation layer

### Start with RED + USE dashboards
- RED (for services):
  + Rate: how many requests?
  + Errors: how many failed?
  + Duration: how long do requests take?
- USE (for infra):
  + Utilization: how busy is it?
  + Saturation: is it overloaded / queued?
  + Errors: is it failing?
- Expose metrics that answer questions, instead of everything "just in case"

### Cardinality problem
- Avoid labels like: `user_id`, `order_id`, `request_id`, `email`
- This will destroy Prometheus memory
- => Put those into logs / traces, not metrics
- Bad:
```yaml
labels:
  level: info
  message: "user login failed"
```
- Good:
```
labels:
  app: shipping-worker
  env: prod
  level: error
```
- Labels = low-cardinality, filter
- Log body = high-cardinality, detail

### Metrics for alerts, not debugging
- Metrics = alert trigger
- Logs/Traces = investigation

### Common Loki mistakes
- Putting `trace_id` as a label
- Parsing too many fields into labels
- Expecting Loki to behave like Elasticsearch
- => Loki is cheap, simple, fast - not full-text search.

### Tracing is optional (until it's not)
- Small monolith -> traces feel useless
- Microservices + async + queues -> traces are necessary
  + Always log `trace_id`, never rely on Tempo UI alone

### Don't build pretty Grafana dashboards first
- Bad dashboard:
  + 30 panels
  + No one knows what action to take
- Good dashboard:
  + 1. Golden signals (RED)
  + 2. Top errors
  + 3. Latency percentiles (P95/P99)
  + 4. Links -> logs & traces

### Alert on symptoms, not causes
- Bad: CPU > 80%
- Good:
  + Error rate > 2%
  + P95 latency > 2s
  + Queue length growing for 5m
- => CPU is context, not incident

### Should use Alertmanager (Prometheus) or Alerting (in Grafana)?
- If an alert protects your infrastructure or uptime -> Prometheus.
  + If Grafana is down -> alerts stop
- If it protects business logic or KPIs -> Grafana.

### LGTM or APM
- LGTM is used when:
  + Don't want vendor lock-in
  + Use Kubernetes
  + Value cost control
  + Accept simplicity over magic
- "click button -> auto insights" -> Datadog/NewRelic
- control & engineering discipline -> LGTM

## 4. Concepts

### Alert evaluation group (in Grafana alerting)
- Example:
  + Evaluation group: `cpu-alerts`
  + Evaluation interval: 30s
  + => Every 30 seconds, all alert rules in `cpu-alerts` are evaluated.
- Why evaluation groups exist
  + Reduce load by controlling evaluation frequency
  + Ensure related alerts are evaluated consistently