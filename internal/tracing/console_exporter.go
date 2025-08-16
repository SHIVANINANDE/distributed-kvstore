package tracing

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace"
)

// ConsoleExporter exports traces to console for development
type ConsoleExporter struct {
	logger *log.Logger
}

// NewConsoleExporter creates a new console exporter
func NewConsoleExporter() (*ConsoleExporter, error) {
	return &ConsoleExporter{
		logger: log.New(os.Stdout, "[TRACE] ", log.LstdFlags),
	}, nil
}

// ExportSpans exports spans to console
func (ce *ConsoleExporter) ExportSpans(ctx context.Context, spans []trace.ReadOnlySpan) error {
	for _, span := range spans {
		spanData := map[string]interface{}{
			"trace_id":    span.SpanContext().TraceID().String(),
			"span_id":     span.SpanContext().SpanID().String(),
			"parent_id":   span.Parent().SpanID().String(),
			"name":        span.Name(),
			"start_time":  span.StartTime(),
			"end_time":    span.EndTime(),
			"duration":    span.EndTime().Sub(span.StartTime()),
			"status":      span.Status(),
			"attributes":  attributesToMap(span.Attributes()),
			"events":      eventsToMaps(span.Events()),
		}

		jsonData, err := json.MarshalIndent(spanData, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal span data: %w", err)
		}

		ce.logger.Printf("Span: %s", string(jsonData))
	}
	return nil
}

// Shutdown shuts down the exporter
func (ce *ConsoleExporter) Shutdown(ctx context.Context) error {
	ce.logger.Println("Console exporter shutting down")
	return nil
}

// attributesToMap converts span attributes to a map
func attributesToMap(attrs []attribute.KeyValue) map[string]interface{} {
	result := make(map[string]interface{})
	for _, attr := range attrs {
		result[string(attr.Key)] = attr.Value.AsInterface()
	}
	return result
}

// eventsToMaps converts span events to a slice of maps
func eventsToMaps(events []trace.Event) []map[string]interface{} {
	result := make([]map[string]interface{}, len(events))
	for i, event := range events {
		result[i] = map[string]interface{}{
			"name":       event.Name,
			"time":       event.Time,
			"attributes": attributesToMap(event.Attributes),
		}
	}
	return result
}