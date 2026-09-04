package operations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"github.com/googleapis/gax-go/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/api/option"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	JobTypeGmailSync  = "gmail.sync"
	JobTypeTillerSync = "tiller.sync"
	jobsBasePath      = "/api/v1/jobs"
)

var (
	errJobsUnavailable = errors.New("background jobs are unavailable")
	jobsTracer         = otel.Tracer("github.com/NerdsWhoFish/kosmos/backend/operations/jobs")
)

type Job struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Scope        string `json:"scope"`
	ConnectionID string `json:"connectionId"`
	Actor        string `json:"actor,omitempty"`
}

type JobQueue interface {
	Enqueue(context.Context, Job, ...string) error
}

type ModuleOption func(*Module)

func WithJobQueue(queue JobQueue) ModuleOption {
	return func(module *Module) { module.jobs = queue }
}

type MemoryJobQueue struct {
	mu   sync.Mutex
	jobs []Job
	ids  map[string]struct{}
}

func NewMemoryJobQueue() *MemoryJobQueue { return &MemoryJobQueue{ids: make(map[string]struct{})} }

func (q *MemoryJobQueue) Enqueue(_ context.Context, job Job, _ ...string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.ids == nil {
		q.ids = make(map[string]struct{})
	}
	if _, exists := q.ids[job.ID]; exists {
		return nil
	}
	q.ids[job.ID] = struct{}{}
	q.jobs = append(q.jobs, job)
	return nil
}

func (q *MemoryJobQueue) Jobs() []Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]Job(nil), q.jobs...)
}

type CloudTasksConfig struct {
	ProjectID           string
	Location            string
	Queue               string
	TargetURL           string
	ServiceAccountEmail string
	Audience            string
}

type CloudTasksQueue struct {
	client              cloudTasksClient
	parent              string
	targetURL           string
	serviceAccountEmail string
	audience            string
}

type cloudTasksClient interface {
	CreateTask(context.Context, *cloudtaskspb.CreateTaskRequest, ...gax.CallOption) (*cloudtaskspb.Task, error)
	Close() error
}

func NewCloudTasksQueue(ctx context.Context, config CloudTasksConfig, options ...option.ClientOption) (*CloudTasksQueue, error) {
	config.ProjectID = strings.TrimSpace(config.ProjectID)
	config.Location = strings.TrimSpace(config.Location)
	config.Queue = strings.TrimSpace(config.Queue)
	config.TargetURL = strings.TrimRight(strings.TrimSpace(config.TargetURL), "/")
	config.ServiceAccountEmail = strings.TrimSpace(config.ServiceAccountEmail)
	config.Audience = strings.TrimSpace(config.Audience)
	if config.Audience == "" && config.TargetURL != "" {
		config.Audience = config.TargetURL
	}
	if config.ProjectID == "" || config.Location == "" || config.Queue == "" || config.ServiceAccountEmail == "" {
		return nil, errors.New("Cloud Tasks project, location, queue, and service account are required")
	}
	if config.TargetURL != "" {
		if err := validateJobTarget(config.TargetURL); err != nil {
			return nil, err
		}
	}
	client, err := cloudtasks.NewClient(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("create Cloud Tasks client: %w", err)
	}
	return &CloudTasksQueue{
		client:              client,
		parent:              fmt.Sprintf("projects/%s/locations/%s/queues/%s", config.ProjectID, config.Location, config.Queue),
		targetURL:           config.TargetURL,
		serviceAccountEmail: config.ServiceAccountEmail,
		audience:            config.Audience,
	}, nil
}

func (q *CloudTasksQueue) Close() error { return q.client.Close() }

func (q *CloudTasksQueue) Enqueue(ctx context.Context, job Job, targetOverrides ...string) error {
	targetURL := q.targetURL
	if len(targetOverrides) > 0 && strings.TrimSpace(targetOverrides[0]) != "" {
		targetURL = strings.TrimRight(strings.TrimSpace(targetOverrides[0]), "/")
	}
	if err := validateJobTarget(targetURL); err != nil {
		return err
	}
	audience := q.audience
	if audience == "" || len(targetOverrides) > 0 {
		audience = targetURL
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("encode job: %w", err)
	}
	headers := map[string]string{"Content-Type": "application/json"}
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(headers))
	task := &cloudtaskspb.Task{
		Name: q.parent + "/tasks/" + job.ID,
		MessageType: &cloudtaskspb.Task_HttpRequest{HttpRequest: &cloudtaskspb.HttpRequest{
			HttpMethod: cloudtaskspb.HttpMethod_POST,
			Url:        targetURL + jobsBasePath + "/execute",
			Headers:    headers,
			Body:       payload,
			AuthorizationHeader: &cloudtaskspb.HttpRequest_OidcToken{OidcToken: &cloudtaskspb.OidcToken{
				ServiceAccountEmail: q.serviceAccountEmail,
				Audience:            audience,
			}},
		}},
	}
	_, err = q.client.CreateTask(ctx, &cloudtaskspb.CreateTaskRequest{Parent: q.parent, Task: task})
	if status.Code(err) == grpcCodes.AlreadyExists {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create Cloud Task: %w", err)
	}
	return nil
}

func validateJobTarget(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("Cloud Tasks target must be an HTTPS origin")
	}
	return nil
}

func (m *Module) RegisterJobRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/jobs/schedule", m.scheduleJobs)
	mux.HandleFunc("POST /api/v1/jobs/execute", m.executeJob)
}

