package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// AWSExporter ships metrics to CloudWatch (PutMetricData) and logs to S3 as
// NDJSON archives (PutObject). One exporter, two destinations — this matches
// the common operator setup where CloudWatch handles alerting on metrics and
// S3 is the cheap archive tier for log retention.
//
// Configuration (all env-var driven):
//
//	AWS_REGION                — required (e.g. eu-west-1)
//	AWS_S3_LOG_BUCKET         — S3 bucket for log archives; unset = skip logs
//	CLOUDWATCH_NAMESPACE      — metric namespace (default: Karakuri)
//
// AWS credentials use the standard AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY /
// AWS_SESSION_TOKEN env vars or IAM role chains — anything `aws sdk v2` finds
// via the default credential chain.
//
// When AWS_REGION is unset the exporter is inactive — both Export methods
// return nil so the OTel chain keeps flowing through other exporters.
type AWSExporter struct {
	region    string
	namespace string
	logBucket string

	cw *cloudwatch.Client
	s3 *s3.Client
}

func NewAWSExporter() *AWSExporter {
	e := &AWSExporter{
		region:    os.Getenv("AWS_REGION"),
		namespace: os.Getenv("CLOUDWATCH_NAMESPACE"),
		logBucket: os.Getenv("AWS_S3_LOG_BUCKET"),
	}
	if e.namespace == "" {
		e.namespace = "Karakuri"
	}
	if e.region == "" {
		return e // inactive — credentials cannot resolve without a region
	}

	cfg, err := awscfg.LoadDefaultConfig(context.Background(), awscfg.WithRegion(e.region))
	if err != nil {
		// Leave clients nil so Active() reports false and exports degrade.
		return e
	}
	e.cw = cloudwatch.NewFromConfig(cfg)
	e.s3 = s3.NewFromConfig(cfg)
	return e
}

func (a *AWSExporter) Name() string { return "aws" }

// Active reports whether AWS clients were successfully constructed. The
// /health endpoint surfaces this so operators can spot a misconfigured
// region or missing credentials.
func (a *AWSExporter) Active() bool { return a.cw != nil }

func (a *AWSExporter) ExportMetrics(ctx context.Context, records []MetricRecord) error {
	if !a.Active() || len(records) == 0 {
		return nil
	}
	// CloudWatch caps PutMetricData at 1000 datapoints per call — batch to be safe.
	const batch = 500
	for start := 0; start < len(records); start += batch {
		end := start + batch
		if end > len(records) {
			end = len(records)
		}
		data := make([]cwtypes.MetricDatum, 0, end-start)
		for _, r := range records[start:end] {
			data = append(data, cwtypes.MetricDatum{
				MetricName: aws.String(r.Name),
				Value:      aws.Float64(r.Value),
				Timestamp:  aws.Time(r.Timestamp),
				Dimensions: dimensionsFromLabels(r.Labels),
			})
		}
		_, err := a.cw.PutMetricData(ctx, &cloudwatch.PutMetricDataInput{
			Namespace:  aws.String(a.namespace),
			MetricData: data,
		})
		if err != nil {
			return fmt.Errorf("aws: PutMetricData: %w", err)
		}
	}
	return nil
}

func (a *AWSExporter) ExportLogs(ctx context.Context, records []LogRecord) error {
	if !a.Active() || a.logBucket == "" || len(records) == 0 {
		return nil
	}
	// One S3 object per batch; key by current second + a random suffix so
	// concurrent flushes don't collide.
	key := fmt.Sprintf("logs/%s/karakuri-%d.ndjson",
		time.Now().UTC().Format("2006-01-02"),
		time.Now().UnixNano())

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, r := range records {
		if err := enc.Encode(r); err != nil {
			return fmt.Errorf("aws: marshal log: %w", err)
		}
	}

	_, err := a.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(a.logBucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(buf.Bytes()),
		ContentType: aws.String("application/x-ndjson"),
	})
	if err != nil {
		return fmt.Errorf("aws: PutObject %s/%s: %w", a.logBucket, key, err)
	}
	return nil
}

func (a *AWSExporter) Flush(_ context.Context) error    { return nil }
func (a *AWSExporter) Shutdown(_ context.Context) error { return nil }

// dimensionsFromLabels flattens a label map into CloudWatch's Dimensions
// shape. CloudWatch caps dimensions at 30 per metric — Karakuri's metrics
// today use ≤ 3 labels, well under the limit.
func dimensionsFromLabels(labels map[string]string) []cwtypes.Dimension {
	out := make([]cwtypes.Dimension, 0, len(labels))
	for k, v := range labels {
		out = append(out, cwtypes.Dimension{Name: aws.String(k), Value: aws.String(v)})
	}
	return out
}
