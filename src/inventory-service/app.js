// app.js
require("./otel");

const express = require("express");
const Redis = require("ioredis");
const { Pool } = require("pg");
const { SQSClient, SendMessageCommand } = require("@aws-sdk/client-sqs");
const { trace } = require("@opentelemetry/api");
const client = require("prom-client");

const app = express();
const tracer = trace.getTracer("inventory-service");

// Prometheus metrics
client.collectDefaultMetrics();

// Redis
const redis = new Redis({
  host: process.env.REDIS_HOST,
  port: 6379,
  lazyConnect: true,
});
redis.on("error", (err) => {
  console.error("Redis error:", err.message);
});

// Postgres
const pgPool = new Pool({
  connectionString: process.env.POSTGRES_DSN,
});

// SQS
const sqs = new SQSClient({
  region: "us-east-1",
  endpoint: process.env.SQS_ENDPOINT,
  credentials: {
    accessKeyId: "test",
    secretAccessKey: "test",
  },
});

const QUEUE_URL = process.env.SQS_QUEUE_URL;

// Routes
app.get("/data", async (req, res) => {
  await tracer.startActiveSpan("process-data", async (span) => {
    try {
      // 1️⃣ Redis
      const hits = await redis.incr("hits");

      // 2️⃣ Postgres
      await pgPool.query(
        "INSERT INTO access_log (timestamp) VALUES (NOW())"
      );

      // 3️⃣ SQS
      const msg = {
        source: "inventory-service",
        hits,
      };

      await sqs.send(
        new SendMessageCommand({
          QueueUrl: QUEUE_URL,
          MessageBody: JSON.stringify(msg),
        })
      );

      // Structured log
      console.log(
        JSON.stringify({
          level: "info",
          msg: "Processed data",
          hits,
          trace_id: span.spanContext().traceId,
        })
      );

      res.json({ hits });
    } catch (err) {
      console.error("❌ /data error:", err);
      span.recordException(err);
      span.setStatus({ code: 2, message: err.message });
      res.status(500).json({ error: err.message });
    } finally {
      span.end();
    }
  });
});

// Prometheus endpoint
app.get("/metrics", async (req, res) => {
  res.set("Content-Type", client.register.contentType);
  res.end(await client.register.metrics());
});

// Health check
app.get("/healthz", async (req, res) => {
  try {
    await redis.ping();
    await pgPool.query("SELECT 1");
    await sqs.send(new GetQueueAttributesCommand({
      QueueUrl: QUEUE_URL,
      AttributeNames: ["QueueArn"],
    }));
    res.send("ok");
  } catch (e) {
    res.status(503).send("not ready");
  }
});

// Start server
app.listen(8000, () => {
  console.log("inventory-service listening on port 8000");
});