func (m *Module) enqueueJob(ctx context.Context, job Job, targetOverrides ...string) error {
	ctx, span := jobsTracer.Start(ctx, "jobs.enqueue")
	defer span.End()
	span.SetAttributes(attribute.String("job.type", job.Type))
	if job.ID == "" || job.Scope == "" || job.ConnectionID == "" || !oneOf(job.Type, JobTypeGmailSync, JobTypeTillerSync) {
		span.SetStatus(codes.Error, "invalid job")
		return errors.New("job payload is invalid")
	}
	if m.jobs == nil {
		span.SetStatus(codes.Error, "queue unavailable")
		return errJobsUnavailable
	}
	if err := m.jobs.Enqueue(ctx, job, targetOverrides...); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "enqueue failed")
		slog.ErrorContext(ctx, "background job enqueue failed", "job.type", job.Type, "job.status", "failed")
		return err
	}
	slog.InfoContext(ctx, "background job enqueued", "job.type", job.Type, "job.status", "queued")
	return nil
}

func (m *Module) scheduleJobs(w http.ResponseWriter, r *http.Request) {
	ctx, span := jobsTracer.Start(r.Context(), "jobs.schedule")
	defer span.End()
	var connections []GoogleConnection
	if err := m.store.List(ctx, m.publicScope, "googleConnections", &connections); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "connections load failed")
		slog.ErrorContext(ctx, "background job scheduling failed", "job.type", "batch", "job.status", "failed")
		writeError(w, http.StatusInternalServerError, "job_schedule_failed", "Could not schedule integration syncs")
		return
	}
	batchKey := strings.TrimSpace(r.Header.Get("X-CloudScheduler-ScheduleTime"))
	if batchKey == "" {
		batchKey = time.Now().UTC().Truncate(time.Minute).Format(time.RFC3339)
	}
	targetURL, err := jobTargetFromRequest(r)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid worker origin")
		writeError(w, http.StatusBadRequest, "invalid_job_origin", "Worker origin is invalid")
		return
	}
	queued := 0
	for _, connection := range connections {
		jobs := []Job{{Type: JobTypeGmailSync, Scope: m.publicScope, ConnectionID: connection.ID, Actor: "system"}}
		if connection.Tiller != nil {
			jobs = append(jobs, Job{Type: JobTypeTillerSync, Scope: m.publicScope, ConnectionID: connection.ID, Actor: "system"})
		}
		for _, job := range jobs {
			job.ID = deterministicID("scheduled|" + batchKey + "|" + job.Type + "|" + connection.ID)
			if err := m.enqueueJob(ctx, job, targetURL); err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "enqueue failed")
				writeError(w, http.StatusServiceUnavailable, "job_schedule_failed", "Could not schedule integration syncs")
				return
			}
			queued++
		}
	}
	slog.InfoContext(ctx, "background jobs scheduled", "job.type", "batch", "job.status", "queued", "job.count", queued)
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "accepted", "queued": queued})
}

func jobTargetFromRequest(r *http.Request) (string, error) {
	host := strings.ToLower(strings.TrimSpace(r.Host))
	if strings.Contains(host, ":") {
		return "", errors.New("worker host must not include a port")
	}
	if !strings.HasSuffix(host, ".run.app") {
		return "", errors.New("worker host must be a Cloud Run origin")
	}
	return "https://" + host, nil
}

func (m *Module) executeJob(w http.ResponseWriter, r *http.Request) {
	ctx, span := jobsTracer.Start(r.Context(), "jobs.execute")
	defer span.End()
	var job Job
	if !decodeJSON(w, r, &job, 32<<10) {
		span.SetStatus(codes.Error, "invalid job")
		return
	}
	span.SetAttributes(attribute.String("job.type", job.Type))
	if job.ID == "" || job.Scope != m.publicScope || job.ConnectionID == "" || !oneOf(job.Type, JobTypeGmailSync, JobTypeTillerSync) {
		span.SetStatus(codes.Error, "invalid job")
		writeError(w, http.StatusBadRequest, "invalid_job", "Job payload is invalid")
		return
	}
	var completed JobExecution
	if err := m.store.Get(ctx, job.Scope, "jobExecutions", job.ID, &completed); err == nil && completed.Status == "completed" {
		slog.InfoContext(ctx, "background job already completed", "job.type", job.Type, "job.status", "completed")
		writeJSON(w, http.StatusOK, map[string]string{"status": "completed"})
		return
	}
	var err error
	switch job.Type {
	case JobTypeGmailSync:
		_, err = m.syncEmailConnection(ctx, job.Scope, job.ConnectionID, job.Actor, job.ID)
	case JobTypeTillerSync:
		_, _, err = m.syncTillerConnection(ctx, job.Scope, job.ConnectionID, job.Actor, job.ID)
	}
	if errors.Is(err, errNotFound) {
		slog.InfoContext(ctx, "background job skipped", "job.type", job.Type, "job.status", "connection_missing")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "execution failed")
		slog.ErrorContext(ctx, "background job execution failed", "job.type", job.Type, "job.status", "failed")
		writeError(w, http.StatusBadGateway, "job_execution_failed", "Integration sync failed")
		return
	}
	execution := JobExecution{ID: job.ID, Type: job.Type, Status: "completed", CompletedAt: time.Now().UTC()}
	if err := m.store.Put(ctx, job.Scope, "jobExecutions", job.ID, execution); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "completion save failed")
		slog.ErrorContext(ctx, "background job completion save failed", "job.type", job.Type, "job.status", "failed")
		writeError(w, http.StatusInternalServerError, "job_completion_failed", "Could not record job completion")
		return
	}
	slog.InfoContext(ctx, "background job completed", "job.type", job.Type, "job.status", "completed")
	writeJSON(w, http.StatusOK, map[string]string{"status": "completed"})
}
