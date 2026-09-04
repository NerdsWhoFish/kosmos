package operations

import (
	"context"
	"encoding/json"
	"testing"

	"cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"github.com/googleapis/gax-go/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeCloudTasksClient struct {
	request *cloudtaskspb.CreateTaskRequest
	err     error
}

func (f *fakeCloudTasksClient) CreateTask(_ context.Context, request *cloudtaskspb.CreateTaskRequest, _ ...gax.CallOption) (*cloudtaskspb.Task, error) {
	f.request = request
	return request.Task, f.err
}

func (*fakeCloudTasksClient) Close() error { return nil }

func TestCloudTasksQueueBuildsOIDCRequest(t *testing.T) {
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(previousPropagator) })
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1},
		SpanID:     trace.SpanID{2},
		TraceFlags: trace.FlagsSampled,
		Remote:     false,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanContext)
	client := &fakeCloudTasksClient{}
	queue := &CloudTasksQueue{
		client:              client,
		parent:              "projects/project-1/locations/us-east1/queues/kosmos-jobs",
		targetURL:           "https://jobs.example.com",
		serviceAccountEmail: "jobs@example.iam.gserviceaccount.com",
		audience:            "https://jobs.example.com",
	}
	job := Job{ID: "job-123", Type: JobTypeGmailSync, Scope: "scope-1", ConnectionID: "connection-1", Actor: "system"}
	if err := queue.Enqueue(ctx, job); err != nil {
		t.Fatal(err)
	}
	request := client.request
	if request.Parent != queue.parent || request.Task.Name != queue.parent+"/tasks/job-123" {
		t.Fatalf("task location = %#v", request)
	}
	httpRequest := request.Task.GetHttpRequest()
	if httpRequest.GetUrl() != queue.targetURL+jobsBasePath+"/execute" || httpRequest.GetHttpMethod() != cloudtaskspb.HttpMethod_POST {
		t.Fatalf("HTTP request = %#v", httpRequest)
	}
	if httpRequest.GetOidcToken().GetServiceAccountEmail() != queue.serviceAccountEmail || httpRequest.GetOidcToken().GetAudience() != queue.audience {
		t.Fatalf("OIDC token = %#v", httpRequest.GetOidcToken())
	}
	if httpRequest.GetHeaders()["traceparent"] == "" {
		t.Fatalf("trace context was not propagated: %#v", httpRequest.GetHeaders())
	}
	var decoded Job
	if err := json.Unmarshal(httpRequest.GetBody(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != job {
		t.Fatalf("job = %#v, want %#v", decoded, job)
	}
}

func TestCloudTasksQueueTreatsExistingTaskAsSuccess(t *testing.T) {
	client := &fakeCloudTasksClient{err: status.Error(codes.AlreadyExists, "duplicate")}
	queue := &CloudTasksQueue{client: client, parent: "projects/p/locations/l/queues/q", targetURL: "https://jobs.example.com"}
	if err := queue.Enqueue(context.Background(), Job{ID: "same-job"}); err != nil {
		t.Fatalf("duplicate enqueue = %v", err)
	}
}

func TestCloudTasksQueueUsesWorkerOriginOverride(t *testing.T) {
	client := &fakeCloudTasksClient{}
	queue := &CloudTasksQueue{client: client, parent: "projects/p/locations/l/queues/q", serviceAccountEmail: "jobs@example.com"}
	if err := queue.Enqueue(context.Background(), Job{ID: "scheduled"}, "https://kosmos-jobs-hash-ue.a.run.app"); err != nil {
		t.Fatal(err)
	}
	request := client.request.Task.GetHttpRequest()
	if request.GetUrl() != "https://kosmos-jobs-hash-ue.a.run.app/api/v1/jobs/execute" || request.GetOidcToken().GetAudience() != "https://kosmos-jobs-hash-ue.a.run.app" {
		t.Fatalf("request = %#v", request)
	}
}

func TestCloudTasksQueueRejectsIncompleteOrInsecureConfig(t *testing.T) {
	for _, config := range []CloudTasksConfig{
		{},
		{ProjectID: "p", Location: "l", Queue: "q", TargetURL: "http://jobs.example.com", ServiceAccountEmail: "jobs@example.com"},
	} {
		if _, err := NewCloudTasksQueue(context.Background(), config); err == nil {
			t.Fatalf("config %#v should fail", config)
		}
	}
}

func TestMemoryJobQueueReturnsDefensiveCopy(t *testing.T) {
	queue := NewMemoryJobQueue()
	job := Job{ID: "job-1", Type: JobTypeGmailSync}
	if err := queue.Enqueue(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if err := queue.Enqueue(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	jobs := queue.Jobs()
	if len(jobs) != 1 {
		t.Fatalf("duplicate job count = %d, want 1", len(jobs))
	}
	jobs[0].Type = JobTypeTillerSync
	if queue.Jobs()[0].Type != JobTypeGmailSync {
		t.Fatal("Jobs returned mutable queue state")
	}
}
