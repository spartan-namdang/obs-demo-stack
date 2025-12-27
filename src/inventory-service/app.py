import os
import time
import json
import redis
import psycopg2
import boto3
import logging
from fastapi import FastAPI
from opentelemetry import trace
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from opentelemetry.instrumentation.redis import RedisInstrumentor
from opentelemetry.instrumentation.psycopg2 import Psycopg2Instrumentor
from opentelemetry.instrumentation.boto3sqs import Boto3SQSInstrumentor
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource
from prometheus_fastapi_instrumentator import Instrumentator

# Setup OTel
resource = Resource(attributes={"service.name": "inventory-service"})
trace.set_tracer_provider(TracerProvider(resource=resource))
otlp_exporter = OTLPSpanExporter(endpoint="tempo:4317", insecure=True)
trace.get_tracer_provider().add_span_processor(BatchSpanProcessor(otlp_exporter))

app = FastAPI()

# Auto-instrument libraries
FastAPIInstrumentor.instrument_app(app)
RedisInstrumentor().instrument()
Psycopg2Instrumentor().instrument()
Boto3SQSInstrumentor().instrument()
Instrumentator().instrument(app).expose(app)

# Connections
r = redis.Redis(host=os.getenv("REDIS_HOST"), port=6379, db=0)
pg_conn = psycopg2.connect(os.getenv("POSTGRES_DSN"))
sqs = boto3.client("sqs", 
                   endpoint_url=os.getenv("SQS_ENDPOINT"), 
                   region_name="us-east-1",
                   aws_access_key_id="test", 
                   aws_secret_access_key="test")
QUEUE_URL = os.getenv("SQS_QUEUE_URL")

@app.get("/data")
def get_data():
    tracer = trace.get_tracer(__name__)
    with tracer.start_as_current_span("process-data"):
        # 1. Redis
        hits = r.incr("hits")
        
        # 2. Postgres
        with pg_conn.cursor() as cur:
            cur.execute("INSERT INTO access_log (timestamp) VALUES (NOW())")
            pg_conn.commit()

        # 3. SQS
        msg = {"source": "inventory-service", "hits": hits}
        sqs.send_message(QueueUrl=QUEUE_URL, MessageBody=json.dumps(msg))
        
        # Log structured JSON
        print(json.dumps({
            "level": "info",
            "msg": "Processed data",
            "hits": hits,
            "trace_id": format(trace.get_current_span().get_span_context().trace_id, "032x")
        }))
        
        return {"hits": hits}