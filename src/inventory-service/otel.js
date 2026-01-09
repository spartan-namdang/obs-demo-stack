// otel.js
const { NodeSDK } = require("@opentelemetry/sdk-node");
const { Resource } = require("@opentelemetry/resources");
const { SemanticResourceAttributes } = require("@opentelemetry/semantic-conventions");
const { OTLPTraceExporter } = require("@opentelemetry/exporter-trace-otlp-grpc");
const {
  HttpInstrumentation,
} = require("@opentelemetry/instrumentation-http");
const {
  ExpressInstrumentation,
} = require("@opentelemetry/instrumentation-express");
const {
  RedisInstrumentation,
} = require("@opentelemetry/instrumentation-redis");
const {
  PgInstrumentation,
} = require("@opentelemetry/instrumentation-pg");

const sdk = new NodeSDK({
  resource: new Resource({
    "service.name": "inventory-service",
  }),
  traceExporter: new OTLPTraceExporter({
    url: "grpc://tempo.monitoring.svc.cluster.local:4317",
  }),
  instrumentations: [
    new HttpInstrumentation(),
    new ExpressInstrumentation(),
    new RedisInstrumentation(),
    new PgInstrumentation(),
  ],
});

sdk.start();
