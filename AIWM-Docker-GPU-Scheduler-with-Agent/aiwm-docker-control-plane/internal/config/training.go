package config

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/application"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/objectstore/s3"
)

// Training is optional; an empty S3 endpoint leaves existing execution unchanged.
func loadTraining() (s3.Config, application.TrainingOptions, error) {
	c := s3.Config{Endpoint: env("AIWM_S3_ENDPOINT", ""), PublicEndpoint: env("AIWM_S3_PUBLIC_ENDPOINT", ""), Region: env("AIWM_S3_REGION", "us-east-1"), Bucket: env("AIWM_S3_BUCKET", "aiwm-artifacts"), AccessKey: env("AIWM_S3_ACCESS_KEY", ""), SecretKey: env("AIWM_S3_SECRET_KEY", "")}
	o := application.DefaultTrainingOptions()
	o.APIURL = env("AIWM_TRAINING_API_URL", "")
	var err error
	c.CreateBucket, err = BoolEnv("AIWM_S3_CREATE_BUCKET", false)
	if err != nil {
		return c, o, err
	}
	o.WarningFraction, err = strconv.ParseFloat(env("AIWM_CHECKPOINT_WARNING_FRACTION", "0.1"), 64)
	if err != nil || !(o.WarningFraction > 0 && o.WarningFraction <= 1) {
		return c, o, fmt.Errorf("invalid AIWM_CHECKPOINT_WARNING_FRACTION")
	}
	for _, v := range []struct {
		key    string
		target *time.Duration
	}{
		{"AIWM_CHECKPOINT_WARNING_MIN", &o.MinWarningLead}, {"AIWM_CHECKPOINT_WARNING_MAX", &o.MaxWarningLead}, {"AIWM_TRAINING_UPLOAD_TIMEOUT", &o.UploadTimeout},
	} {
		*v.target, err = envDuration(v.key, *v.target)
		if err != nil {
			return c, o, err
		}
	}
	if o.MaxWarningLead < o.MinWarningLead {
		return c, o, fmt.Errorf("checkpoint warning maximum must be >= minimum")
	}
	o.MaxUploadBytes, err = strconv.ParseInt(env("AIWM_TRAINING_MAX_UPLOAD_BYTES", "5368709120"), 10, 64)
	if err != nil || o.MaxUploadBytes <= 0 {
		return c, o, fmt.Errorf("invalid AIWM_TRAINING_MAX_UPLOAD_BYTES")
	}
	if c.Endpoint != "" {
		u, e := url.Parse(o.APIURL)
		if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return c, o, fmt.Errorf("AIWM_TRAINING_API_URL must be a Control Plane base URL reachable from training containers")
		}
		if c.AccessKey == "" || c.SecretKey == "" {
			return c, o, fmt.Errorf("AIWM_S3_ACCESS_KEY and AIWM_S3_SECRET_KEY are required")
		}
	}
	return c, o, nil
}
