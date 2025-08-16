package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// Config represents tracing configuration
type Config struct {
	Enabled      bool   `yaml:"enabled" json:"enabled"`
	ServiceName  string `yaml:"service_name" json:"service_name"`
	ServiceVersion string `yaml:"service_version" json:"service_version"`
	Environment  string `yaml:"environment" json:"environment"`
	
	// Exporter configuration
	ExporterType string `yaml:"exporter_type" json:"exporter_type"` // "jaeger", "otlp", "console"
	
	// Jaeger configuration
	JaegerEndpoint string `yaml:"jaeger_endpoint" json:"jaeger_endpoint"`
	
	// OTLP configuration
	OTLPEndpoint string `yaml:"otlp_endpoint" json:"otlp_endpoint"`
	OTLPHeaders  map[string]string `yaml:"otlp_headers" json:"otlp_headers"`
	
	// Sampling configuration
	SamplingRatio float64 `yaml:"sampling_ratio" json:"sampling_ratio"`
}

// TracingService manages OpenTelemetry tracing
type TracingService struct {
	config   Config
	tracer   oteltrace.Tracer
	provider *trace.TracerProvider
}

// NewTracingService creates a new tracing service
func NewTracingService(config Config) (*TracingService, error) {
	if !config.Enabled {
		// Return a no-op tracer
		return &TracingService{
			config: config,
			tracer: otel.Tracer("kvstore-noop"),
		}, nil
	}

	// Create resource
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(config.ServiceName),
			semconv.ServiceVersionKey.String(config.ServiceVersion),
			semconv.DeploymentEnvironmentKey.String(config.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create exporter based on configuration
	var exporter trace.SpanExporter
	switch config.ExporterType {
	case "jaeger":
		exporter, err = jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(config.JaegerEndpoint)))
		if err != nil {
			return nil, fmt.Errorf("failed to create Jaeger exporter: %w", err)
		}
	case "otlp":
		client := otlptracehttp.NewClient(
			otlptracehttp.WithEndpoint(config.OTLPEndpoint),
			otlptracehttp.WithHeaders(config.OTLPHeaders),
		)
		exporter, err = otlptrace.New(context.Background(), client)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
		}
	case "console":
		// For development - logs spans to console
		exporter, err = NewConsoleExporter()
		if err != nil {
			return nil, fmt.Errorf("failed to create console exporter: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported exporter type: %s", config.ExporterType)
	}

	// Create tracer provider
	samplingRatio := config.SamplingRatio
	if samplingRatio <= 0 {
		samplingRatio = 1.0 // Default to sampling all traces
	}

	tp := trace.NewTracerProvider(
		trace.WithResource(res),
		trace.WithBatcher(exporter),
		trace.WithSampler(trace.TraceIDRatioBased(samplingRatio)),
	)

	// Set global tracer provider
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Create tracer
	tracer := tp.Tracer("kvstore")

	return &TracingService{
		config:   config,
		tracer:   tracer,
		provider: tp,
	}, nil
}

// StartSpan starts a new span
func (ts *TracingService) StartSpan(ctx context.Context, name string, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	return ts.tracer.Start(ctx, name, opts...)
}

// AddSpanAttributes adds attributes to the current span
func (ts *TracingService) AddSpanAttributes(span oteltrace.Span, attrs ...attribute.KeyValue) {
	span.SetAttributes(attrs...)
}

// RecordError records an error in the current span
func (ts *TracingService) RecordError(span oteltrace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// SetSpanStatus sets the status of a span
func (ts *TracingService) SetSpanStatus(span oteltrace.Span, code codes.Code, description string) {
	span.SetStatus(code, description)
}

// Close shuts down the tracing service
func (ts *TracingService) Close(ctx context.Context) error {
	if ts.provider != nil {
		return ts.provider.Shutdown(ctx)
	}
	return nil
}

// GetTracer returns the underlying tracer
func (ts *TracingService) GetTracer() oteltrace.Tracer {
	return ts.tracer
}

// TraceOperation is a helper function to trace an operation
func (ts *TracingService) TraceOperation(ctx context.Context, operationName string, fn func(context.Context, oteltrace.Span) error) error {
	ctx, span := ts.StartSpan(ctx, operationName)
	defer span.End()

	err := fn(ctx, span)
	if err != nil {
		ts.RecordError(span, err)
		return err
	}

	ts.SetSpanStatus(span, codes.Ok, "")
	return nil
}

// InstrumentStorageOperation creates a span for storage operations
func (ts *TracingService) InstrumentStorageOperation(ctx context.Context, operation string, key string) (context.Context, oteltrace.Span) {
	return ts.StartSpan(ctx, fmt.Sprintf("storage.%s", operation),
		oteltrace.WithAttributes(
			attribute.String("storage.operation", operation),
			attribute.String("storage.key", key),
			attribute.String("component", "storage"),
		),
	)
}

// InstrumentHTTPRequest creates a span for HTTP requests
func (ts *TracingService) InstrumentHTTPRequest(ctx context.Context, method string, path string) (context.Context, oteltrace.Span) {
	return ts.StartSpan(ctx, fmt.Sprintf("http.%s %s", method, path),
		oteltrace.WithAttributes(
			attribute.String("http.method", method),
			attribute.String("http.route", path),
			attribute.String("component", "http"),
		),
	)
}

// InstrumentGRPCRequest creates a span for gRPC requests
func (ts *TracingService) InstrumentGRPCRequest(ctx context.Context, service string, method string) (context.Context, oteltrace.Span) {
	return ts.StartSpan(ctx, fmt.Sprintf("grpc.%s/%s", service, method),
		oteltrace.WithAttributes(
			attribute.String("rpc.service", service),
			attribute.String("rpc.method", method),
			attribute.String("rpc.system", "grpc"),
			attribute.String("component", "grpc"),
		),
	)
}

// DefaultConfig returns a default tracing configuration
func DefaultConfig() Config {
	return Config{
		Enabled:        false,
		ServiceName:    "kvstore",
		ServiceVersion: "1.0.0",
		Environment:    "development",
		ExporterType:   "console",
		JaegerEndpoint: "http://localhost:14268/api/traces",
		OTLPEndpoint:   "http://localhost:4318",
		OTLPHeaders:    make(map[string]string),
		SamplingRatio:  1.0,
	}
}

// ProductionConfig returns a production-ready tracing configuration
func ProductionConfig(serviceName, version, environment string) Config {
	return Config{
		Enabled:        true,
		ServiceName:    serviceName,
		ServiceVersion: version,
		Environment:    environment,
		ExporterType:   "otlp",
		OTLPEndpoint:   "http://localhost:4318",
		OTLPHeaders:    make(map[string]string),
		SamplingRatio:  0.1, // Sample 10% of traces in production
	}
}

// DevelopmentConfig returns a development tracing configuration
func DevelopmentConfig() Config {
	return Config{
		Enabled:        true,
		ServiceName:    "kvstore-dev",
		ServiceVersion: "dev",
		Environment:    "development",
		ExporterType:   "console",
		SamplingRatio:  1.0, // Sample all traces in development
	}
}